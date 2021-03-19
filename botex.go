package botex

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/dustin/go-humanize"
	natsd "github.com/nats-io/nats-server/v2/server"
	nats "github.com/nats-io/nats.go"
	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/botex/config"
	"github.com/scraperwall/botex/data"
	"github.com/scraperwall/botex/plugins"
	"github.com/scraperwall/botex/store"
	"github.com/scraperwall/geoip/v2"
	log "github.com/sirupsen/logrus"
)

const natsRequestsSubject = "requests"

// Botex detects bad bots
type Botex struct {
	natsSubscriptions []*nats.Subscription

	history   *History
	blocked   *Block
	config    *config.Config
	resources *Resources
	api       *API
	plugins   []Plugin

	ctx context.Context
}

// HandleRequest handles incoming requests
func (b *Botex) HandleRequest(r *data.Request) {
	if r.Timestamp < 1<<32 { // seconds
		r.Time = time.Unix(r.Timestamp, 0)
	} else { // nanoseconds
		r.Time = time.Unix(0, r.Timestamp)
	}

	ip := net.ParseIP(r.Source)

	log.Tracef("received %s - %s", ip, r.URL)
	b.history.Add(r)
	log.Tracef("added %s to history", ip)

	r.ASN = b.resources.ASNDB.Lookup(ip)

	go func() {
		for _, p := range b.plugins {
			p.HandleRequest(r)
		}
	}()

	log.Trace("HandleRequest done")
}

type natsAuth struct {
	User     string
	Password string
}

func (na *natsAuth) Check(c natsd.ClientAuthentication) bool {
	return c.GetOpts().Username == na.User && c.GetOpts().Password == na.Password
}

// New creates a new Botex instance
func New(ctx context.Context, config *config.Config) (*Botex, error) {
	var err error

	blockRecheckChan := make(chan bool, 100)
	blockASNNetChan := make(chan bool, 100)
	resources := NewResources()

	// Badger
	//
	bopts := badger.DefaultOptions(config.BadgerPath)
	bopts.SyncWrites = true
	bopts.DetectConflicts = true
	resources.Store, err = store.NewBadgerDB(ctx, config.BadgerPath)
	if err != nil {
		return nil, err
	}

	resources.BlockChan = make(chan *IPDetails, 100)

	// ASN Database
	//
	resources.ASNDB, err = asndb.New(config.ASNDBFile)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("asndb loaded with %d records", resources.ASNDB.Size())

	// GeoIP Database
	//
	resources.GEOIPDB, err = geoip.New(config.GeoIPDBFile)
	if err != nil {
		return nil, err
	}
	log.Infof("geoipdb loaded")

	// NATS server
	//
	nopts := &natsd.Options{
		HTTPPort: config.NatsHTTPPort,
		Port:     config.NatsPort,
		CustomClientAuthentication: &natsAuth{
			User:     config.NatsUser,
			Password: config.NatsPassword,
		},
		MaxConn:      1 << 12,
		MaxPending:   1 << 32,
		NoLog:        false,
		TraceVerbose: true,
	}

	resources.NatsServer = natsd.New(nopts)
	go resources.NatsServer.Start()
	if !resources.NatsServer.ReadyForConnections(2 * time.Second) {
		resources.NatsServer.Shutdown()
		return nil, errors.New("nats server failed to startup")
	}
	log.Info("natsd started")

	// NATS client
	//
	natsSlowLogFunc := func(c *nats.Conn, s *nats.Subscription, err error) {
		pnum, psize, _ := s.Pending()
		delivered, _ := s.Delivered()
		dropped, _ := s.Dropped()

		log.Warnf("nats error: %s del: %d / drop: %d / pend: %d/%d / err: %v", s.Subject, delivered, dropped, pnum, psize, err)
	}
	resources.NatsConn, err = nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d/", config.NatsPort), nats.ErrorHandler(natsSlowLogFunc), nats.UserInfo(config.NatsUser, config.NatsPassword))
	if err != nil {
		resources.NatsServer.Shutdown()
		return nil, err
	}
	log.Info("nats connection established")

	jsonc, err := nats.NewEncodedConn(resources.NatsConn, nats.JSON_ENCODER)
	if err != nil {
		resources.NatsServer.Shutdown()
		return nil, err
	}
	log.Info("nats json connection established")

	// Resolver
	//
	resources.Resolver, err = NewResolver(ctx, resources, config)
	if err != nil {
		return nil, err
	}

	resolvChan := make(chan *IPResolv, 2000)
	err = resources.Resolver.StartWorkers(resolvChan)
	if err != nil {
		return nil, err
	}
	log.Info("resolver created")

	// Whitelist
	//
	resources.Whitelist, err = NewWhitelist(ctx, blockRecheckChan, config)
	if err != nil {
		log.Fatal(err)
	}
	log.Info("whitelist created")

	b := &Botex{
		config:            config,
		resources:         resources,
		natsSubscriptions: make([]*nats.Subscription, 0),
		plugins:           make([]Plugin, 0),
		ctx:               ctx,
	}

	// NATS subscriptions
	reqSubscription, err := jsonc.Subscribe(natsRequestsSubject, b.HandleRequest)
	if err != nil {
		resources.NatsServer.Shutdown()
		return nil, err
	}
	reqSubscription.SetPendingLimits(200000, 10*1024*1024*1024) // 200.000 messages or 10GB

	b.natsSubscriptions = append(b.natsSubscriptions, reqSubscription)
	log.Info("nats subscriptions done")

	// Block
	b.blocked = NewBlock(ctx, blockRecheckChan, blockASNNetChan, resources, config.BlockTTL)
	// clear blocked items
	if config.ClearBlocked {
		b.blocked.Clear()
	}

	// API
	//
	b.api, err = NewAPI(ctx, config, b)
	if err != nil {
		return nil, err
	}
	log.Info("API running")

	// Networks
	//
	if config.WithNetworks {
		networksPlugin := plugins.NewNetworks(ctx, config)
		b.Use(networksPlugin)

		log.Infof("networks enabled")
	}

	// History
	//
	b.history = NewHistory(ctx, b.plugins, b.resources, config)
	log.Info("history created")

	if config.LogMemoryStats {
		go b.logMemoryStats(ctx)
	}

	go b.resolvWorker(resolvChan)
	go b.blockWorker()
	go b.statsLogWorker()
	go b.blockASNNetWorker(blockASNNetChan)

	// clean up when we're done
	go func() {
		<-ctx.Done()
		err := b.resources.Store.Close()
		if err != nil {
			log.Warn(err)
		}
		close(resolvChan)
		for _, subscr := range b.natsSubscriptions {
			subscr.Drain()
		}
		b.resources.NatsServer.Shutdown()
		b.resources.NatsConn.Drain()
		b.resources.Store.Close()
	}()

	return b, nil
}

// Use adds a plugin to botex
func (b *Botex) Use(p Plugin) {
	p.SetBlocker(b.blocked)
	p.APIHooks(b.api.router)

	b.plugins = append(b.plugins, p)
}

func (b *Botex) logMemoryStats(ctx context.Context) {
	ticker := time.NewTicker(b.config.WindowSize)
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			ticker = nil
			return
		case <-ticker.C:
			if !b.config.LogMemoryStats {
				continue
			}
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			log.Infof("-=- alloc: %s, in_use: %s, objs: %s, idle: %s, released: %s, stack: %s, goroutines: %s, frees: %s",
				humanize.Bytes(m.Alloc),
				humanize.Bytes(m.HeapInuse),
				humanize.FormatInteger("#,###.", int(m.HeapObjects)),
				humanize.Bytes(m.HeapIdle),
				humanize.Bytes(m.HeapReleased),
				humanize.Bytes(m.StackInuse),
				humanize.FormatInteger("#,###.", runtime.NumGoroutine()),
				humanize.FormatInteger("#,###.", int(m.Frees)))
		}
	}
}

func (b *Botex) blockASNNetWorker(blockChan chan bool) {
	for {
		select {
		case <-b.ctx.Done():
			close(blockChan)
			return
		case <-blockChan:
			toBlock := make([]data.IPStats, 0)
			b.history.Each(func(ip string, ipd *IPData) {
				if !ipd.ForceBlock && (ipd.IsBlocked || !b.blocked.IsBlockedByASN(ipd.ASN)) {
					return
				}

				// log.Infof("netblock %s - %d (%s)", ipd.IP, ipd.ASN.ASN, ipd.ASN.Organization)

				toBlock = append(toBlock, data.IPStats{
					IP: ipd.IP,
					Stats: data.Stats{
						ASN:   ipd.ASN,
						Total: ipd.Total,
						App:   ipd.App,
						Other: ipd.Other,
						Ratio: ipd.Ratio,
					},
				})
			})

			b.history.update(true, toBlock...)
		}
	}
}

func (b *Botex) blockWorker() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case block := <-b.resources.BlockChan:
			if block.IsBlocked && !block.ForceBlock {
				continue
			}
			msg := data.IPBlockMessage{
				IP:       block.IP,
				Hostname: block.Hostname,
				BlockMessage: data.BlockMessage{
					ASN:       block.ASN,
					Reason:    block.BlockReason,
					BlockedAt: time.Now(),
					Stats: data.Stats{
						ASN:   block.ASN,
						Total: block.Total,
						App:   block.App,
						Other: block.Other,
						Ratio: block.Ratio,
					},
				},
			}
			if err := b.blocked.BlockIP(msg); err != nil {
				log.Errorf("failed to write blocked IP %s to store: %s", block.IP, err)
			}
			block.IsBlocked = true
		}
	}
}

func (b *Botex) resolvWorker(resolvChan chan *IPResolv) {
	count := 0

OUTER:
	for {
		select {
		case <-b.ctx.Done():
			return
		case rip := <-resolvChan:
			count++

			if rip == nil {
				log.Warn("rip is nil")
				continue OUTER
			}
			log.Tracef("[%d] ip %s resolved to %s", count, rip.IP, rip.Host)
			if rip.Err == "" {
				log.Tracef("resolved %s to %s [%d tries]", rip.IP, rip.Host, rip.Tries)
				b.history.SetHostname(rip.IP, rip.Host)
			} else {
				log.Tracef("failed to resolve %s: %s [%d tries]", rip.IP, rip.Err, rip.Tries)
			}
		}
	}
}

func (b *Botex) statsLogWorker() {
	ticker := time.NewTicker(10 * time.Second)
	log.Infof("starting statsLogWorker")
	for {
		select {
		case <-b.ctx.Done():
			ticker.Stop()
			ticker = nil
			return
		case <-ticker.C:
			numIPs := b.history.Size()
			stats := b.history.TotalStats()

			log.Infof("stats :: %d IPs / %d with hostname / %d blocked :: Requests %d Total / %d App / %d Other / %.2f Ratio", numIPs, stats.WithHostname, b.blocked.CountIPs(), stats.Total, stats.App, stats.Other, stats.Ratio)
		}
	}
}

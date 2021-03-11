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
	"github.com/scraperwall/geoip/v2"
	log "github.com/sirupsen/logrus"
)

const natsRequestsSubject = "requests"

// Botex detects bad bots
type Botex struct {
	natsSubscriptions []*nats.Subscription
	resolver          *Resolver
	history           *History
	networks          *Networks
	blocklist         *Blocklist
	config            *Config
	api               *API

	ctx context.Context
}

// HandleRequest handles incoming requests
func (b *Botex) HandleRequest(r *Request) {
	if r.Timestamp < 1<<32 { // seconds
		r.Time = time.Unix(r.Timestamp, 0)
	} else { // nanoseconds
		r.Time = time.Unix(0, r.Timestamp)
	}

	ip := net.ParseIP(r.Source)
	r.ASN = b.config.ASNDB.Lookup(ip)

	log.Tracef("received %s - %s", ip, r.URL)
	newIP := b.history.Add(r)
	log.Tracef("added %s to history", ip)

	if newIP {
		log.Tracef("enqueueing %s", ip)
		b.resolver.Enqueue(NewIPResolv(ip))
	}

	if b.config.WithNetworks {
		b.networks.HandleRequest(r)
	}

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
func New(ctx context.Context, config *Config) (*Botex, error) {
	var err error

	blocklistRecheckChan := make(chan bool)

	b := &Botex{
		config:            config,
		blocklist:         NewBlocklist(ctx, blocklistRecheckChan, config),
		natsSubscriptions: make([]*nats.Subscription, 0),
		ctx:               ctx,
	}

	b.config.BlockChan = make(chan *IPDetails, 100)

	// ASN Database
	//
	config.ASNDB, err = asndb.New(config.ASNDBFile)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("asndb loaded with %d records", config.ASNDB.Size())

	// GeoIP Database
	//
	config.GEOIPDB, err = geoip.New(config.GeoIPDBFile)
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

	config.NatsServer = natsd.New(nopts)
	go config.NatsServer.Start()
	if !config.NatsServer.ReadyForConnections(2 * time.Second) {
		config.NatsServer.Shutdown()
		return nil, errors.New("nats server failed to startup")
	}

	// NATS client
	//
	natsSlowLogFunc := func(c *nats.Conn, s *nats.Subscription, err error) {
		// limita, limitb, _ := s.PendingLimits()
		pnum, psize, _ := s.Pending()
		delivered, _ := s.Delivered()
		dropped, _ := s.Dropped()

		log.Warnf("nats error: %s del: %d / drop: %d / pend: %d/%d / err: %v", s.Subject, delivered, dropped, pnum, psize, err)
	}
	config.NatsConn, err = nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d/", config.NatsPort), nats.ErrorHandler(natsSlowLogFunc), nats.UserInfo(config.NatsUser, config.NatsPassword))
	if err != nil {
		config.NatsServer.Shutdown()
		return nil, err
	}

	jsonc, err := nats.NewEncodedConn(config.NatsConn, nats.JSON_ENCODER)
	if err != nil {
		b.config.NatsServer.Shutdown()
		return nil, err
	}

	reqSubscription, err := jsonc.Subscribe(natsRequestsSubject, b.HandleRequest)
	if err != nil {
		b.config.NatsServer.Shutdown()
		return nil, err
	}
	reqSubscription.SetPendingLimits(200000, 10*1024*1024*1024) // 200.000 messages or 10GB

	b.natsSubscriptions = append(b.natsSubscriptions, reqSubscription)

	// Badger
	//
	bopts := badger.DefaultOptions(config.BadgerPath)
	bopts.SyncWrites = true
	config.KVStore, err = NewBadgerDB(ctx, config.BadgerPath)
	if err != nil {
		return nil, err
	}

	// Whitelist
	//
	config.Whitelist, err = NewWhitelist(ctx, blocklistRecheckChan, config)
	if err != nil {
		log.Fatal(err)
	}

	// History
	//
	b.history = NewHistory(ctx, config)

	// Resolver
	//
	b.resolver, err = NewResolver(ctx, config)
	if err != nil {
		return nil, err
	}

	resolvChan := make(chan *IPResolv, 2000)
	err = b.resolver.StartWorkers(resolvChan)
	if err != nil {
		return nil, err
	}

	// Networks
	//
	if config.WithNetworks {
		log.Infof("enabling networka")
		b.networks = NewNetworks(ctx, config)
	}

	// API
	//
	b.api, err = NewAPI(ctx, config, b)
	if err != nil {
		return nil, err
	}

	if config.LogMemoryStats {
		go b.logMemoryStats(ctx)
	}

	go b.resolvWorker(resolvChan)
	go b.blockWorker()
	go b.statsLogWorker()

	// clean up when we're done
	go func() {
		<-ctx.Done()
		close(resolvChan)
		for _, subscr := range b.natsSubscriptions {
			subscr.Drain()
		}
		b.config.NatsServer.Shutdown()
		b.config.NatsConn.Drain()
		b.config.KVStore.Close()
	}()

	return b, nil
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

func (b *Botex) blockWorker() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case block := <-b.config.BlockChan:
			if err := b.blocklist.Block(block); err != nil {
				log.Errorf("failed to write blocked IP %s to kvstore: %s", block.IP, err)
			}
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

			log.Infof("stats :: %d IPs / %d with hostname / %d blocked :: Requests %d Total / %d App / %d Other / %.2f Ratio", numIPs, stats.WithHostname, b.blocklist.Count(), stats.Total, stats.App, stats.Other, stats.Ratio)
		}
	}
}

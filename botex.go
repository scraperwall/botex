package botex

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	natsd "github.com/nats-io/nats-server/v2/server"
	nats "github.com/nats-io/nats.go"
	"github.com/scraperwall/asndb"
	"github.com/scraperwall/geoip"
	log "github.com/sirupsen/logrus"
)

// Botex detects bad bots
type Botex struct {
	natsSubscriptions []*nats.Subscription
	asnDB             *asndb.ASNDB
	geoipDB           *geoip.GeoIP
	resolver          *Resolver
	history           *History
	blocklist         *Blocklist
	config            *Config

	ctx context.Context
}

// Idle does nothing
func (b *Botex) Idle() bool {
	return true
}

// HandleRequest handles incoming requests
func (b *Botex) HandleRequest(r *Request) {
	ip := net.ParseIP(r.Source)

	// TODO: is the URL or Host whitelisted?

	r.Time = time.Unix(r.Timestamp, 0)
	b.history.Add(r)
	b.resolver.Enqueue(NewIPResolv(ip))
	// fmt.Printf("%s - %s%s - %d - %s - %s\n", r.Source, r.Host, r.URL, asn.ASN, asn.Organization, geo.Country.Country)
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

	b := &Botex{
		config:            config,
		natsSubscriptions: make([]*nats.Subscription, 0),
		ctx:               ctx,
	}

	b.config.BlockChan = make(chan *IPDetails, 100)

	// ASN Database
	//
	config.ASNDB, err = asndb.New(config.StaticBaseURL)
	if err != nil {
		log.Fatal(err)
	}

	// GeoIP Database
	//
	config.GEOIPDB, err = geoip.NewGeoIP(config.StaticBaseURL)
	if err != nil {
		return nil, err
	}

	// NATS server
	//
	nopts := &natsd.Options{}
	nopts.CustomClientAuthentication = &natsAuth{
		User:     config.NatsUser,
		Password: config.NatsPassword,
	}
	nopts.HTTPPort = config.NatsHTTPPort
	nopts.Port = config.NatsPort

	config.NatsServer = natsd.New(nopts)
	go config.NatsServer.Start()
	if !config.NatsServer.ReadyForConnections(2 * time.Second) {
		config.NatsServer.Shutdown()
		return nil, errors.New("nats server failed to startup")
	}

	// NATS client
	//
	config.NatsConn, err = nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d/", config.NatsPort), nats.UserInfo(config.NatsUser, config.NatsPassword))
	if err != nil {
		config.NatsServer.Shutdown()
		return nil, err
	}

	jsonc, err := nats.NewEncodedConn(config.NatsConn, nats.JSON_ENCODER)
	if err != nil {
		b.config.NatsServer.Shutdown()
		return nil, err
	}

	reqSubscription, err := jsonc.Subscribe("requests", b.HandleRequest)
	if err != nil {
		b.config.NatsServer.Shutdown()
		return nil, err
	}

	b.natsSubscriptions = append(b.natsSubscriptions, reqSubscription)

	// Badger
	//
	bopts := badger.DefaultOptions(config.BadgerPath)
	bopts.SyncWrites = true
	config.KVStore, err = NewBadgerDB(ctx, config.BadgerPath)
	if err != nil {
		return nil, err
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

	resolvChan := make(chan *IPResolv)
	err = b.resolver.StartWorkers(resolvChan)
	if err != nil {
		return nil, err
	}

	go b.resolvWorker(resolvChan)
	go b.blockWorker()

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

func (b *Botex) blockWorker() {
	for {
		select {
		case <-b.ctx.Done():
			log.Trace("exiting block loop")
			break
		case block := <-b.config.BlockChan:
			// TODO: write blocked IP to badger
			log.Tracef("blocking %s  - %s (%s)", block.IP, block.Hostname, block.BlockReason)
			if err := b.blocklist.Block(block); err != nil {
				log.Errorf("failed to write blocked IP %s to kvstore: %s", block.IP, err)
			}
		}
	}
}

func (b *Botex) resolvWorker(resolvChan chan *IPResolv) {
	for {
		select {
		case <-b.ctx.Done():
			log.Trace("exiting resolv loop")
			break
		case rip := <-resolvChan:
			if rip.Err == "" {
				log.Tracef("resolved %s to %s [%d tries]\n", rip.IP, rip.Host, rip.Tries)
				b.history.SetHostname(rip.IP, rip.Host)
			} else {
				log.Tracef("failed to resolve %s: %s [%d tries]\n", rip.IP, rip.Err, rip.Tries)
			}
		}
	}
}

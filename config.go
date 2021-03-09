package botex

import (
	"time"

	natsd "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/geoip/v2"
)

// Config contains all configurable bits and pieces the botex application needs
// The configuration gets passed on to all parts of the application that need to access it
type Config struct {
	WindowSize      time.Duration
	NumWindows      int
	DNSServer       string
	BadgerPath      string
	NatsAddr        string
	NatsPort        int
	NatsHTTPPort    int
	NatsUser        string
	NatsPassword    string
	GeoIPDBFile     string
	ASNDBFile       string
	KeepRequests    int
	ResolverWorkers int
	ResolverTries   int
	ResolverTTL     time.Duration
	LogLevel        int
	BlockTTL        time.Duration
	MaxRatio        float64
	MinAppRequests  int
	MaxAppRequests  int
	LogReplay       string
	LogFormat       string
	APIAddress      string
	LogMemoryStats  bool
	WhitelistTOML   string

	ASNDB      *asndb.DB
	GEOIPDB    *geoip.DB
	NatsServer *natsd.Server
	NatsConn   *nats.Conn
	KVStore    KVStore
	Whitelist  *Whitelist

	BlockChan chan *IPDetails
}

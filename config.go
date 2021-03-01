package botex

import (
	"time"

	natsd "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/scraperwall/asndb"
	"github.com/scraperwall/geoip"
)

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
	StaticBaseURL   string
	KeepRequests    int
	ResolverWorkers int
	ResolverTries   int
	ResolverTTL     time.Duration
	LogLevel        int

	ASNDB      *asndb.ASNDB
	GEOIPDB    *geoip.GeoIP
	NatsServer *natsd.Server
	NatsConn   *nats.Conn
	KVStore    KVStore

	BlockChan chan *IPDetails
}

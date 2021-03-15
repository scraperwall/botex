package botex

import (
	natsd "github.com/nats-io/nats-server/v2/server"
	nats "github.com/nats-io/nats.go"
	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/botex/store"
	"github.com/scraperwall/geoip/v2"
)

type Resources struct {
	ASNDB      *asndb.DB
	GEOIPDB    *geoip.DB
	NatsServer *natsd.Server
	NatsConn   *nats.Conn
	KVStore    store.KVStore
	Whitelist  *Whitelist
	Resolver   *Resolver

	BlockChan chan *IPDetails
}

func NewResources() *Resources {
	return &Resources{}
}

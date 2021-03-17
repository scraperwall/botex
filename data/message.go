package data

import (
	"net"
	"time"

	"github.com/scraperwall/asndb/v2"
)

type BlockMessage struct {
	Stats
	Reason    string
	ASN       *asndb.ASN
	BlockedAt time.Time
}

type IPBlockMessage struct {
	BlockMessage
	IP       net.IP
	Hostname string
}

type NetworkBlockMessage struct {
	BlockMessage
	Network *net.IPNet
}

package data

import (
	"net"
	"time"

	"github.com/scraperwall/asndb/v2"
)

type BlockMessage struct {
	Stats
	Reason    string
	BlockedAt time.Time
}

type IPBlockMessage struct {
	BlockMessage
	IP net.IP
}

type NetworkBlockMessage struct {
	BlockMessage
	Network *net.IPNet
}

type ASNBlockMessage struct {
	BlockMessage
	ASN *asndb.ASN
}

package data

import (
	"net"
	"time"

	"github.com/scraperwall/geoip/v2"
)

type BlockMessage struct {
	Stats
	Reason    string      `json:"reason"`
	City      *geoip.City `json:"city"`
	BlockedAt time.Time   `json:"blocked_at"`
}

type IPBlockMessage struct {
	BlockMessage
	IP       net.IP `json:"ip"`
	Hostname string `json:"hostname"`
}

type NetworkBlockMessage struct {
	BlockMessage
	Network *net.IPNet `json:"network"`
}

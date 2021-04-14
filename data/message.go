package data

import (
	"net"
	"time"

	"github.com/scraperwall/geoip/v2"
)

type BlockMessage struct {
	Stats                 //`json:"stats"`
	Reason    string      `json:"reason"`
	City      *geoip.City `json:"city"`
	BlockedAt time.Time   `json:"blocked_at"`
}

type IPBlockMessage struct {
	BlockMessage        //`json:"blockmessage"`
	IP           net.IP `json:"ip"`
	Hostname     string `json:"hostname"`
}

type NetworkBlockMessage struct {
	BlockMessage            //`json:"blockmessage"`
	Network      *net.IPNet `json:"network"`
}

package data

import (
	"net"

	"github.com/scraperwall/asndb/v2"
)

// Stats contains aggregated statistics about a single IP
type Stats struct {
	Total int        `json:"total"`
	App   int        `json:"app"`
	Other int        `json:"other"`
	Ratio float64    `json:"ratio"`
	IPs   int        `json:"ips"`
	ASN   *asndb.ASN `json:"asn"`
}

type IPStats struct {
	Stats
	IP           net.IP `json:"ip"`
	WithHostname int    `json:"with_hostname"`
}

type NetworkStats struct {
	Stats
	Network      net.IPNet `json:"network"`
	NetworkSize  uint64    `json:"network_size"`
	NetworkRatio float64   `json:"network_ratio"`
}

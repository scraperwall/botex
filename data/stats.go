package data

import (
	"net"

	"github.com/scraperwall/asndb/v2"
)

// Stats contains aggregated statistics about a single IP
type Stats struct {
	Total int
	App   int
	Other int
	Ratio float64
}

type IPStats struct {
	Stats
	IP           net.IP
	WithHostname int
}

type NetworkStats struct {
	Stats
	Network      net.IPNet
	NetworkSize  uint64
	NetworkRatio float64
}

type ASNStats struct {
	Stats
	ASN *asndb.ASN
}

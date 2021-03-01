package botex

import (
	"context"
	"net"

	"github.com/scraperwall/asndb"
	"github.com/scraperwall/geoip"
)

type IPStats struct {
	Total int
	App   int
	Other int
	Ratio float64
}

type IPDetails struct {
	IP          net.IP       `json:"ip"`
	Hostname    string       `json:"hostname"`
	ASN         *asndb.ASN   `json:"asn"`
	GeoIP       *geoip.GeoIP `json:"geoip"`
	Total       int          `json:"total"`
	App         int          `json:"app"`
	Other       int          `json:"other"`
	Ratio       float64      `json:"ratio"`
	BlockReason string       `json:"blockreason"`
}

type IPData struct {
	IPDetails

	config *Config

	BotMinRequests int
	BotMinRatio    float64

	updateChan chan IPStats
	Requests   *Requests

	ctx context.Context
}

func NewIPData(ctx context.Context, ip net.IP, config *Config) *IPData {
	asn := config.ASNDB.Lookup(ip)
	geo, _ := config.GEOIPDB.LookupIP(ip.String())

	updateChan := make(chan IPStats)

	ipd := &IPData{
		IPDetails: IPDetails{
			IP:       ip,
			Hostname: "",
			ASN:      asn,
			GeoIP:    geo,
			Total:    0,
			App:      0,
			Other:    0,
			Ratio:    0.0,
		},
		config:         config,
		BotMinRequests: 10,
		BotMinRatio:    0.8,
		updateChan:     updateChan,
		Requests:       NewRequests(ctx, config, updateChan),
		ctx:            ctx,
	}

	go ipd.updateStats()

	return ipd
}

func (ipd *IPData) Add(r *Request) {
	ipd.Requests.Add(r)
}

func (ipd *IPData) SetHostname(hostname string) {
	ipd.Hostname = hostname
}

func (ipd *IPData) IsBot() bool {
	if ipd.Hostname == "" {
		return false
	}

	// TODO: is the IP/Hostname whitelisted?

	if ipd.Total > ipd.BotMinRequests &&
		ipd.BotMinRatio <= ipd.Ratio {
		return true
	}

	return false
}

func (ipd *IPData) updateStats() {
	for {
		select {
		case <-ipd.ctx.Done():
			close(ipd.updateChan)
			break
		case stats := <-ipd.updateChan:
			ipd.Total = stats.Total
			ipd.App = stats.App
			ipd.Other = stats.Other
			ipd.Ratio = stats.Ratio
		}
	}
}

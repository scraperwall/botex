package botex

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/botex/config"
	"github.com/scraperwall/botex/data"
	"github.com/scraperwall/geoip/v2"
	log "github.com/sirupsen/logrus"
)

// IPDetails contains meta information about an IP, its aggregated statistics and a reason for why it was blocked
type IPDetails struct {
	IP              net.IP       `json:"ip"`
	Hostname        string       `json:"hostname"`
	ASN             *asndb.ASN   `json:"asn"`
	GeoIP           *geoip.GeoIP `json:"geoip"`
	Total           int          `json:"total"`
	App             int          `json:"app"`
	Other           int          `json:"other"`
	Ratio           float64      `json:"ratio"`
	IsBlocked       bool         `json:"isblocked"`
	BlockReason     string       `json:"blockreason"`
	Whitelisted     bool         `json:"whitelisted"`
	WhitelistReason string       `json:"whitelistreason"`
	CreatedAt       time.Time    `json:"createdat"`
	UpdatedAt       time.Time    `json:"updatedat"`
	LastBlockAt     time.Time    `json:"lastblockat"`
	ForceBlock      bool         `json:"-"`
}

// IPData contains IPDetails and the most recent HTTP requests.
// It handles updating the aggregated stats when it receives new requests
type IPData struct {
	IPDetails
	Requests     *Requests
	ipUpdateChan chan data.IPStats
	plugins      []Plugin

	resources *Resources
	config    *config.Config
	mutex     sync.RWMutex
}

// NewIPData creates a new IPData item fro a given IP.
// the parent context and app configuration are passed on from the parent
func NewIPData(updateChan chan data.IPStats, ip net.IP, plugins []Plugin, resources *Resources, config *config.Config) *IPData {
	asn := resources.ASNDB.Lookup(ip)
	geo, _ := resources.GEOIPDB.Lookup(ip)

	ipd := &IPData{
		IPDetails: IPDetails{
			IP:        ip,
			Hostname:  "",
			ASN:       asn,
			GeoIP:     geo,
			Total:     0,
			App:       0,
			Other:     0,
			Ratio:     0.0,
			CreatedAt: time.Now(),
			UpdatedAt: time.Unix(0, 0),
		},
		resources:    resources,
		config:       config,
		ipUpdateChan: updateChan,
		plugins:      plugins,
		mutex:        sync.RWMutex{},
		Requests:     NewRequests(ip, updateChan, config),
	}

	return ipd
}

// Add adds a single HTTP request
func (ipd *IPData) Add(r *data.Request) {
	ipd.UpdatedAt = time.Now()
	ipd.Requests.Add(r)
}

// Update sets the cached statistics numbers using an IPStats item
func (ipd *IPData) Update(stats data.IPStats, force bool) {
	ipd.Total = stats.Total
	ipd.App = stats.App
	ipd.Other = stats.Other
	ipd.Ratio = stats.Ratio

	if now := time.Now(); ipd.ShouldBeBlocked() && (force || ipd.LastBlockAt.Add(ipd.config.BlockTTL).Before(now)) {
		if force {
			ipd.ForceBlock = true
		}
		ipd.LastBlockAt = now
		ipd.resources.BlockChan <- &ipd.IPDetails
	}
}

// SetHostname sets the reverse hostname for an IP
func (ipd *IPData) SetHostname(hostname string) {
	if ipd == nil {
		log.Warn("ipd is nil")
		return
	}
	ipd.Hostname = hostname
	ipd.ipUpdateChan <- data.IPStats{
		Stats: data.Stats{
			ASN: ipd.ASN,
		},
		IP: ipd.IP,
	}
}

// ShouldBeBlocked deterines whether the IP represented by this item should be blocked
func (ipd *IPData) ShouldBeBlocked() bool {

	decision := false

	// don't block while the IP is being resolved or if it is whitelisted
	if ipd.Hostname == "resolving" || ipd.Whitelisted {
		return false
	}

	// don't block unless we've seen new requests in the current window
	threshold := time.Now().Add(-1 * ipd.config.WindowSize)
	if ipd.UpdatedAt.Before(threshold) && ipd.CreatedAt.Before(threshold) {
		return false
	}

	// Is the IP whitelisted?
	if wl, descr := ipd.resources.Whitelist.IsWhitelisted(&ipd.IPDetails); wl {
		ipd.Whitelisted = true
		ipd.WhitelistReason = descr
		return false
	}

	if ipd.App > ipd.config.MaxAppRequests {
		ipd.BlockReason = fmt.Sprintf("too many requests (%d/%d)", ipd.App, ipd.config.MaxAppRequests)
		decision = true
		goto RESOLVE
	}

	if ipd.App > ipd.config.MinAppRequests &&
		ipd.config.MaxRatio <= ipd.Ratio {
		ipd.BlockReason = fmt.Sprintf("too many requests (%d/%d) and app/asset ratio too high (%.2f/%0.2f)", ipd.App, ipd.config.MinAppRequests, ipd.Ratio, ipd.config.MaxRatio)
		decision = true
		goto RESOLVE
	}

	for _, plugin := range ipd.plugins {
		if plugin.ShouldBeBlocked(ipd.IPStats()) {
			decision = true
			// log.Infof("blocked by network: %s", ipd.IP)
			ipd.BlockReason = "blocked by ASN/network"
			goto RESOLVE
		}
	}

RESOLVE:
	if decision == true && ipd.Hostname == "" {
		ipd.Hostname = "resolving"
		ipd.resources.Resolver.Resolve(NewIPResolv(ipd.IP))
		return false
	}

	return decision
}

// Expire removes all expired requests
func (ipd *IPData) Expire() int {
	return ipd.Requests.Expire()
}

func (ipd *IPData) Stats() data.Stats {
	return data.Stats{
		ASN:   ipd.ASN,
		Total: ipd.Total,
		App:   ipd.App,
		Other: ipd.Other,
		Ratio: ipd.Ratio,
	}
}

func (ipd *IPData) IPStats() data.IPStats {
	withHostname := 0
	if ipd.Hostname != "" && ipd.Hostname != "resolving" {
		withHostname = 1
	}
	return data.IPStats{
		Stats:        ipd.Stats(),
		IP:           ipd.IP,
		WithHostname: withHostname,
	}
}

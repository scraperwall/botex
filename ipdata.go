package botex

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/geoip/v2"
	log "github.com/sirupsen/logrus"
)

// IPStats contains aggregated statistics about a single IP
type IPStats struct {
	Total        int
	App          int
	Other        int
	Ratio        float64
	WithHostname int
}

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
}

// IPData contains IPDetails and the most recent HTTP requests.
// It handles updating the aggregated stats when it receives new requests
type IPData struct {
	IPDetails

	config *Config

	updateChan chan IPStats
	removeChan chan net.IP
	Requests   *Requests

	mutex  sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewIPData creates a new IPData item fro a given IP.
// the parent context and app configuration are passed on from the parent
func NewIPData(ctx context.Context, ip net.IP, removeChan chan net.IP, config *Config) *IPData {
	asn := config.ASNDB.Lookup(ip)
	geo, _ := config.GEOIPDB.Lookup(ip)

	updateChan := make(chan IPStats)

	myctx, mycancel := context.WithCancel(ctx)

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
		config:     config,
		updateChan: updateChan,
		removeChan: removeChan,
		mutex:      sync.RWMutex{},
		Requests:   NewRequests(ctx, config, updateChan),
		ctx:        myctx,
		cancel:     mycancel,
	}

	/*
		go func() {
			for range time.Tick(3 * time.Second) {
				log.Infof("%s - %s :: total %d / app %d / oher %d / ratio %.2f", ipd.IP, ipd.Hostname, ipd.Total, ipd.App, ipd.Other, ipd.Ratio)
			}
		}()
	*/
	go ipd.updateStats()

	return ipd
}

// Add adds a single HTTP request
func (ipd *IPData) Add(r *Request) {
	// log.Infof("+ %s - %s%s - %d - %s - %s\n", r.Source, r.Host, r.URL, ipd.ASN.ASN, ipd.ASN.Organization, ipd.GeoIP.Country.Country)
	ipd.UpdatedAt = time.Now()
	ipd.Requests.Add(r)
}

// SetHostname sets the reverse hostname for an IP
func (ipd *IPData) SetHostname(hostname string) {
	if ipd == nil {
		log.Warn("ipd is nil")
		return
	}
	ipd.mutex.Lock()
	ipd.Hostname = hostname
	ipd.mutex.Unlock()
	ipd.updateChan <- IPStats{
		Total: ipd.Total,
		App:   ipd.App,
		Other: ipd.Other,
		Ratio: ipd.Ratio,
	}
}

// ShouldBeBlocked deterines whether the IP represented by this item should be blocked
func (ipd *IPData) ShouldBeBlocked() bool {
	log.Tracef("%s (%s) total: %d/%d/%d, ratio: %.2f/%.2f", ipd.IP, ipd.Hostname, ipd.Total, ipd.config.MinAppRequests, ipd.config.MaxAppRequests, ipd.Ratio, ipd.config.MaxRatio)

	if ipd.Hostname == "" || ipd.Whitelisted {
		return false
	}

	// don't block unless we've seen new requests in the current window
	threshold := time.Now().Add(-1 * ipd.config.WindowSize)
	if ipd.UpdatedAt.Before(threshold) && ipd.CreatedAt.Before(threshold) {
		return false
	}

	// Is the IP whitelisted?
	if wl, descr := ipd.config.Whitelist.IsWhitelisted(&ipd.IPDetails); wl {
		ipd.Whitelisted = true
		ipd.WhitelistReason = descr
		return false
	}

	if ipd.Total > ipd.config.MaxAppRequests {
		ipd.BlockReason = fmt.Sprintf("too many requests (%d/%d)", ipd.Total, ipd.config.MaxAppRequests)
		return true
	}

	if ipd.Total > ipd.config.MinAppRequests &&
		ipd.config.MaxRatio <= ipd.Ratio {
		ipd.BlockReason = fmt.Sprintf("too many requests (%d/%d) and app/asset ratio too high (%.2f/%0.2f)", ipd.Total, ipd.config.MinAppRequests, ipd.Ratio, ipd.config.MaxRatio)
		return true
	}

	return false
}

// Stop signals that this intance is no longer needed an can self-destruct
func (ipd *IPData) Stop() {
	ipd.cancel()
}

func (ipd *IPData) updateStats() {
	for {
		select {
		case <-ipd.ctx.Done():
			ipd.IP = nil
			ipd.ASN = nil
			ipd.GeoIP = nil
			return
		case stats := <-ipd.updateChan:
			ipd.Total = stats.Total
			ipd.App = stats.App
			ipd.Other = stats.Other
			ipd.Ratio = stats.Ratio

			if ipd.Total <= 0 && ipd.CreatedAt.Add(ipd.config.WindowSize).Before(time.Now()) {
				ipd.Requests.Stop()
				//ipd.Requests = nil
				//close(ipd.updateChan)
				ipd.removeChan <- ipd.IP
			}

			if ipd.ShouldBeBlocked() {
				now := time.Now()
				if ipd.LastBlockAt.Add(ipd.config.BlockTTL).Before(now) {
					ipd.LastBlockAt = time.Now()
					ipd.config.BlockChan <- &ipd.IPDetails
				}
			}
		}
	}
}

package botex

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/scraperwall/botex/config"
	"github.com/scraperwall/botex/data"
	log "github.com/sirupsen/logrus"
)

// History is the history of all IPs for which the application has received a request
type History struct {
	config       *config.Config
	resources    *Resources
	data         map[string]*IPData
	plugins      []Plugin
	mutex        sync.RWMutex
	windowSize   time.Duration
	numWindows   int
	ipUpdateChan chan data.IPStats
}

// NewHistory creates a new History item and passes on the context and configuration from its parent
func NewHistory(ctx context.Context, plugins []Plugin, resources *Resources, config *config.Config) *History {
	h := History{
		config:       config,
		resources:    resources,
		data:         make(map[string]*IPData),
		plugins:      plugins,
		windowSize:   config.WindowSize,
		numWindows:   config.NumWindows,
		ipUpdateChan: make(chan data.IPStats),
		mutex:        sync.RWMutex{},
	}

	go func() {
		ticker := time.NewTicker(config.WindowSize)

		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				ticker = nil
				return
			case <-ticker.C:
				h.expire()
			case stats := <-h.ipUpdateChan:
				// log.Infof("received IPStats for %s - total: %d, app: %d, other: %d, ratio: %.2f", stats.IP, stats.Total, stats.App, stats.Other, stats.Ratio)
				h.update(false, stats)
			}
		}
	}()

	return &h
}

func (h *History) expire() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	for ip, ipd := range h.data {
		if ipd.Requests.CanBeExpired() && ipd.Expire() <= 0 {
			log.Tracef("IPData for %s is empty. Removing it.", ip)
			delete(h.data, ip)
		}
	}
}

// update updates the cached stats for an IP
func (h *History) update(force bool, stats ...data.IPStats) {
	for _, s := range stats {
		ipstr := s.IP.String()
		h.mutex.RLock()
		ipd, ok := h.data[ipstr]
		h.mutex.RUnlock()

		if !ok {
			log.Warnf("%s, %d (%s) is not in history", ipstr, s.ASN.ASN, s.ASN.Organization)
			return
		}

		if s.Total <= 0 {
			log.Warnf("%s - %d %s has no data", ipstr, s.ASN.ASN, s.ASN.Organization)
			return
		}
		ipd.Update(s, force)
	}
}

func (h *History) Each(callback func(key string, ipd *IPData)) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for k, i := range h.data {
		callback(k, i)
	}
}

// Add adds a single HTTP request to the history
// If Add has added a new item to the data map it returns true, otherwise it returns false
func (h *History) Add(r *data.Request) bool {
	newIP := false

	ip := net.ParseIP(r.Source)
	ipstr := ip.String()

	h.mutex.Lock()
	if _, ok := h.data[ipstr]; !ok {
		h.data[ipstr] = NewIPData(h.ipUpdateChan, ip, h.plugins, h.resources, h.config)
		newIP = true
	}
	h.mutex.Unlock()

	log.Tracef("history adding %s - %s", time.Now().Sub(r.Time), r.URL)

	h.mutex.RLock()
	ipd, ok := h.data[ipstr]
	h.mutex.RUnlock()
	if ok {
		ipd.Add(r)
	}

	log.Tracef("history added %s %s", ipstr, r.URL)

	return newIP
}

// SetHostname sets the reverse hotname for a given IP
func (h *History) SetHostname(ip net.IP, hostname string) {
	log.Tracef("SetHostname %s %s", ip, hostname)
	h.mutex.RLock()
	ipd, ok := h.data[ip.String()]
	h.mutex.RUnlock()

	if ok {
		ipd.SetHostname(hostname)
	} else {
		log.Warnf("history ipdata for %s (%s) is nil", ip, hostname)
	}
}

// Size returns the number of IPs in the history
func (h *History) Size() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	l := len(h.data)

	return l
}

// TotalStats returns the sum of the stats for all IPs
func (h *History) TotalStats() data.IPStats {
	stats := data.IPStats{}

	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for _, ipd := range h.data {
		stats.Total += ipd.Total
		stats.App += ipd.App
		stats.Other += ipd.Other
		if ipd.Hostname != "" {
			stats.WithHostname++
		}
	}

	stats.Ratio = float64(stats.App) / float64(stats.Total)

	return stats
}

// IPDetails returns the IPDetails for the given IP
func (h *History) IPDetails(ip net.IP) *IPDetails {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	ipd := &h.data[ip.String()].IPDetails

	return ipd
}

// IPData returns the IPData struct for the given IP
func (h *History) IPData(ip net.IP) *IPData {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	ipd := h.data[ip.String()]

	return ipd
}

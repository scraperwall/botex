package botex

import (
	"context"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// History is the history of all IPs for which the application has received a request
type History struct {
	config     *Config
	data       map[string]*IPData
	mutex      sync.RWMutex
	windowSize time.Duration
	numWindows int
	removeChan chan net.IP
	ctx        context.Context
}

// NewHistory creates a new History item and passes on the context and configuration from its parent
func NewHistory(ctx context.Context, config *Config) *History {
	h := History{
		config:     config,
		data:       make(map[string]*IPData),
		windowSize: config.WindowSize,
		numWindows: config.NumWindows,
		removeChan: make(chan net.IP),
		mutex:      sync.RWMutex{},
		ctx:        ctx,
	}

	go h.removeEmptyIPs()

	return &h
}

func (h *History) removeEmptyIPs() {
	for {
		select {
		case <-h.ctx.Done():
			close(h.removeChan)
			return
		case ip := <-h.removeChan:
			h.mutex.Lock()
			log.Tracef("removing %s from history", ip)
			h.data[ip.String()].Stop()
			h.data[ip.String()] = nil
			delete(h.data, ip.String())
			h.mutex.Unlock()
		}
	}
}

// Add adds a single HTTP request to the history
// If Add has added a new item to the data map it returns true, otherwise it returns false
func (h *History) Add(r *Request) bool {
	newIP := false
	h.mutex.Lock()
	defer h.mutex.Unlock()

	ip := net.ParseIP(r.Source)
	ipstr := ip.String()

	if _, ok := h.data[ip.String()]; !ok {
		h.data[ipstr] = NewIPData(h.ctx, ip, h.removeChan, h.config)
		newIP = true
	}

	log.Tracef("%s - %s", time.Now().Sub(r.Time), r.URL)
	h.data[ipstr].Add(r)

	return newIP
}

// SetHostname sets the reverse hotname for a given IP
func (h *History) SetHostname(ip net.IP, hostname string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if ipd, ok := h.data[ip.String()]; ok {
		ipd.SetHostname(hostname)
	} else {
		log.Warnf("history ipdata for %s (%s) is nil", ip, hostname)
	}
}

// Size returns the number of IPs in the history
func (h *History) Size() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return len(h.data)
}

// TotalStats returns the sum of the stats for all IPs
func (h *History) TotalStats() IPStats {
	stats := IPStats{}

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

	return &h.data[ip.String()].IPDetails
}

// IPData returns the IPData struct for the given IP
func (h *History) IPData(ip net.IP) *IPData {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return h.data[ip.String()]
}

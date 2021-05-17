/*
	botex - a bad bot mitigation tool by ScraperWall
	Copyright (C) 2021 ScraperWall, Tobias von Dewitz <tobias@scraperwall.com>

	This program is free software: you can redistribute it and/or modify it
	under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or (at your
	option) any later version.

	This program is distributed in the hope that it will be useful, but WITHOUT
	ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
	FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License
	for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

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

type Decision int

const (
	IsBlocked Decision = iota
	IsHuman
	IsWhitelisted
)

// History is the history of all IPs for which the application has received a request
type History struct {
	config       *config.Config
	resources    *Resources
	data         map[string]*IPData
	stats        *StatsWindows
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
		stats:        NewStatsWindows(resources, config),
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
				h.update(false, stats)
			}
		}
	}()

	return &h
}

func (h *History) expire() {
	go h.stats.Expire()

	h.mutex.Lock()
	defer h.mutex.Unlock()

	for ip, ipd := range h.data {
		if !ipd.Requests.CanBeExpired() {
			continue
		}

		numExpired := ipd.Expire()

		if numExpired <= 0 {
			log.Tracef("IPData for %s is empty. Removing it.", ip)
			delete(h.data, ip)
			h.resources.WebsocketChan <- map[string]interface{}{
				"type":   "ExpiredIP",
				"action": "expireIP",
				"data":   ip,
			}
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

		if !ok || s.Total <= 0 {
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
func (h *History) Add(r *data.Request) (ipd *IPData, newIP bool) {
	newIP = false

	ip := net.ParseIP(r.Source)
	if ip == nil {
		log.Warnf("IP %s failed to parse", r.Source)
		return nil, false
	}

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
		decision := ipd.Add(r)
		err := h.stats.Add(decision, r.Time)
		if err != nil {
			log.Warnf("failed to add request to stats: %s", err)
		}
	}

	log.Tracef("history added %s %s", ipstr, r.URL)

	return ipd, newIP
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

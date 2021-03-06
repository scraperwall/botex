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

package plugins

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/emirpasic/gods/maps/treebidimap"
	"github.com/emirpasic/gods/maps/treemap"
	"github.com/emirpasic/gods/utils"
	"github.com/gin-gonic/gin"
	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/botex/config"
	"github.com/scraperwall/botex/data"
	"github.com/scraperwall/botex/matchers"
	log "github.com/sirupsen/logrus"
)

type NetworkStats struct {
	data.Stats
}

// Networks contains requests statistics for all networks
type Networks struct {
	data       map[string]*NetworkData
	asnData    map[int]*data.Stats
	asnAverage int
	netData    map[string]*data.Stats
	netAverage int
	mutex      sync.RWMutex
	blocker    data.Blocker
	removeChan chan *net.IPNet
	windowSize time.Duration
	numWindows int
	config     *config.Config
	ctx        context.Context
}

// NetworkData contains request statistics for one network
type NetworkData struct {
	Network   *net.IPNet `json:"network"`
	Total     int        `json:"total"`
	App       int        `json:"app"`
	Other     int        `json:"other"`
	Ratio     float64    `json:"ratio"`
	IPCount   int        `json:"ipcount"`
	UpdatedAt time.Time  `json:"updatedat"`
	ASN       *asndb.ASN `json:"asn"`

	totalMap *treemap.Map
	appMap   *treemap.Map
	otherMap *treemap.Map
	ipMap    *treebidimap.Map

	removeChan  chan *net.IPNet
	mutex       sync.RWMutex
	windowSize  time.Duration
	numWindows  int
	expireTimer *time.Timer
	ctx         context.Context
}

// NewNetworkData creates a new NetworkData instance
func NewNetworkData(ctx context.Context, ipnet *net.IPNet, removeChan chan *net.IPNet, config *config.Config) *NetworkData {
	nd := &NetworkData{
		Network:     ipnet,
		Total:       0,
		App:         0,
		Other:       0,
		Ratio:       0.0,
		totalMap:    treemap.NewWithIntComparator(),
		appMap:      treemap.NewWithIntComparator(),
		otherMap:    treemap.NewWithIntComparator(),
		ipMap:       treebidimap.NewWith(utils.IntComparator, utils.StringComparator),
		mutex:       sync.RWMutex{},
		windowSize:  config.WindowSize,
		numWindows:  config.NumWindows,
		removeChan:  removeChan,
		UpdatedAt:   time.Now(),
		expireTimer: time.NewTimer(config.WindowSize),
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				nd.expireTimer.Stop()
				nd.expireTimer = nil

				nd.totalMap = nil
				nd.appMap = nil
				nd.otherMap = nil
				return
			case <-nd.expireTimer.C:
				nd.removeChan <- nd.Network
				return
			}
		}
	}()

	return nd
}

// HandleRequest adds an item to the NetworkData instance
func (nd *NetworkData) HandleRequest(r *data.Request) {
	if nd.ASN == nil {
		nd.ASN = r.ASN
	}

	key := nd.keyFor(r.Time)
	var val int64

	// update stats
	//
	stats := nd.appMap
	if matchers.Assets.MatchString(r.URL) {
		stats = nd.otherMap
	}

	nd.mutex.Lock()
	if v, ok := stats.Get(key); ok {
		val = v.(int64)
	}

	val++

	stats.Put(key, val)

	val = 0
	if total, ok := nd.totalMap.Get(key); ok {
		val = total.(int64)
	}

	val++
	nd.totalMap.Put(key, val)
	nd.mutex.Unlock()

	nd.updateStats()
	nd.UpdatedAt = time.Now()

	// update IPs: remove the IP in case it exists and insert it with the request's time
	ipstr := net.ParseIP(r.Source).String()
	if key, ok := nd.ipMap.GetKey(ipstr); ok {
		nd.ipMap.Remove(key)
	}
	nd.ipMap.Put(int(r.Time.UnixNano()), ipstr)

	// reset the expiration timer
	//
	nd.mutex.Lock()
	defer nd.mutex.Unlock()

	if nd.expireTimer != nil {
		nd.expireTimer.Reset(r.Time.Sub(time.Now()) + nd.windowSize*time.Duration(nd.numWindows))
	} else {
		nd.expireTimer = time.NewTimer(r.Time.Sub(time.Now()) + nd.windowSize*time.Duration(nd.numWindows))
	}
}

func (nd *NetworkData) NumberOfIPs() uint32 {
	if len(nd.ASN.Network.Mask) <= 0 {
		return 0
	}

	ones := make([]byte, 4)
	pos := 3

	l := len(nd.ASN.Network.Mask)
	for i := l - 1; i > l-4; i-- {
		ones[pos] = ^nd.ASN.Network.Mask[i]
		pos--
	}

	return binary.BigEndian.Uint32(ones)
}

func (nd *NetworkData) updateStats() {
	nd.updateCount(&nd.Total, nd.totalMap)
	nd.updateCount(&nd.App, nd.appMap)
	nd.updateCount(&nd.Other, nd.otherMap)
	if nd.Total > 0 {
		nd.Ratio = float64(nd.App) / float64(nd.Total)
	}

	nd.mutex.RLock()
	defer nd.mutex.RUnlock()
	nd.IPCount = nd.ipMap.Size()
}

// Total returns the total number of requests received from a network during the time window
func (nd *NetworkData) updateCount(field *int, stats *treemap.Map) {

	if nd == nil || stats == nil {
		return
	}

	var total int

	nd.mutex.Lock()
	defer nd.mutex.Unlock()

	iter := stats.Iterator()
	for iter.Next() {
		total += int(iter.Value().(int64))
	}

	*field = total
}

func (nd *NetworkData) keyFor(t time.Time) int {
	return int(t.UnixNano() - t.UnixNano()%nd.windowSize.Nanoseconds())
}

func (nd *NetworkData) expireIPs() int {
	threshold := int(time.Now().Add(-1 * nd.windowSize * time.Duration(nd.numWindows)).UnixNano())

	nd.mutex.Lock()
	defer nd.mutex.Unlock()

	iter := nd.ipMap.Iterator()
	numRemoved := 0

	for iter.Next() {
		key := iter.Key().(int)
		if key >= threshold {
			break
		}
		nd.ipMap.Remove(key)
		numRemoved++
	}

	return numRemoved
}

func (nd *NetworkData) expireStats(stats *treemap.Map) int {
	threshold := int(time.Now().Add(-1 * nd.windowSize * time.Duration(nd.numWindows)).UnixNano())

	nd.mutex.Lock()
	defer nd.mutex.Unlock()

	iter := stats.Iterator()
	numRemoved := 0

	for iter.Next() {
		key := iter.Key().(int)
		if key >= threshold {
			break
		}
		stats.Remove(key)
		numRemoved++
	}

	return numRemoved
}

func (nd *NetworkData) expire() {
	numRemoved := 0
	numRemoved += nd.expireStats(nd.totalMap)
	numRemoved += nd.expireStats(nd.appMap)
	numRemoved += nd.expireStats(nd.otherMap)

	nd.expireIPs()

	if numRemoved > 0 {
		nd.updateStats()
	}

}

// NewNetworks creates a new Networks instance
func NewNetworks(ctx context.Context, config *config.Config) *Networks {
	n := Networks{
		data:       make(map[string]*NetworkData),
		mutex:      sync.RWMutex{},
		windowSize: config.WindowSize,
		numWindows: config.NumWindows,
		removeChan: make(chan *net.IPNet),
		config:     config,
		ctx:        ctx,
	}

	go func() {
		ticker := time.NewTicker(config.WindowSize)

		for {
			select {
			case <-ctx.Done():
				n.mutex.Lock()
				n.data = nil
				ticker.Stop()
				ticker = nil
				close(n.removeChan)
				n.mutex.Unlock()
				return
			case network := <-n.removeChan:
				n.Remove(network)
			case <-ticker.C:
				n.updateAndBlock()
				n.logStats()
			}
		}
	}()

	return &n
}

// Remove deletes a network from the list of networks, e.g. when all its requests have expired
func (n *Networks) Remove(network *net.IPNet) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	delete(n.data, network.String())
}

// HandleRequest handles a request and saves it into the network data
func (n *Networks) HandleRequest(r *data.Request) {
	log.Tracef("networks adding %s - %s (%d)", r.Source, r.ASN.Network, len(n.data))

	cidr := r.ASN.Network.String()

	n.mutex.Lock()
	if _, ok := n.data[cidr]; !ok {
		n.data[cidr] = NewNetworkData(n.ctx, r.ASN.Network, n.removeChan, n.config)
	}

	n.data[cidr].HandleRequest(r)
	n.mutex.Unlock()
}

// Count returns the number of networks
func (n *Networks) Count() int {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	return len(n.data)
}

// All returns data about all networks it currently holds
func (n *Networks) All() []*NetworkData {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	res := make([]*NetworkData, len(n.data))
	i := 0

	for _, nd := range n.data {
		res[i] = nd
		i++
	}

	return res
}

// NetworkStats returns the metadata and statistics for one network
func (n *Networks) NetworkStats(ipn *net.IPNet) (nd *NetworkData, found bool) {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	nd, found = n.data[ipn.String()]

	return nd, found
}

// ASNStats returns the metadata and statistics for an autonomous system
func (n *Networks) ASNStats(asn int) (st *data.Stats, found bool) {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	stats, found := n.asnData[asn]

	return stats, found
}

// ShouldBeBlocked decides whether an IP should be blocked based on its network and/or ASN being blocked
func (n *Networks) ShouldBeBlocked(stats data.IPStats) (blocked bool, reason string) {
	return n.blocker.IsBlockedByASN(stats.ASN)
}

// Averages returns the average of requests across all networks
func (n *Networks) Averages() data.NetworkStats {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	res := data.NetworkStats{}

	count := 0
	ncount := 0.0

	for _, nd := range n.data {
		res.Total += nd.Total
		res.App += nd.App
		res.Other += nd.Other
		res.NetworkSize += uint64(nd.NumberOfIPs())

		if nd.NumberOfIPs() > 0 {
			ratio := (float64(nd.Total) / float64(nd.NumberOfIPs()))
			res.NetworkRatio += ratio
			ncount++
		}
		count++
	}

	if count > 0 {
		res.Total = res.Total / count
		res.App = res.App / count
		res.Other = res.Other / count
		res.Ratio = float64(res.App) / float64(res.Total)
		res.NetworkSize = res.NetworkSize / uint64(count)
		res.NetworkRatio = res.NetworkRatio / ncount
	}

	return res
}

func (n *Networks) APIHooks(r *gin.Engine) {
	if r == nil {
		log.Fatal("gin router is nil")
	}
	r.GET("/network/:ip/:bits", n.apiGetNetwork)
	r.GET("/blocked/networks", n.apiGetBlockedNetworks)
	r.GET("/blocked/asns", n.apiGetBlockedASNs)
	r.GET("/networks", n.apiGetNetworks)
	r.GET("/stats/asn/:asn", n.apiGetASNStats)
}

// IsWhitelisted determines whether an IP is whitelisted
// This plugin doesn't whitelist and always returns false
func (n *Networks) IsWhitelisted(ip net.IP) bool {
	return false
}

// SetBlocker sets the instance through which it can block networks
func (n *Networks) SetBlocker(b data.Blocker) {
	n.blocker = b
}

func (n *Networks) updateAndBlock() {

	asns := make(map[int]*data.Stats)
	nets := make(map[string]*data.Stats)
	var total int

	requestLimit := n.config.MaxAppRequests

	asnCounts := make(map[int]int)
	netCounts := make(map[string]int)

	n.mutex.RLock()
	for _, nd := range n.data {
		nd.expire()

		total += nd.Total

		stats, exists := asns[nd.ASN.ASN]
		if !exists {
			asns[nd.ASN.ASN] = &data.Stats{
				ASN: nd.ASN,
			}
			stats = asns[nd.ASN.ASN]
			asnCounts[nd.ASN.ASN] = 0
		}
		stats.Total += nd.Total
		stats.App += nd.App
		stats.Other += nd.Other
		stats.Ratio += nd.Ratio
		stats.IPs = nd.IPCount
		asnCounts[nd.ASN.ASN]++

		stats, exists = nets[nd.ASN.Cidr]
		if !exists {
			nets[nd.ASN.Cidr] = &data.Stats{
				ASN: nd.ASN,
			}
			stats = nets[nd.ASN.Cidr]
			netCounts[nd.ASN.Cidr] = 0
		}
		stats.Total += nd.Total
		stats.App += nd.App
		stats.Other += nd.Other
		stats.Ratio += nd.Ratio
		stats.IPs = nd.IPCount
		netCounts[nd.ASN.Cidr]++
	}
	n.mutex.RUnlock()

	numBlocked := 0

	for asn, stats := range asns {
		c := asnCounts[asn]
		if stats.Total > requestLimit && stats.Ratio/float64(c) > n.config.MaxRatio {
			numBlocked++
			n.blocker.BlockASN(data.BlockMessage{
				BlockedAt: time.Now(),
				Reason:    fmt.Sprintf("asn has too many requests (%d/%d) and ratio is too high (%.2f/%.2f)", stats.Total, requestLimit, stats.Ratio/float64(c), n.config.MaxRatio),
				Stats: data.Stats{
					ASN:   stats.ASN,
					Total: stats.Total,
					App:   stats.App,
					Other: stats.Other,
					IPs:   stats.IPs,
					Ratio: stats.Ratio / float64(c),
				},
			})
		}
	}

	for network, stats := range nets {
		c := netCounts[network]

		if stats.Total > requestLimit && stats.Ratio/float64(c) > n.config.MaxRatio {
			numBlocked++
			n.blocker.BlockNetwork(data.NetworkBlockMessage{
				Network: stats.ASN.Network,
				BlockMessage: data.BlockMessage{
					BlockedAt: time.Now(),
					Reason:    fmt.Sprintf("network has too many requests (%d/%d) and ratio is too high (%.2f/%.2f)", stats.Total, requestLimit, stats.Ratio/float64(c), n.config.MaxRatio),
					Stats: data.Stats{
						ASN:   stats.ASN,
						Total: stats.Total,
						App:   stats.App,
						Other: stats.Other,
						IPs:   stats.IPs,
						Ratio: stats.Ratio / float64(c),
					},
				},
			})
		}
	}

	if numBlocked > 0 {
		log.Trace("updating blocks due to new ASN/Network block")
		n.blocker.CheckBlocked()
	}

	n.mutex.Lock()
	defer n.mutex.Unlock()

	n.asnData = asns
	if l := len(asns); l > 0 {
		n.asnAverage = total / l
	} else {
		n.asnAverage = 0
	}

	n.netData = nets
	if l := len(nets); l > 0 {
		n.netAverage = total / l
	} else {
		n.netAverage = 0
	}
}

func (n *Networks) logStats() {
	avgs := n.Averages()
	log.Infof("networks: %d, total: %d, app: %d, other: %d, ratio: %.2f, asn avg: %d, net avg: %d", n.Count(), avgs.Total, avgs.App, avgs.Other, avgs.Ratio, n.asnAverage, n.netAverage)
}

func (n *Networks) apiGetBlockedNetworks(c *gin.Context) {
	blocked := n.blocker.BlockedNetworks()

	sort.Slice(blocked, func(a, b int) bool {
		return blocked[a].Total > blocked[b].Total
	})

	c.JSON(http.StatusOK, blocked)
}

func (n *Networks) apiGetBlockedASNs(c *gin.Context) {
	blocked := n.blocker.BlockedASNs()

	sort.Slice(blocked, func(a, b int) bool {
		return blocked[a].Total > blocked[b].Total
	})

	c.JSON(http.StatusOK, blocked)
}

func (n *Networks) apiGetNetworks(c *gin.Context) {
	c.JSON(http.StatusOK, n.All())
}

func (n *Networks) apiGetNetwork(c *gin.Context) {
	_, network, err := net.ParseCIDR(fmt.Sprintf("%s/%s", c.Param("ip"), c.Param("bits")))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotAcceptable, gin.H{"error": fmt.Sprintf("%s is not a valid network in CIDR notation", c.Param("cidr"))})
		return
	}
	nw, ok := n.NetworkStats(network)
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{})
		return
	}

	c.JSON(http.StatusOK, nw)
}

func (n *Networks) apiGetASNStats(c *gin.Context) {
	asn, err := strconv.Atoi(c.Param("asn"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotAcceptable, gin.H{"error": fmt.Sprintf("%s is not a number", c.Param("asn"))})
		return
	}
	st, ok := n.ASNStats(asn)
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{})
		return
	}

	c.JSON(http.StatusOK, st)
}

package botex

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/emirpasic/gods/maps/treemap"
	"github.com/scraperwall/asndb/v2"
	log "github.com/sirupsen/logrus"
)

// Networks contains requests statistics for all networks
type Networks struct {
	data       map[string]*NetworkData
	mutex      sync.RWMutex
	removeChan chan *net.IPNet
	windowSize time.Duration
	numWindows int
	config     *Config
	ctx        context.Context
}

// NetworkData contains request statistics for one network
type NetworkData struct {
	Network   *net.IPNet `json:"network"`
	Total     int        `json:"total"`
	App       int        `json:"app"`
	Other     int        `json:"other"`
	Ratio     float64    `json:"ratio"`
	UpdatedAt time.Time  `json:"updatedat"`
	ASN       *asndb.ASN `json:"asn"`

	totalMap *treemap.Map
	appMap   *treemap.Map
	otherMap *treemap.Map

	removeChan  chan *net.IPNet
	mutex       sync.RWMutex
	windowSize  time.Duration
	numWindows  int
	expireTimer *time.Timer
	ctx         context.Context
}

// NewNetworkData creates a new NetworkData instance
func NewNetworkData(ctx context.Context, ipnet *net.IPNet, removeChan chan *net.IPNet, config *Config) *NetworkData {
	nd := &NetworkData{
		Network:     ipnet,
		Total:       0,
		App:         0,
		Other:       0,
		Ratio:       0.0,
		totalMap:    treemap.NewWithIntComparator(),
		appMap:      treemap.NewWithIntComparator(),
		otherMap:    treemap.NewWithIntComparator(),
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
				if nd.expireTimer != nil {
					nd.expireTimer.Stop()
					nd.expireTimer = nil
				}

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
func (nd *NetworkData) HandleRequest(r *Request) {
	if nd.ASN == nil {
		nd.ASN = r.ASN
	}

	key := nd.keyFor(r.Time)
	var val int64

	stats := nd.appMap
	if assetRegexp.MatchString(r.URL) {
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

	nd.mutex.Lock()
	defer nd.mutex.Unlock()

	if nd.expireTimer != nil {
		nd.expireTimer.Reset(nd.windowSize * time.Duration(nd.numWindows))
	} else {
		nd.expireTimer = time.NewTimer(nd.windowSize * time.Duration(nd.numWindows))
	}
}

func (nd *NetworkData) updateStats() {
	nd.updateCount(&nd.Total, nd.totalMap)
	nd.updateCount(&nd.App, nd.appMap)
	nd.updateCount(&nd.Other, nd.otherMap)
	if nd.Total > 0 {
		nd.Ratio = float64(nd.App) / float64(nd.Total)
	}
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
	if numRemoved > 0 {
		nd.updateStats()
	}
}

// NewNetworks creates a new Networks instance
func NewNetworks(ctx context.Context, config *Config) *Networks {
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
				n.expire()
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
func (n *Networks) HandleRequest(r *Request) {
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

// Get returns the metadata and statistics for one network
func (n *Networks) Get(ipn *net.IPNet) (nd *NetworkData, found bool) {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	nd, found = n.data[ipn.String()]

	return nd, found
}

// Averages returns the average of requests across all networks
func (n *Networks) Averages() IPStats {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	res := IPStats{}

	count := 0

	for _, nd := range n.data {
		res.Total += nd.Total
		res.App += nd.App
		res.Other += nd.Other
		count++
	}

	if count > 0 {
		res.Total = res.Total / count
		res.App = res.App / count
		res.Other = res.Other / count
		res.Ratio = float64(res.App) / float64(res.Total)
	}

	return res
}

func (n *Networks) expire() {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	for _, nd := range n.data {
		nd.expire()
	}
}

func (n *Networks) logStats() {
	avgs := n.Averages()
	log.Infof("networks: %d, total: %d, app: %d, other: %d, ratio: %.2f", n.Count(), avgs.Total, avgs.App, avgs.Other, avgs.Ratio)
}

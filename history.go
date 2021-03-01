package botex

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/scraperwall/asndb"
	"github.com/scraperwall/geoip"
)

type History struct {
	config     *Config
	data       map[string]*IPData
	asnDB      *asndb.ASNDB
	geoipDB    *geoip.GeoIP
	mutex      sync.RWMutex
	windowSize time.Duration
	numWindows int
	ctx        context.Context
}

func NewHistory(ctx context.Context, config *Config) *History {
	h := History{
		config:     config,
		data:       make(map[string]*IPData),
		windowSize: config.WindowSize,
		numWindows: config.NumWindows,
		mutex:      sync.RWMutex{},
		ctx:        ctx,
	}

	return &h
}

func (h *History) Add(r *Request) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	ip := net.ParseIP(r.Source)
	ipstr := ip.String()

	if _, ok := h.data[r.Source]; !ok {
		h.data[ipstr] = NewIPData(h.ctx, ip, h.config)
	}

	h.data[ipstr].Add(r)

}

func (h *History) SetHostname(ip net.IP, hostname string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.data[ip.String()].SetHostname(hostname)
}

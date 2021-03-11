package botex

import (
	"net"
	"regexp"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

var assetRegexp *regexp.Regexp

func init() {
	assetRegexp = regexp.MustCompile(`\.(jpe?g|png|gif|webp|tiff?|pdf|css|js|woff2?|ttf|eot|svg|ttc)\b`)
}

// Requests contains all HTTP requests for a given time.
// The requests are divied by their type: app and other (assets)
// Requests notifies its parent of changes through updateChan
type Requests struct {
	IP                 net.IP
	appWindow          *Window
	otherWindow        *Window
	allWindow          *Window
	userAgents         *MapWindow
	latest             *RequestsWindow
	updateChan         chan IPStats
	updateChanIsClosed bool
	mutex              sync.RWMutex
	createdAt          time.Time
	expiredAt          time.Time
	windowSize         time.Duration
}

// NewRequests creates a new Requests item.
// The app context and configuration get passed into the new item
func NewRequests(ip net.IP, updateChan chan IPStats, config *Config) *Requests {
	reqs := &Requests{
		IP:                 ip,
		appWindow:          NewWindow(config.WindowSize, config.NumWindows),
		otherWindow:        NewWindow(config.WindowSize, config.NumWindows),
		allWindow:          NewWindow(config.WindowSize, config.NumWindows),
		userAgents:         NewMapWindow(config.WindowSize, config.NumWindows),
		latest:             NewRequestsWindow(config.KeepRequests, config.WindowSize, config.NumWindows),
		mutex:              sync.RWMutex{},
		createdAt:          time.Now(),
		expiredAt:          time.Now(),
		windowSize:         config.WindowSize,
		updateChan:         updateChan,
		updateChanIsClosed: false,
	}

	return reqs
}

// Total returns the total number of requests
func (r *Requests) Total() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return int(r.allWindow.Count())
}

// App returns the number of all application requests
func (r *Requests) App() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return int(r.appWindow.Count())
}

// Other returns the number of all non-app requests
func (r *Requests) Other() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return int(r.otherWindow.Count())
}

// CanBeExpired determines whether there are requests that can be expired
func (r *Requests) CanBeExpired() bool {
	now := time.Now()
	timeSinceCreation := now.Sub(r.createdAt)
	timeSinceLastExpire := now.Sub(r.expiredAt)

	return timeSinceCreation > r.windowSize && timeSinceLastExpire > r.windowSize
}

// Expire expires everything and updates the stats
func (r *Requests) Expire() int {

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if !r.CanBeExpired() {
		return r.latest.Len()
	}

	// expire time window stats
	allSize := r.allWindow.Expire()
	appSize := r.appWindow.Expire()
	otherSize := r.otherWindow.Expire()
	uaSize := r.userAgents.Expire()
	latestSize := r.latest.Expire()

	size := allSize + appSize + otherSize + uaSize + latestSize
	log.Tracef("requests expire - all: %d, app: %d, other: %d, ua: %d, latest: %d", allSize, appSize, otherSize, uaSize, latestSize)

	r.expiredAt = time.Now()
	return size
}

// Add adds a request
func (r *Requests) Add(req *Request) {
	if r == nil || req == nil {
		log.Fatal("r or req is nil")
		return
	}

	log.Tracef("expire %s", r.IP)
	r.Expire()
	log.Tracef("expire %s done", r.IP)

	r.mutex.Lock()
	if r.allWindow == nil {
		log.Tracef("allWindow %s is nil", r.IP)
		return
	}
	r.allWindow.Add(req.Time)
	log.Tracef("allWindow add %s done", r.IP)

	if assetRegexp.MatchString(req.URL) {
		r.otherWindow.Add(req.Time)
	} else {
		r.appWindow.Add(req.Time)
	}
	log.Tracef("app/otherWindow add %s done", r.IP)

	r.latest.Add(req)
	r.userAgents.Add(req)

	r.mutex.Unlock()
	log.Trace("Requests Add after mutex unlock")

	// update the stats on each request
	// this makes sure the maximum number for the overall time window is used
	//
	total := r.allWindow.Count()
	app := r.appWindow.Count()
	other := r.otherWindow.Count()
	ratio := 0.0
	if total > 0.0 {
		ratio = float64(app) / float64(total)
	}

	stats := IPStats{
		IP:    r.IP,
		Total: int(total),
		App:   int(app),
		Other: int(other),
		Ratio: ratio,
	}

	defer func() {
		// recovering from panic caused by writing to a closed channel:
		// mark the updateChan as closed
		if recover() != nil {
			r.updateChanIsClosed = true
		}
	}()

	log.Tracef("Requests Add before send to updateChan: %s", stats.IP)
	r.updateChan <- stats
	log.Tracef("Requests Add after send to updateChan: %s", stats.IP)

}

// Ratio returns the ratio of app requests / total requests
func (r *Requests) Ratio() float64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return float64(r.allWindow.Count()) / float64(r.appWindow.Count())
}

// Latest returns the most recent requests
func (r *Requests) Latest() []*Request {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.latest.Requests()
}

// Useragents returns a map made up of the user agent and its responding count
func (r *Requests) Useragents() map[string]int {
	r.mutex.RLock()
	r.mutex.RUnlock()

	return r.userAgents.TotalMap()
}

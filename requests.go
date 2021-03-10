package botex

import (
	"context"
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
	app                *Window
	other              *Window
	all                *Window
	userAgents         *MapWindow
	latest             *RequestsWindow
	updateChan         chan IPStats
	updateChanIsClosed bool
	config             *Config
	mutex              sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
}

// NewRequests creates a new Requests item.
// The app context and configuration get passed into the new item
func NewRequests(ctx context.Context, config *Config, updateChan chan IPStats) *Requests {
	myctx, mycancel := context.WithCancel(ctx)

	reqs := &Requests{
		config:             config,
		app:                NewWindow(myctx, config.WindowSize, config.NumWindows),
		other:              NewWindow(myctx, config.WindowSize, config.NumWindows),
		all:                NewWindow(myctx, config.WindowSize, config.NumWindows),
		userAgents:         NewMapWindow(myctx, config.WindowSize, config.NumWindows),
		latest:             NewRequestsWindow(myctx, config),
		updateChan:         updateChan,
		updateChanIsClosed: false,
		mutex:              sync.RWMutex{},
		cancel:             mycancel,
	}

	go func() {
		ticker := time.NewTicker(config.WindowSize)
		for {
			select {
			case <-myctx.Done():
				reqs.app = nil
				reqs.other = nil
				reqs.all = nil
				reqs.userAgents = nil
				reqs.latest = nil
				return
			case <-ticker.C:
				reqs.updateStats()
			}
		}
	}()

	return reqs
}

// Stop halts all go funcs
func (r *Requests) Stop() {
	r.cancel()
}

// Total returns the total number of requests
func (r *Requests) Total() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return int(r.all.Count())
}

// App returns the number of all application requests
func (r *Requests) App() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return int(r.app.Count())
}

// Other returns the number of all non-app requests
func (r *Requests) Other() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return int(r.other.Count())
}

func (r *Requests) updateStats() {
	if r.updateChanIsClosed {
		log.Info("updateChan is closed")
		return
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// update the stats on each request
	// this makes sure the maximum number for the overall time window is used
	//
	total := r.all.Count()
	app := r.app.Count()
	other := r.other.Count()

	defer func() {
		// recovering from panic caused by writing to a closed channel:
		// mark the updateChan as closed
		if recover() != nil {
			r.updateChanIsClosed = true
		}
	}()

	ratio := 0.0
	if total > 0.0 {
		ratio = float64(app) / float64(total)
	}

	stats := IPStats{
		Total: int(total),
		App:   int(app),
		Other: int(other),
		Ratio: ratio,
	}

	r.updateChan <- stats
}

// Add adds a request
func (r *Requests) Add(req *Request) {
	if r == nil || req == nil || r.updateChanIsClosed {
		return
	}

	r.mutex.Lock()
	if r.all == nil {
		return
	}
	r.all.Add(req.Time)

	if assetRegexp.MatchString(req.URL) {
		r.other.Add(req.Time)
	} else {
		r.app.Add(req.Time)
	}

	r.latest.Add(req)
	r.userAgents.Add(req)
	r.mutex.Unlock()

	r.updateStats()
}

// Ratio returns the ratio of app requests / total requests
func (r *Requests) Ratio() float64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return float64(r.all.Count()) / float64(r.app.Count())
}

// Latest returns the most recent requests
func (r *Requests) Latest() []*Request {
	return r.latest.Requests()
}

// Useragents returns a map made up of the user agent and its responding count
func (r *Requests) Useragents() map[string]int {
	return r.userAgents.TotalMap()
}

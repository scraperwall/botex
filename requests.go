package botex

import (
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/scraperwall/rolling"
)

var assetRegexp *regexp.Regexp

func init() {
	assetRegexp = regexp.MustCompile(`\.(jpe?g|png|gif|webp|tiff?|pdf|css|js|woff2?|ttf|eot|svg|ttc)\b`)
}

// Requests contains all HTTP requests for a given time.
// The requests are divied by their type: app and other (assets)
// Requests notifies its parent of changes through updateChan
type Requests struct {
	app                *rolling.TimePolicy
	other              *rolling.TimePolicy
	all                *rolling.TimePolicy
	userAgents         *MapWindow
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
		app:                rolling.NewTimePolicy(rolling.NewWindow(config.NumWindows), config.WindowSize),
		other:              rolling.NewTimePolicy(rolling.NewWindow(config.NumWindows), config.WindowSize),
		all:                rolling.NewTimePolicy(rolling.NewWindow(config.NumWindows), config.WindowSize),
		userAgents:         NewMapWindow(myctx, config.WindowSize, config.NumWindows),
		updateChan:         updateChan,
		updateChanIsClosed: false,
		mutex:              sync.RWMutex{},
		ctx:                myctx,
		cancel:             mycancel,
	}

	go func() {
		ticker := time.NewTicker(config.WindowSize)

		for {
			select {
			case <-reqs.ctx.Done():
				ticker.Stop()
				ticker = nil
				reqs.app = nil
				reqs.other = nil
				reqs.all = nil
				reqs.userAgents = nil
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

	return int(r.all.Reduce(rolling.Sum))
}

// App returns the number of all application requests
func (r *Requests) App() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return int(r.app.Reduce(rolling.Sum))
}

// Other returns the number of all non-app requests
func (r *Requests) Other() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return int(r.other.Reduce(rolling.Sum))
}

func (r *Requests) updateStats() {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// update the stats on each request
	// this makes sure the maximum number for the overall time window is used
	//
	total := r.all.Reduce(rolling.Sum)
	app := r.app.Reduce(rolling.Sum)
	other := r.other.Reduce(rolling.Sum)

	defer func() {
		// recovering from panic caused by writing to a closed channel:
		// mark the updateChan as closed
		if recover() != nil {
			r.updateChanIsClosed = true
		}
	}()

	ratio := 0.0
	if total > 0.0 {
		ratio = app / total
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
	if r.updateChanIsClosed {
		return
	}

	r.mutex.Lock()
	r.all.Add(1, req.Time)

	if assetRegexp.MatchString(req.URL) {
		r.other.Add(1, req.Time)
	} else {
		r.app.Add(1, req.Time)
	}
	r.mutex.Unlock()

	r.updateStats()
}

// Ratio returns the ratio of app requests / total requests
func (r *Requests) Ratio() float64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.all.Reduce(rolling.Count) / r.app.Reduce(rolling.Count)
}

// ByTimeWindow returns an array of counts by each time window
func (r *Requests) ByTimeWindow() []int {
	var data []int

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	agg := func(w rolling.Window) float64 {
		for _, bucket := range w {
			data = append(data, len(bucket))
		}

		return 0.0
	}

	r.all.Reduce(agg)

	return data
}

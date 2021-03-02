package botex

import (
	"context"
	"regexp"
	"sync"

	"github.com/asecurityteam/rolling"
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
}

// NewRequests creates a new Requests item.
// The app context and configuration get passed into the new item
func NewRequests(ctx context.Context, config *Config, updateChan chan IPStats) *Requests {
	return &Requests{
		config:             config,
		app:                rolling.NewTimePolicy(rolling.NewWindow(config.NumWindows), config.WindowSize),
		other:              rolling.NewTimePolicy(rolling.NewWindow(config.NumWindows), config.WindowSize),
		all:                rolling.NewTimePolicy(rolling.NewWindow(config.NumWindows), config.WindowSize),
		userAgents:         NewMapWindow(ctx, config.WindowSize, config.NumWindows),
		updateChan:         updateChan,
		updateChanIsClosed: false,
		mutex:              sync.RWMutex{},
		ctx:                ctx,
	}
}

// Total returns the total number of requests
func (r *Requests) Total() int {
	return int(r.all.Reduce(rolling.Count))
}

// App returns the number of all application requests
func (r *Requests) App() int {
	return int(r.app.Reduce(rolling.Count))
}

// Other returns the number of all non-app requests
func (r *Requests) Other() int {
	return int(r.other.Reduce(rolling.Count))
}

// Add adds a request
func (r *Requests) Add(req *Request) {
	if r.updateChanIsClosed {
		return
	}

	r.all.Append(1.0)

	if assetRegexp.MatchString(req.URL) {
		r.other.Append(1.0)
	} else {
		r.app.Append(1.0)
	}

	// update the stats on each request
	// this makes sure the maximum number for the overall time window is used
	//
	total := r.all.Reduce(rolling.Count)
	app := r.app.Reduce(rolling.Count)
	other := r.other.Reduce(rolling.Count)

	defer func() {
		// recovering from panic caused by writing to a closed channel:
		// mark the updateChan as closed
		if recover() != nil {
			r.updateChanIsClosed = true
		}
	}()

	r.updateChan <- IPStats{
		Total: int(total),
		App:   int(app),
		Other: int(other),
		Ratio: app / total,
	}
}

// Ratio returns the ratio of app requests / total requests
func (r *Requests) Ratio() float64 {
	return r.all.Reduce(rolling.Count) / r.app.Reduce(rolling.Count)
}

// ByTimeWindow returns an array of counts by each time window
func (r *Requests) ByTimeWindow() []int {
	var data []int

	agg := func(w rolling.Window) float64 {
		for _, bucket := range w {
			data = append(data, len(bucket))
		}

		return 0.0
	}

	r.all.Reduce(agg)

	return data
}

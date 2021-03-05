package botex

import (
	"context"
	"sync"
	"time"

	"github.com/scraperwall/rolling"
)

// MapWindow is a map that contains a rolling.TimePolicy for each entry
type MapWindow struct {
	windowSize time.Duration
	numWindows int
	data       map[string]*rolling.TimePolicy
	mutex      sync.RWMutex
	ctx        context.Context
}

// NewMapWindow creates a new MapWindow. windowSize determines the size of each window, numWindows how many windows there should be
// The context of the parent gets passed on to the new instance
func NewMapWindow(ctx context.Context, windowSize time.Duration, numWindows int) *MapWindow {
	mw := MapWindow{
		windowSize: windowSize,
		numWindows: numWindows,
		data:       make(map[string]*rolling.TimePolicy),
		mutex:      sync.RWMutex{},
		ctx:        ctx,
	}

	go mw.cleanup()

	return &mw
}

// Add adds one item for the given key
func (mw *MapWindow) Add(key string) {
	mw.mutex.Lock()
	defer mw.mutex.Unlock()

	if _, ok := mw.data[key]; !ok {
		window := rolling.NewWindow(mw.numWindows)
		mw.data[key] = rolling.NewTimePolicy(window, mw.windowSize)
	}

	mw.data[key].Append(1.0)
}

// Total determines the sum of all map entries
func (mw *MapWindow) Total() int {
	mw.mutex.RLock()
	defer mw.mutex.RUnlock()

	total := 0

	for _, window := range mw.data {
		total += int(window.Reduce(rolling.Count))
	}

	return total
}

// TotalMap returns a map that contains the key along with the sum of its entries
func (mw *MapWindow) TotalMap() map[string]int {
	mw.mutex.RLock()
	defer mw.mutex.RUnlock()

	data := make(map[string]int)

	for k, window := range mw.data {
		data[k] = int(window.Reduce(rolling.Count))
	}

	return data
}

// Size returns how many entries the MapWindow has got
func (mw *MapWindow) Size() int {
	mw.mutex.RLock()
	defer mw.mutex.RUnlock()

	return len(mw.data)
}

func (mw *MapWindow) cleanup() {
	ticker := time.NewTicker(mw.windowSize)

	for {
		select {
		case <-mw.ctx.Done():
			ticker.Stop()
			ticker = nil
			mw.data = nil
			return
		case <-ticker.C:
			mw.mutex.Lock()
			for ua, window := range mw.data {
				if window.Reduce(rolling.Count) <= 0.0 {
					delete(mw.data, ua)
				}
			}
			mw.mutex.Unlock()
		}
	}
}

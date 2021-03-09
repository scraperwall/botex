package botex

import (
	"context"
	"sync"
	"time"
)

// MapWindow is a map that contains a rolling.TimePolicy for each entry
type MapWindow struct {
	windowSize time.Duration
	numWindows int
	data       map[string]*Window
	mutex      sync.RWMutex
	ctx        context.Context
}

// NewMapWindow creates a new MapWindow. windowSize determines the size of each window, numWindows how many windows there should be
// The context of the parent gets passed on to the new instance
func NewMapWindow(ctx context.Context, windowSize time.Duration, numWindows int) *MapWindow {
	mw := MapWindow{
		windowSize: windowSize,
		numWindows: numWindows,
		data:       make(map[string]*Window),
		mutex:      sync.RWMutex{},
		ctx:        ctx,
	}

	go mw.cleanup()

	return &mw
}

// Add adds one item for the given key
func (mw *MapWindow) Add(req *Request) {
	mw.mutex.Lock()
	defer mw.mutex.Unlock()

	if _, ok := mw.data[req.UserAgent]; !ok {
		mw.data[req.UserAgent] = NewWindow(mw.ctx, mw.windowSize, mw.numWindows)
	}

	mw.data[req.UserAgent].Add(req.Time)
}

// Total determines the sum of all map entries
func (mw *MapWindow) Total() int {
	mw.mutex.RLock()
	defer mw.mutex.RUnlock()

	total := 0

	for _, window := range mw.data {
		total += int(window.Count())
	}

	return total
}

// TotalMap returns a map that contains the key along with the sum of its entries
func (mw *MapWindow) TotalMap() map[string]int {
	mw.mutex.RLock()
	defer mw.mutex.RUnlock()

	data := make(map[string]int)

	for k, window := range mw.data {
		data[k] = int(window.Count())
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
				if window.Count() <= 0 {
					delete(mw.data, ua)
				}
			}
			mw.mutex.Unlock()
		}
	}
}

package botex

import (
	"sync"
	"time"
)

// MapWindow is a map that contains a rolling.TimePolicy for each entry
type MapWindow struct {
	windowSize time.Duration
	numWindows int
	data       map[string]*Window
	mutex      sync.RWMutex
}

// NewMapWindow creates a new MapWindow. windowSize determines the size of each window, numWindows how many windows there should be
// The context of the parent gets passed on to the new instance
func NewMapWindow(windowSize time.Duration, numWindows int) *MapWindow {
	mw := MapWindow{
		windowSize: windowSize,
		numWindows: numWindows,
		data:       make(map[string]*Window),
		mutex:      sync.RWMutex{},
	}

	return &mw
}

// Add adds one item for the given key
func (mw *MapWindow) Add(req *Request) {
	mw.mutex.Lock()
	defer mw.mutex.Unlock()

	if _, ok := mw.data[req.UserAgent]; !ok {
		mw.data[req.UserAgent] = NewWindow(mw.windowSize, mw.numWindows)
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

// Expire removes expired items
func (mw *MapWindow) Expire() int {
	mw.mutex.Lock()
	defer mw.mutex.Unlock()

	count := 0

	for k, w := range mw.data {
		size := w.Expire()
		if size <= 0 {
			delete(mw.data, k)
		}
		count += size
	}

	return count
}

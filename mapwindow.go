package botex

import (
	"context"
	"sync"
	"time"

	"github.com/asecurityteam/rolling"
)

type MapWindow struct {
	windowSize time.Duration
	numWindows int
	data       map[string]*rolling.TimePolicy
	mutex      sync.RWMutex
	ctx        context.Context
}

func NewMapWindow(ctx context.Context, windowSize time.Duration, numWindows int) *MapWindow {
	mw := MapWindow{
		windowSize: windowSize,
		numWindows: numWindows,
		data:       make(map[string]*rolling.TimePolicy),
		mutex:      sync.RWMutex{},
	}

	go mw.cleanup()

	return &mw
}

func (mw *MapWindow) Add(ua string) {
	mw.mutex.Lock()
	defer mw.mutex.Unlock()

	if _, ok := mw.data[ua]; !ok {
		window := rolling.NewWindow(mw.numWindows)
		mw.data[ua] = rolling.NewTimePolicy(window, mw.windowSize)
	}

	mw.data[ua].Append(1.0)
}

func (mw *MapWindow) Total() int {
	mw.mutex.RLock()
	defer mw.mutex.RUnlock()

	total := 0

	for _, window := range mw.data {
		total += int(window.Reduce(rolling.Count))
	}

	return total
}

func (mw *MapWindow) TotalMap() map[string]int {
	mw.mutex.RLock()
	defer mw.mutex.RUnlock()

	data := make(map[string]int)

	for ua, window := range mw.data {
		data[ua] = int(window.Reduce(rolling.Count))
	}

	return data
}

func (mw *MapWindow) Size() int {
	mw.mutex.RLock()
	defer mw.mutex.RUnlock()

	return len(mw.data)
}

func (mw *MapWindow) cleanup() {
	for {
		select {
		case <-mw.ctx.Done():
			break
		case <-time.After(time.Second):
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

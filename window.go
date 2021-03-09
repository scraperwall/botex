package botex

import (
	"context"
	"sync"
	"time"

	"github.com/emirpasic/gods/maps/treemap"
)

// Window implements a rolling window using a TreeMap as storage
type Window struct {
	data  *treemap.Map
	mutex sync.RWMutex

	windowSize time.Duration
	numWindows int
}

// NewWindow creates a new TreemapWindow instance with numWindows buckets that each cover a windowSize time range
func NewWindow(ctx context.Context, windowSize time.Duration, numWindows int) *Window {
	w := Window{
		data:       treemap.NewWithIntComparator(),
		mutex:      sync.RWMutex{},
		windowSize: windowSize,
		numWindows: numWindows,
	}

	go w.expire(ctx, windowSize)

	return &w
}

// Add adds an item to the Treemap instance
func (w *Window) Add(t time.Time) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	key := w.keyFor(t)
	var val int64

	if v, ok := w.data.Get(key); ok {
		val = v.(int64)
	}

	val++

	w.data.Put(w.keyFor(t), val)
}

// Count returns the total count of items in all buckets
func (w *Window) Count() int64 {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	if w == nil || w.data == nil {
		return 0
	}

	var total int64

	iter := w.data.Iterator()
	for iter.Next() {
		total += iter.Value().(int64)
	}

	return total
}

func (w *Window) keyFor(t time.Time) int {
	return int(t.UnixNano() - t.UnixNano()%w.windowSize.Nanoseconds())
}

func (w *Window) removeExpired() {
	threshold := int(time.Now().Add(-1 * w.windowSize * time.Duration(w.numWindows)).UnixNano())

	w.mutex.Lock()
	defer w.mutex.Unlock()

	iter := w.data.Iterator()

	for iter.Next() {
		key := iter.Key().(int)
		if key >= threshold {
			return
		}
		w.data.Remove(key)
	}
}

func (w *Window) expire(ctx context.Context, windowSize time.Duration) {
	ticker := time.NewTicker(windowSize)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			ticker = nil
			w.mutex.Lock()
			w.data = nil
			w.mutex.Unlock()
			return
		case <-ticker.C:
			w.removeExpired()
		}
	}
}

package botex

import (
	"sync"
	"time"

	"github.com/emirpasic/gods/maps/treemap"
	log "github.com/sirupsen/logrus"
)

// Window implements a rolling window using a TreeMap as storage
type Window struct {
	data  *treemap.Map
	mutex sync.RWMutex

	windowSize time.Duration
	numWindows int
}

// NewWindow creates a new TreemapWindow instance with numWindows buckets that each cover a windowSize time range
// TODO: remove ctx as arg
func NewWindow(windowSize time.Duration, numWindows int) *Window {
	w := Window{
		data:       treemap.NewWithIntComparator(),
		mutex:      sync.RWMutex{},
		windowSize: windowSize,
		numWindows: numWindows,
	}

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

	w.data.Put(key, val)
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

// Expire removes expired items from the window
func (w *Window) Expire() int {
	threshold := int(time.Now().Add(-1 * w.windowSize * time.Duration(w.numWindows)).UnixNano())

	log.Tracef("window expire - threshold: %d, len: %d", threshold, w.data.Size())
	w.mutex.Lock()
	defer w.mutex.Unlock()

	iter := w.data.Iterator()

	for iter.Next() {
		key := iter.Key().(int)
		log.Tracef("window expire - diff: %d", key-threshold)
		if key >= threshold {
			break
		}
		w.data.Remove(key)
	}

	log.Tracef("window expire done - len: %d", w.data.Size())
	return w.data.Size()
}

func (w *Window) Map() map[int]int64 {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	res := make(map[int]int64)

	iter := w.data.Iterator()

	for iter.Next() {
		key := iter.Key().(int)
		// log.Infof("%d = %d", key, iter.Value())
		res[key] = iter.Value().(int64)
	}

	return res
}

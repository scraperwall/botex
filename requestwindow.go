package botex

import (
	"container/list"
	"sync"
	"time"

	"github.com/scraperwall/botex/data"
	log "github.com/sirupsen/logrus"
)

// RequestsWindow contains the most recent requests
// The number of requests is limited by a maximum number of requests the list may contain (maxSize) and
// ttl, the time requests stay in the list before they expire and are removed
type RequestsWindow struct {
	data       *list.List
	maxSize    int
	ttl        time.Duration
	windowSize time.Duration
	numWindows int
	mutex      sync.RWMutex
}

// NewRequestsWindow creates a new RequestsWindow.
// The app context and configuration get passed into the new item
func NewRequestsWindow(maxRequests int, windowSize time.Duration, numWindows int) *RequestsWindow {
	rw := RequestsWindow{
		data:       list.New(),
		maxSize:    maxRequests,
		ttl:        windowSize * time.Duration(numWindows),
		windowSize: windowSize,
		numWindows: numWindows,
		mutex:      sync.RWMutex{},
	}

	return &rw
}

// Add adds a single request
func (rw *RequestsWindow) Add(r *data.Request) {
	rw.mutex.Lock()
	defer rw.mutex.Unlock()

	rw.data.PushFront(r)
	if rw.data.Len() > rw.maxSize {
		rw.data.Remove(rw.data.Back())
	}
}

// Requests returns an array of all requests in the list
func (rw *RequestsWindow) Requests() []*data.Request {
	rw.mutex.Lock()
	defer rw.mutex.Unlock()
	reqs := make([]*data.Request, rw.data.Len())

	i := 0
	for e := rw.data.Front(); e != nil; e = e.Next() {
		reqs[i] = e.Value.(*data.Request)
		i++
	}

	return reqs
}

// Len returns the number of items in a RequestsWindow
func (rw *RequestsWindow) Len() int {
	return rw.data.Len()
}

// Expire removes expired requests from the window
func (rw *RequestsWindow) Expire() int {
	now := time.Now()

	rw.mutex.Lock()
	defer rw.mutex.Unlock()

	if rw.data == nil || rw.data.Len() <= 0 {
		return 0
	}

	for {
		oldest := rw.data.Back()

		if oldest == nil {
			break
		}

		if oldest.Value == nil {
			break
		}

		if tDiff := now.Sub(oldest.Value.(*data.Request).Time); tDiff <= rw.ttl {
			break
		}

		rw.data.Remove(oldest)
		log.Tracef("expiring %s (%v) from requests (%d)", oldest.Value.(*data.Request).URL, now.Sub(oldest.Value.(*data.Request).Time), rw.Len())
	}

	return rw.data.Len()
}

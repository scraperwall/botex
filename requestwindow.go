package botex

import (
	"container/list"
	"context"
	"sync"
	"time"

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
	ctx        context.Context
}

// NewRequestsWindow creates a new RequestsWindow.
// The app context and configuration get passed into the new item
func NewRequestsWindow(ctx context.Context, config *Config) *RequestsWindow {
	rw := RequestsWindow{
		data:       list.New(),
		maxSize:    config.KeepRequests,
		ttl:        config.WindowSize * time.Duration(config.NumWindows),
		windowSize: config.WindowSize,
		numWindows: config.NumWindows,
		mutex:      sync.RWMutex{},
		ctx:        ctx,
	}

	go rw.cleanup()

	return &rw
}

// Add adds a single request
func (rw *RequestsWindow) Add(r *Request) {
	rw.mutex.Lock()
	defer rw.mutex.Unlock()

	rw.data.PushFront(r)
	if rw.data.Len() > rw.maxSize {
		rw.data.Remove(rw.data.Back())
	}
}

// Requests returns an array of all requests in the list
func (rw *RequestsWindow) Requests() []*Request {
	rw.mutex.Lock()
	defer rw.mutex.Unlock()
	reqs := make([]*Request, rw.data.Len())

	i := 0
	for e := rw.data.Front(); e != nil; e = e.Next() {
		reqs[i] = e.Value.(*Request)
		i++
	}

	return reqs
}

// Len returns the number of items in a RequestsWindow
func (rw *RequestsWindow) Len() int {
	return rw.data.Len()
}

func (rw *RequestsWindow) expire() {
	now := time.Now()

	rw.mutex.Lock()
	defer rw.mutex.Unlock()

	if rw.data == nil || rw.data.Len() <= 0 {
		log.Warn("data is empty")
		return
	}

	for {
		oldest := rw.data.Back()

		if oldest == nil {
			break
		}

		if oldest.Value == nil {
			break
		}

		if tDiff := now.Sub(oldest.Value.(*Request).Time); tDiff <= rw.ttl {
			break
		}

		rw.data.Remove(oldest)
		log.Tracef("expiring %s (%v) from requests (%d)", oldest.Value.(*Request).URL, now.Sub(oldest.Value.(*Request).Time), rw.Len())
	}
}

func (rw *RequestsWindow) cleanup2() {
	<-rw.ctx.Done()
	if rw.data != nil {
		rw.data.Init()
		rw.data = nil
	}
}

func (rw *RequestsWindow) cleanup() {
	ticker := time.NewTicker(time.Minute)

	for {
		select {
		case <-rw.ctx.Done():
			if rw.data != nil {
				rw.data.Init()
				rw.data = nil
			}
			ticker.Stop()
			ticker = nil
			return
		case <-ticker.C:
			rw.expire()
		}
	}
}

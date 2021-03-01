package botex

import (
	"container/list"
	"context"
	"sync"
	"time"
)

type RequestsWindow struct {
	data    *list.List
	maxSize int
	ttl     time.Duration
	mutex   sync.RWMutex
	ctx     context.Context
}

func NewRequestsWindow(ctx context.Context, config *Config) *RequestsWindow {
	rw := RequestsWindow{
		data:    list.New(),
		maxSize: config.KeepRequests,
		ttl:     config.WindowSize * time.Duration(config.NumWindows),
		mutex:   sync.RWMutex{},
		ctx:     ctx,
	}

	go rw.cleanup()

	return &rw
}

func (rw *RequestsWindow) Add(r *Request) {
	rw.mutex.Lock()
	defer rw.mutex.Unlock()

	rw.data.PushFront(r)
	if rw.data.Len() > rw.maxSize {
		rw.data.Remove(rw.data.Back())
	}
}

func (rw RequestsWindow) Requests() []*Request {
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

func (rw *RequestsWindow) cleanup() {
	for {
		select {
		case <-rw.ctx.Done():
			rw.data.Init()
			break
		case <-time.After(time.Second):
			now := time.Now()

			rw.mutex.Lock()
			for now.Sub(rw.data.Back().Value.(*Request).Time) > rw.ttl {
				rw.data.Remove(rw.data.Back())
			}
			rw.mutex.Unlock()
		}
	}
}

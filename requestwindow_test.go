package botex

import (
	"context"
	"fmt"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

func TestRequestWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rw := NewRequestsWindow(ctx, &Config{
		KeepRequests: 100,
		WindowSize:   100 * time.Millisecond,
		NumWindows:   5,
	})

	for i := 0; i < 130; i++ {
		req := &Request{
			UserAgent: "test/1.0",
			Host:      "test.scw",
			Time:      time.Now(),
			URL:       fmt.Sprintf("/index/%d", i),
		}
		rw.Add(req)
		time.Sleep(5 * time.Millisecond)
	}

	if rw.Len() != 100 {
		t.Errorf("RequestWindow should contain exactly 100 elements but has %d", rw.Len())
	}

	// time.Sleep(500 * time.Millisecond)

	if rw.Len() != 95 {
		t.Errorf("RequestWindow should contain exactly 95 elements after one window has passed but has %d", rw.Len())
	}

	cancel()
	log.Println(rw.data.Len())

}

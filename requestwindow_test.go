package botex

import (
	"fmt"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

func TestRequestWindow(t *testing.T) {
	rw := NewRequestsWindow(100, 100, 5)

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

	log.Println(rw.data.Len())
}

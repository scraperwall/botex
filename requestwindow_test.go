/*
	botex - a bad bot mitigation tool by ScraperWall
	Copyright (C) 2021 ScraperWall, Tobias von Dewitz <tobias@scraperwall.com>

	This program is free software: you can redistribute it and/or modify it
	under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or (at your
	option) any later version.

	This program is distributed in the hope that it will be useful, but WITHOUT
	ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
	FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License
	for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package botex

import (
	"fmt"
	"testing"
	"time"

	"github.com/scraperwall/botex/data"
	log "github.com/sirupsen/logrus"
)

func TestRequestWindow(t *testing.T) {
	rw := NewRequestsWindow(100, 100, 5)

	for i := 0; i < 130; i++ {
		req := &data.Request{
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

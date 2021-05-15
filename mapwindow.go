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
	"sync"
	"time"

	"github.com/scraperwall/botex/data"
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
func (mw *MapWindow) Add(req *data.Request) {
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

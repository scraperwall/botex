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

package store

import (
	"time"
)

// KVStoreEachFunc is the function that gets called on each item in the Each function
type KVStoreEachFunc func([]byte)

// KVStore defines an embedded key/value store database interface.
type KVStore interface {
	Get(key []byte) (value []byte, err error)
	SetEx(key, value []byte, ttl time.Duration) error
	Set(key, value []byte) error
	Has(key []byte) (bool, error)
	All(prefix []byte) ([][]byte, error)
	Count(prefix []byte) (int, error)
	Remove(prefix []byte) error
	Each(prefix []byte, callback KVStoreEachFunc) error
	ErrNotFound() error
	Close() error
}

package store

import (
	"time"
)

// KVStoreEachFunc is the function that gets called on each item in the Each function
type KVStoreEachFunc func([]byte)

// KVStore defines an embedded key/value store database interface.
type KVStore interface {
	Get(prefix []byte) (value []byte, err error)
	SetEx(prefix, value []byte, ttl time.Duration) error
	Set(prefix, value []byte) error
	Has(prefix []byte) (bool, error)
	All(prefix []byte) ([][]byte, error)
	Count(prefix []byte) (int, error)
	Remove(prefix []byte) error
	Each(prefix []byte, callback KVStoreEachFunc) error
	ErrNotFound() error
	Close() error
}

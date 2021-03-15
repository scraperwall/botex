package store

import (
	"time"
)

const (
	// Default BadgerDB discardRatio. It represents the discard ratio for the
	// BadgerDB GC.
	//
	// Ref: https://godoc.org/github.com/dgraph-io/badger#DB.RunValueLogGC
	badgerDiscardRatio = 0.5

	// Default BadgerDB GC interval
	badgerGCInterval = 10 * time.Minute
)

var (
	// BadgerAlertNamespace defines the alerts BadgerDB namespace.
	BadgerAlertNamespace = []byte("alerts")
)

// KVStoreEachFunc is the function that gets called on each item in the Each function
type KVStoreEachFunc func([]byte)

// KVStore defines an embedded key/value store database interface.
type KVStore interface {
	Get(namespace, key []byte) (value []byte, err error)
	SetEx(namespace, key, value []byte, ttl time.Duration) error
	Set(namespace, key, value []byte) error
	Has(namespace, key []byte) (bool, error)
	All(namespace, prefix []byte) ([][]byte, error)
	Count(namespace, prefix []byte) (int, error)
	Remove(namespace, key []byte) error
	Each(namespace []byte, prefix []byte, callback KVStoreEachFunc) error
	ErrNotFound() error
	Close() error
}
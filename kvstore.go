package botex

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

// KVStore defines an embedded key/value store database interface.
type KVStore interface {
	Get(namespace, key []byte) (value []byte, err error)
	SetEx(namespace, key, value []byte, ttl time.Duration) error
	Set(namespace, key, value []byte) error
	Has(namespace, key []byte) (bool, error)
	All(namespace, prefix []byte) ([][]byte, error)
	Close() error
}

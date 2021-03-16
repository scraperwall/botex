package botex

import (
	"context"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/scraperwall/botex/store"
	log "github.com/sirupsen/logrus"
)

// BadgerDB is a wrapper around a BadgerDB backend database that implements
// the DB interface.
type BadgerDB struct {
	db  *badger.DB
	ctx context.Context
}

// NewBadgerDB returns a new initialized BadgerDB database implementing the DB
// interface. If the database cannot be initialized, an error will be returned.
func NewBadgerDB(ctx context.Context, dataDir string) (store.KVStore, error) {
	opts := badger.DefaultOptions(dataDir)
	opts.SyncWrites = true
	opts.Dir, opts.ValueDir = dataDir, dataDir

	badgerDB, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	bdb := &BadgerDB{
		db:  badgerDB,
		ctx: ctx,
	}

	go bdb.runGC()
	return bdb, nil
}

// Get implements the DB interface. It attempts to get a value for a given key
// and namespace. If the key does not exist in the provided namespace, an error
// is returned, otherwise the retrieved value.
func (bdb *BadgerDB) Get(namespace, key []byte) ([]byte, error) {
	var value []byte
	var err error

	err = bdb.db.View(func(txn *badger.Txn) error {
		item, err2 := txn.Get(bdb.badgerNamespaceKey(namespace, key))
		if err2 != nil {
			return err2
		}

		return item.Value(func(data []byte) error {
			value = make([]byte, len(data))
			copy(value, data)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return value, nil
}

// Set implements the DB interface. It attempts to store a value for a given key
// and namespace. If the key/value pair cannot be saved, an error is returned.
func (bdb *BadgerDB) Set(namespace, key, value []byte) error {
	err := bdb.db.Update(func(txn *badger.Txn) error {
		return txn.Set(bdb.badgerNamespaceKey(namespace, key), value)
	})

	if err != nil {
		return err
	}

	return nil
}

// SetEx stores the given key and value for the time given by ttl
// If the key/value pair can't be saved an error is returned
func (bdb *BadgerDB) SetEx(namespace, key, value []byte, ttl time.Duration) error {
	err := bdb.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(bdb.badgerNamespaceKey(namespace, key), value).WithTTL(ttl)
		return txn.SetEntry(e)
	})

	if err != nil {
		log.Info(err)
		return err
	}

	return nil
}

// Remove removes a single entry from the database
func (bdb *BadgerDB) Remove(namespace, key []byte) error {
	return bdb.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(bdb.badgerNamespaceKey(namespace, key))
	})
}

// Has implements the DB interface. It returns a boolean reflecting if the
// datbase has a given key for a namespace or not. An error is only returned if
// an error to Get would be returned that is not of type badger.ErrKeyNotFound.
func (bdb *BadgerDB) Has(namespace, key []byte) (ok bool, err error) {
	_, err = bdb.Get(namespace, key)
	switch err {
	case badger.ErrKeyNotFound:
		ok, err = false, nil
	case nil:
		ok, err = true, nil
	}

	return
}

// Close implements the DB interface. It closes the connection to the underlying
// BadgerDB database as well as invoking the context's cancel function.
func (bdb *BadgerDB) Close() error {
	return bdb.db.Close()
}

// runGC triggers the garbage collection for the BadgerDB backend database. It
// should be run in a goroutine.
func (bdb *BadgerDB) runGC() {
	ticker := time.NewTicker(10 * time.Minute)
	for {
		select {
		case <-ticker.C:
			err := bdb.db.RunValueLogGC(0.5)
			if err != nil {
				// don't report error when GC didn't result in any cleanup
				if err == badger.ErrNoRewrite {
					log.Debugf("no BadgerDB GC occurred: %v", err)
				} else {
					log.Errorf("failed to GC BadgerDB: %v", err)
				}
			}

		case <-bdb.ctx.Done():
			return
		}
	}
}

// All returns all values for the given namespace and prefix.
func (bdb *BadgerDB) All(namespace, prefix []byte) ([][]byte, error) {
	res := make([][]byte, 0)

	err := bdb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := bdb.badgerNamespaceKey(namespace, prefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				data := make([]byte, 0)
				copy(data, v)
				res = append(res, data)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return res, nil
}

// Each iterates over all items that match namespace and prefix
func (bdb *BadgerDB) Each(namespace, prefix []byte, callback store.KVStoreEachFunc) error {
	return bdb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := bdb.badgerNamespaceKey(namespace, prefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				callback(v)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Count returns the number of entries that match namespace and prefix
func (bdb *BadgerDB) Count(namespace, prefix []byte) (int, error) {
	c := 0

	err := bdb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := bdb.badgerNamespaceKey(namespace, prefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			c++
		}
		return nil
	})

	if err != nil {
		return c, err
	}

	return c, nil
}

// ErrNotFound is the error badger returns when it can't find a key in the database
func (bdb *BadgerDB) ErrNotFound() error {
	return badger.ErrKeyNotFound
}

// badgerNamespaceKey returns a composite key used for lookup and storage for a
// given namespace and key.
func (bdb *BadgerDB) badgerNamespaceKey(namespace, key []byte) []byte {
	return []byte(fmt.Sprintf("%s/%s", string(namespace), string(key)))
}

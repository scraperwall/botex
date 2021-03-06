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
	"context"
	"time"

	badger "github.com/dgraph-io/badger/v3"
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
func NewBadgerDB(ctx context.Context, dataDir string) (KVStore, error) {
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
func (bdb *BadgerDB) Get(key []byte) ([]byte, error) {
	var value []byte
	var err error

	err = bdb.db.View(func(txn *badger.Txn) error {
		item, err2 := txn.Get(key)
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
func (bdb *BadgerDB) Set(key, value []byte) error {
	err := bdb.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})

	if err != nil {
		return err
	}

	return nil
}

// SetEx stores the given key and value for the time given by ttl
// If the key/value pair can't be saved an error is returned
func (bdb *BadgerDB) SetEx(key, value []byte, ttl time.Duration) error {
	err := bdb.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key, value).WithTTL(ttl)
		return txn.SetEntry(e)
	})

	if err != nil {
		log.Info(err)
		return err
	}

	return nil
}

// Remove removes a single entry from the database
func (bdb *BadgerDB) Remove(prefix []byte) error {
	keysToDelete := make([][]byte, 0)

	err := bdb.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.AllVersions = false
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			keysToDelete = append(keysToDelete, it.Item().KeyCopy(nil))
		}

		return nil
	})
	if err != nil {
		return err
	}

	return bdb.db.Update(func(txn *badger.Txn) error {
		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
}

// Has implements the DB interface. It returns a boolean reflecting if the
// datbase has a given key for a namespace or not. An error is only returned if
// an error to Get would be returned that is not of type badger.ErrKeyNotFound.
func (bdb *BadgerDB) Has(key []byte) (ok bool, err error) {
	_, err = bdb.Get(key)
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
func (bdb *BadgerDB) All(prefix []byte) ([][]byte, error) {
	res := make([][]byte, 0)

	err := bdb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				data := make([]byte, len(v))
				bytesCopied := copy(data, v)
				if bytesCopied <= 0 {
					log.Warnf("not enough bytes copied: %d", bytesCopied)
				}
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
func (bdb *BadgerDB) Each(prefix []byte, callback KVStoreEachFunc) error {
	return bdb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		// prefix := bdb.badgerNamespaceKey(namespace, prefix)
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
func (bdb *BadgerDB) Count(prefix []byte) (int, error) {
	c := 0

	err := bdb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		// prefix := bdb.badgerNamespaceKey(namespace, prefix)
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

package main

import (
	"flag"
	"fmt"
	"log"

	badger "github.com/dgraph-io/badger/v3"
)

func main() {
	dbdir := flag.String("dir", "", "badger db dir")
	prefix := flag.String("prefix", "", "return all keys with this prefix")

	flag.Parse()

	opts := badger.DefaultOptions(*dbdir)
	opts.SyncWrites = true
	opts.Dir, opts.ValueDir = *dbdir, *dbdir

	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek([]byte(*prefix)); it.ValidForPrefix([]byte(*prefix)); it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				fmt.Println(string(item.Key()))
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

}

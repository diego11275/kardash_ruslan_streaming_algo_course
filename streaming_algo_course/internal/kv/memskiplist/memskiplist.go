package memskiplist

import (
	"context"

	"kvschool/internal/kv"
	"kvschool/internal/skiplist"
)

// Store — адаптер SkipList к интерфейсу kv.Store.
type Store struct {
	sl *skiplist.SkipList
}

func New(seed int64) *Store {
	return &Store{sl: skiplist.New(seed)}
}

func (s *Store) Put(_ context.Context, key, value []byte) error {
	return s.sl.Put(key, value)
}

func (s *Store) Get(_ context.Context, key []byte) ([]byte, error) {
	v, err := s.sl.Get(key)
	if err == skiplist.ErrNotFound {
		return nil, kv.ErrNotFound
	}
	return v, err
}

func (s *Store) Delete(_ context.Context, key []byte) error {
	err := s.sl.Delete(key)
	if err == skiplist.ErrNotFound {
		return nil
	}
	return err
}

func (s *Store) Scan(_ context.Context, start, end []byte) (kv.Iterator, error) {
	it, err := s.sl.Scan(start, end)
	if err != nil {
		return nil, err
	}
	return &iter{it: it}, nil
}

func (s *Store) Close() error { return nil }

type iter struct{ it skiplist.Iterator }

func (i *iter) Next() (kv.Pair, bool, error) {
	k, v, ok, err := i.it.Next()
	if err != nil || !ok {
		return kv.Pair{}, ok, err
	}
	return kv.Pair{Key: k, Value: v}, true, nil
}

func (i *iter) Close() error { return i.it.Close() }

var _ kv.Store = (*Store)(nil)

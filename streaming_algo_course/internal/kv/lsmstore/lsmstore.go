// lsmstore.go
package lsmstore

import (
	"context"
	"fmt"

	"kvschool/internal/kv"
	"kvschool/internal/lsm"
)

type Store struct {
	engine *lsm.Engine
}

type Options struct {
	Dir                    string
	MemtableFlushThreshold int
	SyncWrites             bool
	L0CompactThreshold     int
}

func Open(opts Options) (*Store, error) {
	engine, err := lsm.Open(lsm.Options{
		Dir:                    opts.Dir,
		MemtableFlushThreshold: opts.MemtableFlushThreshold,
		SyncWrites:             opts.SyncWrites,
		L0CompactThreshold:     opts.L0CompactThreshold,
	})
	if err != nil {
		return nil, fmt.Errorf("lsmstore: open: %w", err)
	}
	return &Store{engine: engine}, nil
}

func (s *Store) Put(ctx context.Context, key, value []byte) error {
	return s.engine.Put(key, value)
}

func (s *Store) Get(ctx context.Context, key []byte) ([]byte, error) {
	val, err := s.engine.Get(key)
	if err == lsm.ErrNotFound {
		return nil, kv.ErrNotFound
	}
	return val, err
}

func (s *Store) Delete(ctx context.Context, key []byte) error {
	return s.engine.Delete(key)
}

func (s *Store) Scan(ctx context.Context, start, end []byte) (kv.Iterator, error) {
	it, err := s.engine.Scan(start, end)
	if err != nil {
		return nil, err
	}
	return &scanAdapter{it}, nil
}

func (s *Store) Close() error {
	return s.engine.Close()
}

// Engine возвращает внутренний LSM‑движок (для тестов и бенчмарков).
func (s *Store) Engine() *lsm.Engine {
    return s.engine
}

func (s *Store) ForceFlush() error {
    return s.engine.ForceFlush()
}

type scanAdapter struct {
	it *lsm.Iterator
}

func (a *scanAdapter) Next() (kv.Pair, bool, error) {
	k, v, ok, err := a.it.Next()
	if err != nil || !ok {
		return kv.Pair{}, ok, err
	}
	return kv.Pair{Key: k, Value: v}, true, nil
}

func (a *scanAdapter) Close() error {
	a.it.Close()
	return nil
}

var _ kv.Store = (*Store)(nil)
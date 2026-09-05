package memmap

import (
	"bytes"
	"context"
	"sort"
	"sync"

	"kvschool/internal/kv"
)

// Store — простая in-memory реализация KV поверх map.
// Нужна, чтобы репозиторий был запускаемым до выполнения практик.
// Для высоких нагрузок она не подходит (копирование, аллокации, отсутствие амортизации записи).
type Store struct {
	mu sync.Mutex
	m  map[string][]byte
}

func New() *Store {
	return &Store{m: make(map[string][]byte)}
}

func (s *Store) Put(_ context.Context, key, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[string(key)] = append([]byte(nil), value...)
	return nil
}

func (s *Store) Get(_ context.Context, key []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[string(key)]
	if !ok {
		return nil, kv.ErrNotFound
	}
	return append([]byte(nil), v...), nil
}

func (s *Store) Delete(_ context.Context, key []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, string(key))
	return nil
}

func (s *Store) Scan(_ context.Context, start, end []byte) (kv.Iterator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(s.m))
	for k := range s.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Отфильтровываем диапазон, чтобы итератор был простым.
	filtered := make([]kv.Pair, 0, len(keys))
	for _, k := range keys {
		kb := []byte(k)
		if start != nil && bytes.Compare(kb, start) < 0 {
			continue
		}
		if end != nil && bytes.Compare(kb, end) >= 0 {
			break
		}
		filtered = append(filtered, kv.Pair{
			Key:   kb,
			Value: append([]byte(nil), s.m[k]...),
		})
	}
	return &sliceIter{pairs: filtered}, nil
}

func (s *Store) Close() error { return nil }

type sliceIter struct {
	i     int
	pairs []kv.Pair
}

func (it *sliceIter) Next() (kv.Pair, bool, error) {
	if it.i >= len(it.pairs) {
		return kv.Pair{}, false, nil
	}
	p := it.pairs[it.i]
	it.i++
	return p, true, nil
}

func (it *sliceIter) Close() error { return nil }

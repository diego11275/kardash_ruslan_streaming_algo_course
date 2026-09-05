package kv

import (
	"context"
	"errors"
)

// ErrNotFound означает, что ключ отсутствует.
var ErrNotFound = errors.New("kv: ключ не найден")

// ErrNotImplemented используется в заготовках для практик.
var ErrNotImplemented = errors.New("kv: функция не реализована")

// Pair — элемент итерации по диапазону ключей.
type Pair struct {
	Key   []byte
	Value []byte
}

// Iterator — упорядоченная итерация по ключам.
// Семантика:
// - Next возвращает (Pair, true) пока данные есть.
// - Когда элементы закончились, возвращает (Pair{}, false) и далее только false.
// - Close можно вызывать многократно.
type Iterator interface {
	Next() (Pair, bool, error)
	Close() error
}

// Store — минимальный контракт KV, достаточный для задач 1–3 дней.
// В курсе предполагается однопоточность, поэтому Store не обязан быть thread-safe.
type Store interface {
	Put(ctx context.Context, key, value []byte) error
	Get(ctx context.Context, key []byte) ([]byte, error)
	Delete(ctx context.Context, key []byte) error

	// Scan возвращает итератор по ключам в диапазоне [start, end).
	// Если start == nil, считается -∞. Если end == nil, считается +∞.
	Scan(ctx context.Context, start, end []byte) (Iterator, error)

	Close() error
}

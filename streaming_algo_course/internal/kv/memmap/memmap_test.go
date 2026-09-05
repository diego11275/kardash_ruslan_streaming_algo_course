package memmap

import (
	"context"
	"testing"

	"kvschool/internal/kv"
)

func TestStore_PutGetDelete(t *testing.T) {
	ctx := context.Background()
	s := New()
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Put(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, []byte("a"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "1" {
		t.Fatalf("value mismatch: got=%q want=%q", string(got), "1")
	}
	if err := s.Delete(ctx, []byte("a")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Get(ctx, []byte("a"))
	if err != kv.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_ScanRange(t *testing.T) {
	ctx := context.Background()
	s := New()
	t.Cleanup(func() { _ = s.Close() })

	_ = s.Put(ctx, []byte("a"), []byte("1"))
	_ = s.Put(ctx, []byte("b"), []byte("2"))
	_ = s.Put(ctx, []byte("c"), []byte("3"))

	it, err := s.Scan(ctx, []byte("b"), []byte("d"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	t.Cleanup(func() { _ = it.Close() })

	p, ok, err := it.Next()
	if err != nil || !ok {
		t.Fatalf("Next1: ok=%v err=%v", ok, err)
	}
	if string(p.Key) != "b" {
		t.Fatalf("key1 mismatch: %q", string(p.Key))
	}
	p, ok, err = it.Next()
	if err != nil || !ok {
		t.Fatalf("Next2: ok=%v err=%v", ok, err)
	}
	if string(p.Key) != "c" {
		t.Fatalf("key2 mismatch: %q", string(p.Key))
	}
	_, ok, err = it.Next()
	if err != nil || ok {
		t.Fatalf("Next3: ok=%v err=%v", ok, err)
	}
}

//go:build day2

package lsmstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"math/rand"

	"kvschool/internal/kv"
)

func TestLSMStore_PersistAcrossRestart(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "db")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Put(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open2: %v", err)
	}
	defer s2.Close()

	got, err := s2.Get(ctx, []byte("a"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "1" {
		t.Fatalf("value mismatch: got=%q want=%q", string(got), "1")
	}
}


func tempDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "lsmstore-edge")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func openStore(t *testing.T, dir string, sync bool) *Store {
	t.Helper()
	s, err := Open(Options{
		Dir:                    dir,
		MemtableFlushThreshold: 1 << 20, // большой порог, чтобы не мешал тестам
		SyncWrites:             sync,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestEdge_EmptyKeyAndValue(t *testing.T) {
	s := openStore(t, tempDir(t), true)
	ctx := context.Background()

	// пустой ключ
	if err := s.Put(ctx, []byte{}, []byte("v")); err != nil {
		t.Fatal(err)
	}
	v, err := s.Get(ctx, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	if string(v) != "v" {
		t.Fatalf("got %q, want 'v'", v)
	}
	// пустое значение
	if err := s.Put(ctx, []byte("key"), []byte{}); err != nil {
		t.Fatal(err)
	}
	v2, err := s.Get(ctx, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if len(v2) != 0 {
		t.Fatalf("expected empty value, got %x", v2)
	}
}

func TestEdge_RangeScans(t *testing.T) {
	s := openStore(t, tempDir(t), true)
	ctx := context.Background()
	// добавляем ключи "a", "b", "c", "d"
	for _, k := range []string{"a", "b", "c", "d"} {
		if err := s.Put(ctx, []byte(k), []byte(k)); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("start after end", func(t *testing.T) {
		it, err := s.Scan(ctx, []byte("c"), []byte("a"))
		if err != nil {
			t.Fatal(err)
		}
		defer it.Close()
		_, ok, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatalf("expected no results, got a pair")
		}
	})

	t.Run("start == end", func(t *testing.T) {
		it, err := s.Scan(ctx, []byte("b"), []byte("b"))
		if err != nil {
			t.Fatal(err)
		}
		defer it.Close()
		_, ok, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatalf("expected no results for [b,b)")
		}
	})

	t.Run("nil start", func(t *testing.T) {
		it, err := s.Scan(ctx, nil, []byte("c"))
		if err != nil {
			t.Fatal(err)
		}
		defer it.Close()
		p, ok, err := it.Next()
		if err != nil || !ok {
			t.Fatalf("expected first key 'a'")
		}
		if string(p.Key) != "a" {
			t.Fatalf("got %s", p.Key)
		}
	})

	t.Run("nil end", func(t *testing.T) {
		it, err := s.Scan(ctx, []byte("c"), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer it.Close()
		var keys []string
		for {
			p, ok, err := it.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			keys = append(keys, string(p.Key))
		}
		if len(keys) != 2 || keys[0] != "c" || keys[1] != "d" {
			t.Fatalf("unexpected keys: %v", keys)
		}
	})
}

func TestEdge_OverwriteAndDelete(t *testing.T) {
	s := openStore(t, tempDir(t), true)
	ctx := context.Background()

	key := []byte("update.me")
	// overwrite several times
	for _, v := range [][]byte{[]byte("1"), []byte("2"), []byte("3")} {
		if err := s.Put(ctx, key, v); err != nil {
			t.Fatal(err)
		}
	}
	v, err := s.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(v) != "3" {
		t.Fatalf("got %s", v)
	}

	// delete
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(ctx, key)
	if err != kv.ErrNotFound {
		t.Fatalf("expected ErrNotFound after deletion, got %v", err)
	}

	// scan: deleted key must not appear
	it, err := s.Scan(ctx, []byte("u"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	_, ok, _ := it.Next()
	if ok {
		t.Fatalf("deleted key appeared in scan")
	}
}

func TestEdge_TombstoneThroughFlush(t *testing.T) {
	// Маленький порог flush, чтобы tombstones попали в SSTable
	dir := tempDir(t)
	s, err := Open(Options{
		Dir:                    dir,
		MemtableFlushThreshold: 64,
		SyncWrites:             true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	key := []byte("ghost")
	if err := s.Put(ctx, key, []byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	// Заставляем flush (добавим ещё данных, чтобы превысить порог)
	if err := s.Put(ctx, []byte("padding"), make([]byte, 100)); err != nil {
		t.Fatal(err)
	}
	// После flush ключ должен быть невидим
	_, err = s.Get(ctx, key)
	if err != kv.ErrNotFound {
		t.Fatalf("expected ErrNotFound after flush+tombstone, got %v", err)
	}
}


func benchStore(b *testing.B, sync bool) *Store {
	b.Helper()
	dir, err := os.MkdirTemp("", "lsmstore-bench")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { os.RemoveAll(dir) })
	s, err := Open(Options{
		Dir:                    dir,
		MemtableFlushThreshold: 100 * 1024 * 1024, // большой, чтобы избежать flush во время замера
		SyncWrites:             sync,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { s.Close() })
	return s
}

func BenchmarkPutSync(b *testing.B) {
	s := benchStore(b, true)
	ctx := context.Background()
	key := []byte("key")
	val := []byte("value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.Put(ctx, key, val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPutNoSync(b *testing.B) {
	s := benchStore(b, false)
	ctx := context.Background()
	key := []byte("key")
	val := []byte("value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.Put(ctx, key, val); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPutLSM измеряет вставку N уникальных ключей в LSM‑хранилище.
// Тренд: амортизированное O(N) – при росте N время на один Put должно оставаться
// примерно постоянным, несмотря на периодические flush'и и compaction.
func BenchmarkPutLSMNoSync(b *testing.B) {
    // sizes := []int{1000, 5000, 10000, 50000, 100000, 200000}
    // Можно также генерировать размеры прогрессивно:
    sizes := make([]int, 30)
	for i := 0; i < 30; i++ {
        sizes[i] = 10000 + (500000)*i/30
    }

    for _, size := range sizes {
        b.Run(fmt.Sprintf("N=%d", size), func(b *testing.B) {
            // для каждого замера создаём свежее хранилище (в tempDir)
            dir := b.TempDir()
            store, err := Open(Options{
                Dir:                    dir,
                MemtableFlushThreshold: 1 << 20, // 1 MB – достаточно велик, чтобы не флашить слишком часто
                SyncWrites:             false,    // отключаем fsync для чистоты измерения
            })
            if err != nil {
                b.Fatal(err)
            }
            defer store.Close()
            ctx := context.Background()

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                // В одном бенч‑цикле вставляем size элементов
                for j := 0; j < size; j++ {
                    key := []byte(fmt.Sprintf("key%08d", j))
                    val := key // значение равно ключу
                    if err := store.Put(ctx, key, val); err != nil {
                        b.Fatal(err)
                    }
                }
            }
            b.StopTimer()
        })
    }
}


// BenchmarkPutLSM измеряет вставку N уникальных ключей в LSM‑хранилище.
// Тренд: амортизированное O(N) – при росте N время на один Put должно оставаться
// примерно постоянным, несмотря на периодические flush'и и compaction.
func BenchmarkPutLSMSync(b *testing.B) {
    // sizes := []int{1000, 5000, 10000, 50000, 100000, 200000}
    // Можно также генерировать размеры прогрессивно:
    sizes := make([]int, 30)
	for i := 0; i < 30; i++ {
        sizes[i] = 10000 + (500000)*i/30
    }

    for _, size := range sizes {
        b.Run(fmt.Sprintf("N=%d", size), func(b *testing.B) {
            // для каждого замера создаём свежее хранилище (в tempDir)
            dir := b.TempDir()
            store, err := Open(Options{
                Dir:                    dir,
                MemtableFlushThreshold: 1 << 20, // 1 MB – достаточно велик, чтобы не флашить слишком часто
                SyncWrites:             true,    
            })
            if err != nil {
                b.Fatal(err)
            }
            defer store.Close()
            ctx := context.Background()

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                // В одном бенч‑цикле вставляем size элементов
                for j := 0; j < size; j++ {
                    key := []byte(fmt.Sprintf("key%08d", j))
                    val := key // значение равно ключу
                    if err := store.Put(ctx, key, val); err != nil {
                        b.Fatal(err)
                    }
                }
            }
            b.StopTimer()
        })
    }
}


// BenchmarkDeleteLSM измеряет удаление N ранее добавленных ключей.
// Тренд: также O(N) амортизированно (каждый Delete пишет tombstone).
func BenchmarkDeleteLSM(b *testing.B) {
    // sizes := []int{1000, 5000, 10000, 50000, 100000}
	sizes := make([]int, 30)
	for i := 0; i < 30; i++ {
        sizes[i] = 10000 + (500000)*i/30
    }


    for _, size := range sizes {
        b.Run(fmt.Sprintf("N=%d", size), func(b *testing.B) {
            dir := b.TempDir()
            store, err := Open(Options{
                Dir:                    dir,
                MemtableFlushThreshold: 1 << 20,
                SyncWrites:             false,
            })
            if err != nil {
                b.Fatal(err)
            }
            defer store.Close()
            ctx := context.Background()

            // Предварительно заполняем хранилище ключами
            keys := make([][]byte, size)
            for i := 0; i < size; i++ {
                key := []byte(fmt.Sprintf("key%08d", i))
                keys[i] = key
                if err := store.Put(ctx, key, key); err != nil {
                    b.Fatal(err)
                }
            }
            // Перемешиваем порядок удаления (опционально, чтобы избежать паттерна последовательного доступа)
            rand.Shuffle(size, func(i, j int) {
                keys[i], keys[j] = keys[j], keys[i]
            })

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                // Удаляем все size ключей
                for j := 0; j < size; j++ {
                    if err := store.Delete(ctx, keys[j]); err != nil {
                        b.Fatal(err)
                    }
                }
            }
            b.StopTimer()
        })
    }
}

func BenchmarkGet(b *testing.B) {
	s := benchStore(b, true)
	ctx := context.Background()
	key := []byte("k")
	val := []byte("v")
	if err := s.Put(ctx, key, val); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Get(ctx, key); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanRange(b *testing.B) {
	s := benchStore(b, true)
	ctx := context.Background()
	// предзаполним 1000 ключей
	for i := 0; i < 1000; i++ {
		k := []byte(fmt.Sprintf("key%06d", i))
		if err := s.Put(ctx, k, k); err != nil {
			b.Fatal(err)
		}
	}
	start := []byte("key000500")
	end := []byte("key000600")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it, err := s.Scan(ctx, start, end)
		if err != nil {
			b.Fatal(err)
		}
		for {
			_, ok, err := it.Next()
			if err != nil {
				b.Fatal(err)
			}
			if !ok {
				break
			}
		}
		it.Close()
	}
}
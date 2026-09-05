//go:build day2

package lsmstore

import (
    "context"
    "fmt"
    "path/filepath"
    "testing"
)

func BenchmarkScanWithManyL0(b *testing.B) {
    for _, numL0 := range []int{10, 20, 50,100,200,400,800} {
        b.Run(fmt.Sprintf("L0=%d", numL0), func(b *testing.B) {
            dir := b.TempDir()
            store, err := Open(Options{
                Dir:                    dir,
                MemtableFlushThreshold: 100 * 1024, // 100 KB – достаточно для 100 ключей
                SyncWrites:             false,
                L0CompactThreshold:     1 << 30,    // отключаем компакшн (огромный порог)
            })
            if err != nil {
                b.Fatal(err)
            }
            defer store.Close()
            ctx := context.Background()

            keysPerTable := 100
            for l := 0; l < numL0; l++ {
                for i := 0; i < keysPerTable; i++ {
                    key := []byte(fmt.Sprintf("key_%d_%d", l, i))
                    if err := store.Put(ctx, key, key); err != nil {
                        b.Fatal(err)
                    }
                }
                // Принудительный flush – создаём один SSTable
                if err := store.ForceFlush(); err != nil {
                    b.Fatal(err)
                }
            }

            // Проверяем количество L0-файлов (игнорируем level1.sst, если появился)
            files, _ := filepath.Glob(filepath.Join(dir, "[0-9]*.sst"))
            if len(files) != numL0 {
                b.Fatalf("expected %d L0 files, got %d", numL0, len(files))
            }

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                it, err := store.Scan(ctx, nil, nil)
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
        })
    }
}
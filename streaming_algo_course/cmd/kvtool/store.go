package main

import (
	"context"
	"fmt"
	"sort"

	"kvschool/internal/kv"
	"kvschool/internal/kv/memskiplist"
	"kvschool/internal/stream"
	"kvschool/internal/testutil"
)

func memSkipListDefault() kv.Store {
	// seed=1 чтобы поведение было воспроизводимым в тестах.
	return memskiplist.New(1)
}

// runLoadWithReport загружает данные, обновляя Count-Min Sketch, и выводит топ-10 ключей по оценённой частоте.
func runLoadWithReport(ctx context.Context, st kv.Store, count int, keyGen testutil.KeyGenerator) error {
	// Создаём Count-Min Sketch с параметрами: width=2048, depth=4, seed=12345
	// Это даёт приемлемую память (~64KB) и погрешность ~count/width.
	cms := stream.NewCountMinSketch(2048, 4, 12345)

	// Загружаем данные: все операции Put (чтобы ключи были в хранилище)
	for i := 0; i < count; i++ {
		key := keyGen.Next()
		value := []byte(fmt.Sprintf("value_%d", i))
		if err := st.Put(ctx, key, value); err != nil {
			return fmt.Errorf("ошибка Put: %w", err)
		}
		cms.Add(key)
	}

	// Получаем все ключи из хранилища (можно было бы использовать keyGen, но хранилище может иметь дубликаты)
	it, err := st.Scan(ctx, nil, nil)
	if err != nil {
		return fmt.Errorf("ошибка Scan: %w", err)
	}
	defer it.Close()

	type item struct {
		key  string
		freq uint64
	}
	var items []item

	for {
		pair, ok, err := it.Next()
		if err != nil {
			return fmt.Errorf("ошибка итератора: %w", err)
		}
		if !ok {
			break
		}
		est, _ := cms.Estimate(pair.Key)
		items = append(items, item{string(pair.Key), est})
	}

	// Сортировка по убыванию частоты
	sort.Slice(items, func(i, j int) bool {
		return items[i].freq > items[j].freq
	})

	fmt.Println("\n=== Top-10 keys by estimated frequency (Count-Min Sketch) ===")
	for i, it := range items {
		if i >= 10 {
			break
		}
		fmt.Printf("%2d. %s ~ %d\n", i+1, it.key, it.freq)
	}
	fmt.Println("============================================================")

	return nil
}
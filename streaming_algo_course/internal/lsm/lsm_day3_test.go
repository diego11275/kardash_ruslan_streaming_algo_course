//go:build day3

package lsm

import (
    "bytes"
    "fmt"
    "testing"
    "time"

    "kvschool/internal/bloom"
    "kvschool/internal/sstable"
)


// mockSlowSSTable имитирует SSTable, чтение которого занимает 50 микросекунд.
type mockSlowSSTable struct {
    key   []byte
    value []byte
    bloom *bloom.Filter
}

func (m *mockSlowSSTable) Lookup(key []byte) ([]byte, bool, byte, error) {
    // Симуляция дискового чтения: 50 µs
    time.Sleep(50 * time.Microsecond)
    if bytes.Equal(key, m.key) {
        return m.value, true, sstable.RecordTypePut, nil
    }
    return nil, false, 0, nil
}

func (m *mockSlowSSTable) Iterator(start, end []byte) (*sstable.Iter, error) {
    return nil, nil
}
func (m *mockSlowSSTable) Close() error { return nil }

// BenchmarkDDoSWithMockTables эмулирует DDoS-атаку на 200 SSTable с дисковыми задержками.
func BenchmarkDDoSWithMockTables(b *testing.B) {
    const numTables = 200
    const keysPerTable = 1000

    // Создаём моки без Bloom
    mocksNoBloom := make([]*mockSlowSSTable, numTables)
    for i := 0; i < numTables; i++ {
        mocksNoBloom[i] = &mockSlowSSTable{
            key:   []byte(fmt.Sprintf("key_%d_%d", i, 0)), // каждый мок содержит только один ключ
            value: []byte("v"),
            bloom: nil,
        }
    }

    // Создаём моки с Bloom-фильтром, который содержит все keysPerTable ключей
    mocksWithBloom := make([]*mockSlowSSTable, numTables)
    for i := 0; i < numTables; i++ {
        bf := bloom.New(1024*8, 7) // 1% false positive для 1000 элементов
        for j := 0; j < keysPerTable; j++ {
            key := []byte(fmt.Sprintf("key_%d_%d", i, j))
            _ = bf.Add(key)
        }
        mocksWithBloom[i] = &mockSlowSSTable{
            key:   []byte(fmt.Sprintf("key_%d_%d", i, 0)),
            value: []byte("v"),
            bloom: bf,
        }
    }

    // Ключ, которого нет ни в одной таблице
    missingKey := []byte("nonexistent")

    b.Run("NoBloom", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            for _, m := range mocksNoBloom {
                // без фильтра всегда ходим в Lookup
                _, _, _, _ = m.Lookup(missingKey)
            }
        }
    })

    b.Run("WithBloom", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            for _, m := range mocksWithBloom {
                // сначала проверяем фильтр
                if m.bloom != nil {
                    maybe, _ := m.bloom.MayContain(missingKey)
                    if !maybe {
                        continue // ключа точно нет, экономим диск
                    }
                }
                _, _, _, _ = m.Lookup(missingKey)
            }
        }
    })
}

// BenchmarkLSMGetMissWithManyTables измеряет время Get отсутствующего ключа
// при большом количестве SSTable (100 файлов) с Bloom-фильтром и без.
func BenchmarkLSMGetMissWithManyTables(b *testing.B) {
    tests := []struct {
        name        string
        useBloom    bool
        compactThr  int // отключаем compaction
    }{
        {"WithBloom", true, 1000},
        {"NoBloom", false, 1000},
    }

    for _, tc := range tests {
        b.Run(tc.name, func(b *testing.B) {
            dir := b.TempDir()
            opts := Options{
                Dir:                    dir,
                MemtableFlushThreshold: 128,   // маленький порог – каждый Put будет вызывать flush
                SyncWrites:             false,
                L0CompactThreshold:     tc.compactThr, // отключаем компакшен
                BloomFalsePositiveRate: 0.01,
            }
            engine, err := Open(opts)
            if err != nil {
                b.Fatal(err)
            }
            defer engine.Close()

            // Если не используем bloom, обнулим фильтры
            if !tc.useBloom {
                engine.mu.Lock()
                for i := range engine.sstBloom {
                    engine.sstBloom[i] = nil
                }
                engine.l1Bloom = nil
                engine.mu.Unlock()
            }

            // Вставляем 2000 ключей, чтобы создать ~2000/(128/средняя длина ключа) ≈ 2000/4 ≈ 500 SSTable
            // но для стабильности создадим 100 SSTable вручную через ForceFlush
            for i := 0; i < 100; i++ {
                // Каждый раз заливаем достаточно данных, чтобы вызвать flush
                for j := 0; j < 32; j++ {
                    key := []byte(fmt.Sprintf("key_%d_%d", i, j))
                    if err := engine.Put(key, []byte("v")); err != nil {
                        b.Fatal(err)
                    }
                }
                if err := engine.ForceFlush(); err != nil {
                    b.Fatal(err)
                }
            }

            // Проверяем, что действительно создалось много SSTable
            engine.mu.Lock()
            numSST := len(engine.sstables)
            engine.mu.Unlock()
            if numSST < 50 {
                b.Logf("Warning: only %d SSTables created, expected >=50", numSST)
            }

            missingKey := []byte("nonexistent")
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                _, _ = engine.Get(missingKey)
            }
        })
    }
}

func TestBloom_RealLSM(t *testing.T) {
    dir := t.TempDir()
    opts := Options{
        Dir:                    dir,
        MemtableFlushThreshold: 100,
        L0CompactThreshold:     100,
        BloomFalsePositiveRate: 0.01,
    }
    engine, err := Open(opts)
    if err != nil {
        t.Fatal(err)
    }
    defer engine.Close()

    // вставляем 2000 ключей -> должно быть ~20 SSTable
    for i := 0; i < 2000; i++ {
        if err := engine.Put([]byte(fmt.Sprintf("key_%d", i)), []byte("v")); err != nil {
            t.Fatal(err)
        }
    }
    if err := engine.ForceFlush(); err != nil {
        t.Fatal(err)
    }

    // считаем обращения к SSTable через мок? сложно, но хотя бы проверим, что bloom не nil
    engine.mu.Lock()
    for i, bf := range engine.sstBloom {
        if bf == nil && engine.sstables[i] != nil {
            t.Errorf("SSTable %d has nil bloom", i)
        }
    }
    engine.mu.Unlock()

    // проверяем, что несуществующий ключ не вызывает панику и возвращает ErrNotFound
    _, err = engine.Get([]byte("nonexistent"))
    if err != ErrNotFound {
        t.Errorf("expected ErrNotFound, got %v", err)
    }
}

// mockSSTableReader имитирует SSTable с подсчётом вызовов Lookup
type mockSSTableReader struct {
	reads    int
	key      []byte
	value    []byte
	hasKey   bool
	bloom    *bloom.Filter
}

func (m *mockSSTableReader) Lookup(key []byte) ([]byte, bool, byte, error) {
	m.reads++
	if m.hasKey && string(key) == string(m.key) {
		return m.value, true, sstable.RecordTypePut, nil
	}
	return nil, false, 0, nil
}

func (m *mockSSTableReader) Iterator(start, end []byte) (*sstable.Iter, error) {
	return nil, nil
}
func (m *mockSSTableReader) Close() error { return nil }

// TestDDoSWithBloom демонстрирует сокращение обращений к диску благодаря Bloom Filter
func TestDDoSWithBloom(t *testing.T) {
	// Создаём 100 фейковых SSTable, каждый содержит 1000 уникальных ключей
	const numSSTables = 100
	const keysPerSSTable = 1000

	// Список моков SSTable
	mocks := make([]*mockSSTableReader, numSSTables)
	blooms := make([]*bloom.Filter, numSSTables)

	for i := 0; i < numSSTables; i++ {
		m := &mockSSTableReader{
			key:    []byte(fmt.Sprintf("key_%d_%d", i, 0)),
			value:  []byte("value"),
			hasKey: true,
		}
		// Создаём bloom-фильтр для этого SSTable, содержащий все 1000 ключей
		bf := bloom.New(1024*8, 7) // ~1% false positive для 1000 элементов
		for j := 0; j < keysPerSSTable; j++ {
			key := []byte(fmt.Sprintf("key_%d_%d", i, j))
			_ = bf.Add(key)
		}
		m.bloom = bf
		mocks[i] = m
		blooms[i] = bf
	}

	// Эмулируем DDoS: 10 000 запросов несуществующих ключей
	ddosKeys := make([][]byte, 10000)
	for i := 0; i < 10000; i++ {
		ddosKeys[i] = []byte(fmt.Sprintf("nonexistent_%d", i))
	}

	// Случай 1: с Bloom Filter
	readsWithBloom := 0
	for _, key := range ddosKeys {
		for i, m := range mocks {
			if blooms[i] != nil {
				maybe, _ := blooms[i].MayContain(key)
				if !maybe {
					continue
				}
			}
			_, found, _, _ := m.Lookup(key)
			if found {
				// не должно случиться
			}
		}
	}
	// Суммируем обращения
	for _, m := range mocks {
		readsWithBloom += m.reads
		m.reads = 0 // сброс для следующего эксперимента
	}

	// Случай 2: без Bloom Filter
	readsWithoutBloom := 0
	for _, key := range ddosKeys {
		for _, m := range mocks {
			_, found, _, _ := m.Lookup(key)
			if found {
			}
		}
	}
	for _, m := range mocks {
		readsWithoutBloom += m.reads
	}

	t.Logf("Чтений с Bloom: %d", readsWithBloom)
	t.Logf("Чтений без Bloom: %d", readsWithoutBloom)

	// Ожидаем, что с Bloom число обращений будет не более (numSSTables * len(ddosKeys) * falsePositiveRate)
	// Для falsePositiveRate ~1% и 100 SSTable, 10000 запросов -> ожидаем ~10000 * 100 * 0.01 = 10000 обращений
	// Без Bloom: 10000 * 100 = 1 000 000.
	// Устанавливаем порог: с Bloom должно быть менее 5% от числа без Bloom (т.е. < 50000)
	if readsWithBloom >= readsWithoutBloom/20 {
		t.Errorf("Bloom filter неэффективен: %d обращений против %d без фильтра", readsWithBloom, readsWithoutBloom)
	} else {
		t.Logf("Bloom filter сократил обращения в %d раз", readsWithoutBloom/(readsWithBloom+1))
	}
}

// BenchmarkLSMGetMissWithBloom - замер времени Get несуществующего ключа в LSM с Bloom
func BenchmarkLSMGetMissWithBloom(b *testing.B) {
	dir := b.TempDir()
	opts := Options{
		Dir:                    dir,
		MemtableFlushThreshold: 1024, // маленький порог, чтобы создать много SSTable
		SyncWrites:             false,
		L0CompactThreshold:     10,
		BloomFalsePositiveRate: 0.01,
	}
	engine, err := Open(opts)
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	// Заполняем 10000 уникальных ключей, вызывая flush
	for i := 0; i < 10000; i++ {
		key := []byte(fmt.Sprintf("key_%d", i))
		if err := engine.Put(key, []byte("value")); err != nil {
			b.Fatal(err)
		}
	}
	// Принудительно флашим остатки
	engine.mu.Lock()
	if engine.memSize > 0 {
		_ = engine.flushMemtable()
	}
	engine.mu.Unlock()

	// Проверяем несуществующий ключ
	missingKey := []byte("nonexistent")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Get(missingKey)
	}
}

// BenchmarkLSMGetMissNoBloom - то же без Bloom (отключаем фильтр)
func BenchmarkLSMGetMissNoBloom(b *testing.B) {
	dir := b.TempDir()
	opts := Options{
		Dir:                    dir,
		MemtableFlushThreshold: 1024,
		SyncWrites:             false,
		L0CompactThreshold:     10,
		BloomFalsePositiveRate: 0.5, // высокое значение, чтобы фильтр практически не отсеивал (или nil)
	}
	engine, err := Open(opts)
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	// Отключаем bloom-фильтры вручную (можно просто не использовать их, но параметр 0.5 даст большой FP)
	// Для чистоты эксперимента обнулим фильтры
	for i := range engine.sstBloom {
		engine.sstBloom[i] = nil
	}
	engine.l1Bloom = nil

	for i := 0; i < 10000; i++ {
		key := []byte(fmt.Sprintf("key_%d", i))
		if err := engine.Put(key, []byte("value")); err != nil {
			b.Fatal(err)
		}
	}
	engine.mu.Lock()
	if engine.memSize > 0 {
		_ = engine.flushMemtable()
	}
	engine.mu.Unlock()

	missingKey := []byte("nonexistent")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Get(missingKey)
	}
}

func BenchmarkBloomAddOptimal(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			m, k, _ := bloom.OptimalParams(uint64(n), 0.01)
			f := bloom.New(m, k)
			key := []byte("test")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = f.Add(key)
			}
		})
	}
}

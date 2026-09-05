//go:build day1

package skiplist

import (
	"bytes"
	"testing"
	"fmt"
	// "math/rand"
)

func TestSkipList_BasicCRUD(t *testing.T) {
	sl := New(1)

	if err := sl.Put([]byte("b"), []byte("2")); err != nil {
		t.Fatalf("Put b: %v", err)
	}
	if err := sl.Put([]byte("a"), []byte("1")); err != nil {
		t.Fatalf("Put a: %v", err)
	}
	if err := sl.Put([]byte("c"), []byte("3")); err != nil {
		t.Fatalf("Put c: %v", err)
	}

	v, err := sl.Get([]byte("a"))
	if err != nil {
		t.Fatalf("Get a: %v", err)
	}
	if !bytes.Equal(v, []byte("1")) {
		t.Fatalf("Get a mismatch: %q", string(v))
	}

	if err := sl.Delete([]byte("b")); err != nil {
		t.Fatalf("Delete b: %v", err)
	}
	_, err = sl.Get([]byte("b"))
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSkipList_ScanOrderAndRange(t *testing.T) {
	sl := New(1)
	_ = sl.Put([]byte("a"), []byte("1"))
	_ = sl.Put([]byte("b"), []byte("2"))
	_ = sl.Put([]byte("c"), []byte("3"))

	it, err := sl.Scan([]byte("b"), []byte("d"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	defer it.Close()

	var keys []string
	for {
		k, _, ok, err := it.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
		keys = append(keys, string(k))
	}
	if len(keys) != 2 || keys[0] != "b" || keys[1] != "c" {
		t.Fatalf("unexpected keys: %#v", keys)
	}
}

// ---------------------------------------------------------------------
// Тесты на крайние случаи (уровень B)
// ---------------------------------------------------------------------

// Проверка обновления существующего ключа (повторная запись)
func TestSkipList_UpdateKey(t *testing.T) {
	sl := New(42)
	key := []byte("imsi")
	val1 := []byte("first")
	val2 := []byte("updated")

	if err := sl.Put(key, val1); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	if err := sl.Put(key, val2); err != nil {
		t.Fatalf("second Put: %v", err)
	}
	got, err := sl.Get(key)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if !bytes.Equal(got, val2) {
		t.Errorf("expected %q, got %q", val2, got)
	}
}

// Пустые ключи и значения
func TestSkipList_EmptyKeyValue(t *testing.T) {
	sl := New(1)

	// пустой ключ
	emptyKey := []byte{}
	val := []byte("something")
	if err := sl.Put(emptyKey, val); err != nil {
		t.Fatalf("Put empty key: %v", err)
	}
	got, err := sl.Get(emptyKey)
	if err != nil {
		t.Fatalf("Get empty key: %v", err)
	}
	if !bytes.Equal(got, val) {
		t.Errorf("expected %q, got %q", val, got)
	}

	// пустое значение
	key := []byte("key")
	emptyVal := []byte{}
	if err := sl.Put(key, emptyVal); err != nil {
		t.Fatalf("Put empty value: %v", err)
	}
	got, err = sl.Get(key)
	if err != nil {
		t.Fatalf("Get empty value: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty value, got %q", got)
	}
}

// Диапазоны Scan: start=nil, end=nil, nil с одной стороны
func TestSkipList_ScanNilBoundaries(t *testing.T) {
	sl := New(1)
	// вставляем несколько ключей
	keys := []string{"a", "b", "c", "d"}
	for _, k := range keys {
		_ = sl.Put([]byte(k), []byte(k))
	}

	t.Run("start=nil", func(t *testing.T) {
		it, err := sl.Scan(nil, []byte("c"))
		if err != nil {
			t.Fatal(err)
		}
		defer it.Close()
		var out []string
		for {
			k, _, ok, err := it.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			out = append(out, string(k))
		}
		expected := []string{"a", "b"}
		if !sliceEqual(out, expected) {
			t.Errorf("start=nil: got %v, want %v", out, expected)
		}
	})

	t.Run("end=nil", func(t *testing.T) {
		it, err := sl.Scan([]byte("b"), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer it.Close()
		var out []string
		for {
			k, _, ok, err := it.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			out = append(out, string(k))
		}
		expected := []string{"b", "c", "d"}
		if !sliceEqual(out, expected) {
			t.Errorf("end=nil: got %v, want %v", out, expected)
		}
	})

	t.Run("both nil", func(t *testing.T) {
		it, err := sl.Scan(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer it.Close()
		var out []string
		for {
			k, _, ok, err := it.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			out = append(out, string(k))
		}
		expected := []string{"a", "b", "c", "d"}
		if !sliceEqual(out, expected) {
			t.Errorf("both nil: got %v, want %v", out, expected)
		}
	})
}

// Пустой диапазон (start >= end) или диапазон без элементов
func TestSkipList_ScanEmptyRange(t *testing.T) {
	sl := New(1)
	_ = sl.Put([]byte("a"), []byte("1"))
	_ = sl.Put([]byte("b"), []byte("2"))
	_ = sl.Put([]byte("c"), []byte("3"))

	// start == end
	it, err := sl.Scan([]byte("b"), []byte("b"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	_, _, ok, err := it.Next()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected empty iterator for start==end, but got element")
	}

	// start > end
	it, err = sl.Scan([]byte("c"), []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	_, _, ok, err = it.Next()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected empty iterator for start>end, but got element")
	}

	// диапазон между существующими ключами, но без элементов
	it, err = sl.Scan([]byte("ab"), []byte("ac"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	_, _, ok, err = it.Next()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected empty iterator for gap range, but got element")
	}
}

// Удаление: повторное удаление, удаление несуществующего ключа
func TestSkipList_DeleteEdgeCases(t *testing.T) {
	sl := New(1)
	key := []byte("x")
	_ = sl.Put(key, []byte("val"))

	// удаление существующего
	if err := sl.Delete(key); err != nil {
		t.Fatalf("delete existing: %v", err)
	}
	// повторное удаление того же ключа
	if err := sl.Delete(key); err != ErrNotFound {
		t.Errorf("second delete: expected ErrNotFound, got %v", err)
	}
	// удаление несуществующего
	if err := sl.Delete([]byte("none")); err != ErrNotFound {
		t.Errorf("delete nonexistent: expected ErrNotFound, got %v", err)
	}
}

// Scan после удаления некоторых ключей
func TestSkipList_ScanAfterDeletes(t *testing.T) {
	sl := New(1)
	_ = sl.Put([]byte("a"), []byte("1"))
	_ = sl.Put([]byte("b"), []byte("2"))
	_ = sl.Put([]byte("c"), []byte("3"))
	_ = sl.Put([]byte("d"), []byte("4"))

	// удаляем b и d
	_ = sl.Delete([]byte("b"))
	_ = sl.Delete([]byte("d"))

	it, err := sl.Scan([]byte("a"), []byte("e"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	var keys []string
	for {
		k, _, ok, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		keys = append(keys, string(k))
	}
	expected := []string{"a", "c"}
	if !sliceEqual(keys, expected) {
		t.Errorf("after deletions: got %v, want %v", keys, expected)
	}
}

// ---------------------------------------------------------------------
// Бенчмарки (уровень B)
// ---------------------------------------------------------------------

// Вспомогательная функция для генерации ключей
func makeKey(i int) []byte {
	return []byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
}

// BenchmarkPut измеряет вставку N элементов.
// Тренд: O(log N) – рост должен замедляться.
// func BenchmarkPut(b *testing.B) {
// 	sizes := make([]int, 30)
// 	for i := 0; i < 30; i++ {
//     	sizes[i] = 100000+(500000)*i/30
// 	}
	
// 	for _, size := range sizes {
// 		b.Run(fmt.Sprintf("N=%d", size), func(b *testing.B) {
// 			// для каждого замера создаём новый скиплист
// 			sl := New(1)
// 			b.ResetTimer()
// 			for i := 0; i < b.N; i++ {
// 				for j:=0; j<size;j++{
// 					key := makeKey(j)
// 					val := key
// 					_ = sl.Put(key, val)
				
// 				}
// 			}
// 		})
// 	}
// }


// BenchmarkDelete измеряет удаление всех элементов из SkipList.
// Тренд: O(log N) на операцию удаления, поэтому общее время удаления N элементов должно быть O(N log N).
// func BenchmarkDelete(b *testing.B) {
//     sizes := make([]int, 30)
//     for i := 0; i < 30; i++ {
//         sizes[i] = 100000 + (500000)*i/30
//     }

//     for _, size := range sizes {
//         b.Run(fmt.Sprintf("N=%d", size), func(b *testing.B) {
//             // Фиксированный генератор для перемешивания ключей
//             rng := rand.New(rand.NewSource(42))

//             for iter := 0; iter < b.N; iter++ {
//                 b.StopTimer()
//                 sl := New(1)

//                 // Заранее генерируем и сохраняем ключи
//                 keys := make([][]byte, size)
//                 for j := 0; j < size; j++ {
//                     key := makeKey(j)
//                     keys[j] = key
//                     _ = sl.Put(key, key)
//                 }

//                 // Перемешиваем порядок удаления
//                 rng.Shuffle(size, func(i, j int) {
//                     keys[i], keys[j] = keys[j], keys[i]
//                 })

//                 // ----- ИЗМЕРЕНИЕ УДАЛЕНИЯ -----
//                 b.StartTimer()
//                 for j := 0; j < size; j++ {
//                     _ = sl.Delete(keys[j])
//                 }
//                 b.StopTimer()
//                 // ---------------------------------
//             }
//         })
//     }
// }

// вспомогательная функция для сравнения срезов строк
func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// putWithRandomLevel – полная копия Put, но вызывающая randomLevel.
// Это позволяет изолировать эффект именно от способа генерации уровня,
// оставляя идентичной всю остальную логику вставки.
func (s *SkipList) putWithRandomLevel(key, value []byte) error {
    keyCopy := make([]byte, len(key))
    copy(keyCopy, key)
    var valueCopy []byte
    if value != nil {
        valueCopy = make([]byte, len(value))
        copy(valueCopy, value)
    }

    update := make([]*skipListNode, s.maxLevel)
    current := s.head

    for i := s.maxLevel - 1; i >= 0; i-- {
        for current.next[i] != nil && bytes.Compare(current.next[i].key, keyCopy) < 0 {
            current = current.next[i]
        }
        update[i] = current
    }

    if current.next[0] != nil && bytes.Equal(current.next[0].key, keyCopy) {
        current.next[0].value = valueCopy
        return nil
    }

    // ЕДИНСТВЕННОЕ отличие – здесь старый способ
    newLevel := s.randomLevel()

    newNode := &skipListNode{
        key:   keyCopy,
        value: valueCopy,
        next:  make([]*skipListNode, newLevel+1),
    }

    for i := 0; i <= newLevel; i++ {
        newNode.next[i] = update[i].next[i]
        update[i].next[i] = newNode
    }
    return nil
}

// BenchmarkPutRandomLevel – вставка с randomLevel (классический цикл)
func BenchmarkPutRandomLevel(b *testing.B) {
    const N = 17000000 // количество вставляемых элементов
    keys := make([][]byte, N)
    for i := 0; i < N; i++ {
        keys[i] = []byte(fmt.Sprintf("user:%08d", i))
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        b.StopTimer()
        sl := New(42)
        b.StartTimer()
        for _, k := range keys {
            _ = sl.putWithRandomLevel(k, k) // используем старый метод
        }
    }
}

// BenchmarkPutFastRandomLevel – вставка с fastRandomLevel (таблица)
func BenchmarkPutFastRandomLevelBits(b *testing.B) {
    const N = 17000000
    keys := make([][]byte, N)
    for i := 0; i < N; i++ {
        keys[i] = []byte(fmt.Sprintf("user:%08d", i))
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        b.StopTimer()
        sl := New(42) // fastRandomLevel использует общую таблицу, но это ок
        b.StartTimer()
        for _, k := range keys {
            _ = sl.Put(k, k) // здесь внутри fastRandomLevel
        }
    }
}
//go:build day3

package bloom

import (
	"testing"
)

// ----- Тесты на крайние случаи -----

func TestBloom_EmptyKey(t *testing.T) {
	f := New(1024, 3)
	empty := []byte{}
	if err := f.Add(empty); err != nil {
		t.Fatalf("Add empty key: %v", err)
	}
	ok, err := f.MayContain(empty)
	if err != nil {
		t.Fatalf("MayContain empty key: %v", err)
	}
	if !ok {
		t.Fatal("false negative for empty key")
	}
}

func TestBloom_DuplicateAdd(t *testing.T) {
	f := New(1024, 3)
	key := []byte("duplicate")
	for i := 0; i < 10; i++ {
		if err := f.Add(key); err != nil {
			t.Fatalf("Add #%d: %v", i, err)
		}
	}
	ok, _ := f.MayContain(key)
	if !ok {
		t.Fatal("key not found after multiple adds")
	}
}

func TestBloom_FalsePositiveRate(t *testing.T) {
	// 1000 элементов, p=0.01 → m≈9586 бит, k≈7
	f, err := NewWithOptimal(1000, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	// Добавляем 1000 уникальных ключей
	keys := make([][]byte, 1000)
	for i := 0; i < 1000; i++ {
		keys[i] = []byte{byte(i >> 8), byte(i)}
		f.Add(keys[i])
	}
	// Проверяем 1000 заведомо отсутствующих ключей
	fp := 0
	total := 1000
	for i := 0; i < total; i++ {
		nonKey := []byte{byte(255 - byte(i)), byte(i)}
		ok, _ := f.MayContain(nonKey)
		if ok {
			fp++
		}
	}
	rate := float64(fp) / float64(total)
	t.Logf("False positive rate: %.2f%% (expected ≤ 1%%)", rate*100)
	if rate > 0.05 { // допустимый запас – 5% вместо 1% из-за малого N
		t.Errorf("FP rate too high: %.2f%%", rate*100)
	}
}

// ----- Бенчмарки -----

func benchmarkBloomAdd(b *testing.B, n int) {
	f, _ := NewWithOptimal(uint64(n), 0.01)
	keys := make([][]byte, n)
	for i := 0; i < n; i++ {
		keys[i] = []byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Add(keys[i%n])
	}
}

func BenchmarkBloomAdd_100(b *testing.B)   { benchmarkBloomAdd(b, 100) }
func BenchmarkBloomAdd_1000(b *testing.B)  { benchmarkBloomAdd(b, 1000) }
func BenchmarkBloomAdd_10000(b *testing.B) { benchmarkBloomAdd(b, 10000) }

func benchmarkBloomMayContain(b *testing.B, n int) {
	f, _ := NewWithOptimal(uint64(n), 0.01)
	keys := make([][]byte, n)
	for i := 0; i < n; i++ {
		keys[i] = []byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
		f.Add(keys[i])
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.MayContain(keys[i%n])
	}
}

func BenchmarkBloomMayContain_100(b *testing.B)   { benchmarkBloomMayContain(b, 100) }
func BenchmarkBloomMayContain_1000(b *testing.B)  { benchmarkBloomMayContain(b, 1000) }
func BenchmarkBloomMayContain_10000(b *testing.B) { benchmarkBloomMayContain(b, 10000) }

func TestBloom_NoFalseNegatives(t *testing.T) {
	// Параметры маленькие намеренно: цель теста — свойство "нет false negative",
	// а не качество false positive.
	f := New(1024, 3)

	keys := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	for _, k := range keys {
		if err := f.Add(k); err != nil {
			t.Fatalf("Add(%q): %v", string(k), err)
		}
	}
	for _, k := range keys {
		ok, err := f.MayContain(k)
		if err != nil {
			t.Fatalf("MayContain(%q): %v", string(k), err)
		}
		if !ok {
			t.Fatalf("false negative for key=%q", string(k))
		}
	}
}

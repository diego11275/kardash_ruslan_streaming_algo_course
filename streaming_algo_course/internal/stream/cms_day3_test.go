//go:build day3

package stream

import (
	"fmt"
	"math/rand"
	"testing"
)

// ----- Тесты на крайние случаи -----

func TestCMS_EmptyKey(t *testing.T) {
	cms := NewCountMinSketch(64, 4, 1)
	empty := []byte{}
	if err := cms.Add(empty); err != nil {
		t.Fatalf("Add empty key: %v", err)
	}
	est, err := cms.Estimate(empty)
	if err != nil {
		t.Fatalf("Estimate empty key: %v", err)
	}
	if est < 1 {
		t.Fatal("estimation too low for empty key")
	}
}

func TestCMS_DuplicateAdd(t *testing.T) {
	cms := NewCountMinSketch(64, 4, 1)
	key := []byte("hot")
	for i := 0; i < 100; i++ {
		cms.Add(key)
	}
	est, _ := cms.Estimate(key)
	if est < 100 {
		t.Fatalf("estimate %d < true count 100", est)
	}
}

func TestCMS_ZeroParameters(t *testing.T) {
	// ширина = 0 -> заменяется на 1
	cms := NewCountMinSketch(0, 0, 1)
	if cms.width != 1 || cms.depth != 1 {
		t.Fatalf("width or depth not corrected: w=%d d=%d", cms.width, cms.depth)
	}
	key := []byte("key")
	if err := cms.Add(key); err != nil {
		t.Fatal(err)
	}
	est, _ := cms.Estimate(key)
	if est != 1 {
		t.Fatalf("expected 1, got %d", est)
	}
}

func TestCMS_Overflow(t *testing.T) {
	// Проверяем, что счётчики не паникуют при переполнении (wrap around uint64)
	cms := NewCountMinSketch(1, 1, 1)
	key := []byte("x")
	// Добавляем больше, чем 2^64 – это невозможно за разумное время,
	// но можем проверить, что после 2^64-1 добавлений следующее добавление обнулит счётчик (wrap).
	// В реальности до переполнения не дойдём, но структура должна корректно работать с uint64.
	// Просто добавим много раз и убедимся, что значение не уменьшается.
	for i := 0; i < 1_000_000; i++ {
		cms.Add(key)
	}
	est, _ := cms.Estimate(key)
	if est < 1_000_000 {
		t.Fatalf("underestimate after many adds: %d", est)
	}
}

// ----- Бенчмарки -----

func benchmarkCMSAdd(b *testing.B, width, depth uint32) {
	cms := NewCountMinSketch(width, depth, 1)
	key := []byte("benchKey")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cms.Add(key)
	}
}

func BenchmarkCMSAdd_64x4(b *testing.B)   { benchmarkCMSAdd(b, 64, 4) }
func BenchmarkCMSAdd_256x4(b *testing.B)  { benchmarkCMSAdd(b, 256, 4) }
func BenchmarkCMSAdd_1024x4(b *testing.B) { benchmarkCMSAdd(b, 1024, 4) }

func benchmarkCMSEstimate(b *testing.B, width, depth uint32) {
	cms := NewCountMinSketch(width, depth, 1)
	key := []byte("benchKey")
	for i := 0; i < 1000; i++ {
		cms.Add(key)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cms.Estimate(key)
	}
}

func BenchmarkCMSEstimate_64x4(b *testing.B)   { benchmarkCMSEstimate(b, 64, 4) }
func BenchmarkCMSEstimate_256x4(b *testing.B)  { benchmarkCMSEstimate(b, 256, 4) }
func BenchmarkCMSEstimate_1024x4(b *testing.B) { benchmarkCMSEstimate(b, 1024, 4) }

func TestCountMinSketch_EstimateMonotone(t *testing.T) {
	cms := NewCountMinSketch(64, 4, 1)

	for i := 0; i < 10; i++ {
		if err := cms.Add([]byte("hot")); err != nil {
			t.Fatalf("Add hot: %v", err)
		}
	}
	est, err := cms.Estimate([]byte("hot"))
	if err != nil {
		t.Fatalf("Estimate hot: %v", err)
	}
	// Для CMS типично: оценка >= истинного значения (overestimate допустим),
	// но undercount — индикатор ошибки.
	if est < 10 {
		t.Fatalf("estimate too small: %d", est)
	}
}


// generateZipf создаёт ключи по закону Ципфа.
func generateZipf(numItems, numKeys int, s float64) ([]string, map[string]uint64) {
	rng := rand.New(rand.NewSource(42))
	zipf := rand.NewZipf(rng, s, 1, uint64(numKeys-1))
	trueFreq := make(map[string]uint64, numKeys)
	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = fmt.Sprintf("key-%d", i)
	}
	stream := make([]string, numItems)
	for i := range stream {
		k := keys[zipf.Uint64()]
		stream[i] = k
		trueFreq[k]++
	}
	return stream, trueFreq
}

func TestConservativeImprovement(t *testing.T) {
	stream, trueFreq := generateZipf(50000, 1000, 1.5)

	width, depth, seed := uint32(200), uint32(4), uint32(12345)
	classic := NewCountMinSketch(width, depth, seed)
	conserv := NewCountMinSketch(width, depth, seed)

	for _, key := range stream {
		classic.Add([]byte(key))
		conserv.AddConservative([]byte(key))
	}

	var classicErrSum, conservErrSum, theorErrSum float64
	count := 0
	N := float64(len(stream)) // total элементов в потоке

	for key, trueCnt := range trueFreq {
		estClassic, _ := classic.Estimate([]byte(key))
		estConserv, _ := conserv.Estimate([]byte(key))
		if trueCnt == 0 {
			continue
		}
		classicErr := float64(estClassic-trueCnt) / float64(trueCnt)
		conservErr := float64(estConserv-trueCnt) / float64(trueCnt)
		classicErrSum += classicErr
		conservErrSum += conservErr

		// Теоретическая оценка снизу: (total/width) / count_true
		theorErr := (N / float64(width)) / float64(trueCnt*trueCnt)
		theorErrSum += theorErr

		count++
	}
	avgClassic := classicErrSum / float64(count)
	avgConserv := conservErrSum / float64(count)
	avgTheor := theorErrSum / float64(count)

	t.Logf("Средняя относительная ошибка (классический Add): %.4f", avgClassic)
	t.Logf("Средняя относительная ошибка (AddConservative): %.4f", avgConserv)
	t.Logf("Теоретическая оценка снизу (total/width / true_freq): %.4f", avgTheor)

	if avgConserv >= avgClassic {
		t.Errorf("Ожидалось уменьшение ошибки, но консервативный дал %.4f против %.4f",
			avgConserv, avgClassic)
	}
}

func TestFirst10Keys(t *testing.T) {
    stream, trueFreq := generateZipf(50000, 1000, 1.5)

    width, depth, seed := uint32(200), uint32(4), uint32(12345)
    classic := NewCountMinSketch(width, depth, seed)
    conserv := NewCountMinSketch(width, depth, seed)

    for _, key := range stream {
        classic.Add([]byte(key))
        conserv.AddConservative([]byte(key))
    }

    N := float64(len(stream))
    fmt.Println("Key        | True | Classic | ClRelErr | Conserv | CvRelErr | TheorRel")
    fmt.Println("-----------+------+---------+----------+---------+----------+---------")
    for i := 0; i < 10; i++ {
        key := fmt.Sprintf("key-%d", i)
        trueCnt := trueFreq[key]
        estClassic, _ := classic.Estimate([]byte(key))
        estConserv, _ := conserv.Estimate([]byte(key))

        clRelErr := float64(estClassic-trueCnt) / float64(trueCnt)
        cvRelErr := float64(estConserv-trueCnt) / float64(trueCnt)
        theorRel := (N / float64(width)) / float64(trueCnt*trueCnt)

        fmt.Printf("%-10s | %4d | %6d | %8.4f | %7d | %8.4f | %8.6f\n",
            key, trueCnt, estClassic, clRelErr, estConserv, cvRelErr, theorRel)
    }
}
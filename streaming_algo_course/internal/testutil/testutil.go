package testutil

import (
	"fmt"
	"math/rand"
)

const alphaNum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandKey возвращает псевдослучайный ключ из alphaNum.
// Нужен для воспроизводимых тестов/бенчей: студент передаёт rng с фиксированным seed.
func RandKey(rng *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphaNum[rng.Intn(len(alphaNum))]
	}
	return b
}

// KeyGenerator генерирует ключи.
type KeyGenerator interface {
	Next() []byte
}

type UniformGenerator struct {
	Rng *rand.Rand
	Len int
}

func (g *UniformGenerator) Next() []byte {
	return RandKey(g.Rng, g.Len)
}

type ZipfGenerator struct {
	Rng   *rand.Rand
	Zipf  *rand.Zipf
	Items [][]byte
}

// NewZipfGenerator создаёт генератор, который возвращает ключи из фиксированного набора items
// с распределением Zipf (s > 1, v >= 1). Чем больше s, тем больше перекос.
func NewZipfGenerator(rng *rand.Rand, s, v float64, itemsCount, keyLen int) *ZipfGenerator {
	// Генерируем "словарь" ключей
	items := make([][]byte, itemsCount)
	for i := range items {
		items[i] = []byte(fmt.Sprintf("user_%08d", i)) // user_00000123
	}

	z := rand.NewZipf(rng, s, v, uint64(itemsCount-1))
	return &ZipfGenerator{
		Rng:   rng,
		Zipf:  z,
		Items: items,
	}
}

func (z *ZipfGenerator) Next() []byte {
	idx := z.Zipf.Uint64()
	return append([]byte(nil), z.Items[idx]...) // copy
}

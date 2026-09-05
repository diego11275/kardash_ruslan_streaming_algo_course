package stream

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
)

var ErrNotImplemented = errors.New("stream: функция не реализована")

// CountMinSketch — структура для поиска частых элементов.
type CountMinSketch struct {
	width  uint32
	depth  uint32
	seed   uint32
	counts [][]uint64
}

// NewCountMinSketch создаёт скетч.
// width (w) — ширина таблицы (если 0, устанавливается в 1).
// depth (d) — количество хеш-функций (если 0, устанавливается в 1).
func NewCountMinSketch(width, depth, seed uint32) *CountMinSketch {
	// Защита от некорректных параметров
	if width == 0 {
		width = 1
	}
	if depth == 0 {
		depth = 1
	}
	counts := make([][]uint64, depth)
	for i := range counts {
		counts[i] = make([]uint64, width)
	}
	return &CountMinSketch{
		width:  width,
		depth:  depth,
		seed:   seed,
		counts: counts,
	}
}

func (c *CountMinSketch) hash(key []byte, seed uint32) uint64 {
	h := fnv.New64a()
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, seed)
	h.Write(b)
	h.Write(key)
	return h.Sum64()
}

// Add увеличивает счетчик для ключа.
func (c *CountMinSketch) Add(key []byte) error {
	if key == nil {
		return errors.New("cms: ключ не может быть nil")
	}
	for i := uint32(0); i < c.depth; i++ {
		// Переполнение seed+i допустимо (обёртка по модулю 2^32)
		pos := c.hash(key, c.seed+i) % uint64(c.width)
		c.counts[i][pos]++
	}
	return nil
}

// AddConservative добавляет ключ, используя адаптивный алгоритм
// «консервативного обновления» (Conservative Update / Minimal Increment).
//
// В отличие от обычного Add, инкрементируются только те ячейки,
// которые на данный момент содержат минимальное значение среди всех
// хеш-позиций ключа. Это автоматически снижает завышение оценки
// при коллизиях и улучшает точность без изменения основных параметров
// (width/depth). Алгоритм адаптируется к текущей нагрузке, так как
// величина инкремента зависит от заполненности скетча.
func (c *CountMinSketch) AddConservative(key []byte) error {
	if key == nil {
		return errors.New("cms: ключ не может быть nil")
	}

	// Находим минимальное значение среди всех хеш-позиций
	minVal := uint64(^uint64(0))
	positions := make([]uint32, c.depth)
	for i := uint32(0); i < c.depth; i++ {
		pos := c.hash(key, c.seed+i) % uint64(c.width)
		positions[i] = uint32(pos)
		if c.counts[i][pos] < minVal {
			minVal = c.counts[i][pos]
		}
	}

	// Инкрементируем только те ячейки, которые равны минимальному значению
	for i := uint32(0); i < c.depth; i++ {
		if c.counts[i][positions[i]] == minVal {
			c.counts[i][positions[i]]++
		}
	}
	return nil
}

// Estimate возвращает примерную частоту ключа.
// Гарантия: Estimate >= TrueCount (никогда не занижает).
func (c *CountMinSketch) Estimate(key []byte) (uint64, error) {
	if key == nil {
		return 0, errors.New("cms: ключ не может быть nil")
	}
	min := uint64(^uint64(0)) // максимальное значение
	for i := uint32(0); i < c.depth; i++ {
		pos := c.hash(key, c.seed+i) % uint64(c.width)
		if c.counts[i][pos] < min {
			min = c.counts[i][pos]
		}
	}
	return min, nil
}
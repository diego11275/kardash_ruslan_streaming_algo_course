package bloom

import (
	"errors"
	"hash/fnv"
	"math"
)

// ErrNotImplemented используется в заготовке практики третьего дня.
var ErrNotImplemented = errors.New("bloom: функция не реализована")

// Filter — вероятностный фильтр Блума.
type Filter struct {
	bits []byte		// срез байтов
	m    uint64		// размер битового массива
	k    uint8      // количество хеш функций
}

// New создаёт фильтр с заданным размером битового массива (m) и количеством хеш-функций (k).
// Если m или k равны 0, они заменяются на 1 (минимальные корректные значения).
func New(size uint64, hashes uint8) *Filter {
	if size == 0 {
		size = 1
	}
	if hashes == 0 {
		hashes = 1
	}
	// формула используется для округления вверх 
	// до целого числа байт при выделении памяти под битовый массив
	numBytes := (size + 7) / 8
	return &Filter{
		bits: make([]byte, numBytes),
		m:    size,
		k:    hashes,
	}
}

// OptimalParams вычисляет оптимальные параметры фильтра Блума:
//   - m – размер битового массива (округлённый вверх до целого)
//   - k – количество хеш-функций (целое, не менее 1)
// на основе ожидаемого количества элементов n и допустимой вероятности ложноположительного срабатывания p.
// Формулы:
//   m = ceil( -n * ln(p) / (ln(2))^2 )
//   k = round( (m / n) * ln(2) )
// Возвращает ошибку, если n == 0 или p не в интервале (0,1).
func OptimalParams(n uint64, p float64) (m uint64, k uint8, err error) {
	if n == 0 {
		return 0, 0, errors.New("bloom: количество элементов n должно быть > 0")
	}
	if p <= 0 || p >= 1 {
		return 0, 0, errors.New("bloom: вероятность p должна быть в интервале (0,1)")
	}
	ln2 := math.Ln2
	lnp := math.Log(p)
	mFloat := -float64(n) * lnp / (ln2 * ln2)
	m = uint64(math.Ceil(mFloat))
	if m == 0 {
		m = 1
	}
	kFloat := (float64(m) / float64(n)) * ln2
	k = uint8(math.Max(1, math.Round(kFloat)))
	return m, k, nil
}

// NewWithOptimal создаёт фильтр Блума с автоматическим подбором параметров
// m и k, оптимизированных под хранение n элементов с вероятностью ложного срабатывания p.
func NewWithOptimal(n uint64, p float64) (*Filter, error) {
	m, k, err := OptimalParams(n, p)
	if err != nil {
		return nil, err
	}
	return New(m, k), nil
}

func (f *Filter) setBit(pos uint64) {
	f.bits[pos/8] |= 1 << (pos % 8)
}

func (f *Filter) testBit(pos uint64) bool {
	return (f.bits[pos/8] & (1 << (pos % 8))) != 0
}

func (f *Filter) hash(key []byte) (uint64, uint64) {
	// создаём хеш-объект FNV-1a
	h1 := fnv.New64a()
	h1.Write(key)
	hash1 := h1.Sum64()

	// создаём хеш-объект FNV-1
	h2 := fnv.New64()
	h2.Write(key)
	hash2 := h2.Sum64()
	// Гарантируем, что hash2 нечётный – улучшает равномерность распределения
	if hash2 == 0 {
		hash2 = 1
	} else {
		hash2 |= 1
	}
	return hash1, hash2
}

// Add добавляет ключ в фильтр.
func (f *Filter) Add(key []byte) error {
	if key == nil {
		return errors.New("bloom: ключ не может быть nil")
	}
	h1, h2 := f.hash(key)
	for i := uint8(0); i < f.k; i++ {
		pos := (h1 + uint64(i)*h2) % f.m
		f.setBit(pos)
	}
	return nil
}

// MayContain проверяет наличие ключа.
// Возвращает false, если ключа точно нет.
// Возвращает true, если ключ возможно есть.
func (f *Filter) MayContain(key []byte) (bool, error) {
	if key == nil {
		return false, errors.New("bloom: ключ не может быть nil")
	}
	h1, h2 := f.hash(key)
	for i := uint8(0); i < f.k; i++ {
		pos := (h1 + uint64(i)*h2) % f.m
		if !f.testBit(pos) {
			return false, nil
		}
	}
	return true, nil
}
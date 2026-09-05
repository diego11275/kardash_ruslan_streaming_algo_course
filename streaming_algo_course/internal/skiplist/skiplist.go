package skiplist

import (
	"bytes"
	"errors"
	"math/rand"
	"math/bits"
)

// ErrNotFound означает отсутствие ключа (IMSI).
var ErrNotFound = errors.New("skiplist: ключ не найден")

// ErrNotImplemented используется в заготовке практики первого дня.
var ErrNotImplemented = errors.New("skiplist: функция не реализована")

// SkipList — In-Memory движок для HLR.
// Обеспечивает O(log N) на чтение/запись и упорядоченный доступ.
//
// В практической реализации вам нужно хранить:
// - ключи/значения как []byte
// - уровни (forward pointers)
// - генератор уровней с фиксируемым seed (для детерминизма тестов)
// SkipList — In-Memory движок для HLR.
type SkipList struct {
	head     *skipListNode
	maxLevel int
	p        float64
	rand     *rand.Rand
}

// skipListNode представляет узел SkipList.
type skipListNode struct {
	key   []byte
	value []byte
	next  []*skipListNode // указатели на следующие узлы на каждом уровне
}

// New создаёт SkipList. seed требуется для детерминируемых тестов (воспроизводимость поведения при ошибках).
func New(seed int64) *SkipList {
	const maxLevel = 25   // достаточно для 2^20 элементов (maxLevel = ln(N)/ln(2))
	const p = 0.5         // классическая вероятность

	src := rand.NewSource(seed) // создаём источник случайности, который генерирует псевдослучайную последовательность чисел, полностью определяемую переданным целочисленным seed.
	r := rand.New(src)			// создаём новый генератор случайных чисел, который использует переданный источник src.
	
	head := &skipListNode{
		next: make([]*skipListNode, maxLevel),
	}

	return &SkipList{
		head:     head,
		maxLevel: maxLevel,
		p:        p,
		rand:     r,
	}
}

// randomLevel генерирует случайный уровень для нового узла.
func (s *SkipList) randomLevel() int {
	level := 0
	for s.rand.Float64() < s.p && level < s.maxLevel-1 {
		level++
	}
	return level
}

func (s *SkipList) fastRandomLevelBits() int {
    r := s.rand.Uint64()
    // Количество trailing zeros – это уровень (при условии, что биты независимы)
    level := bits.TrailingZeros64(r)
    if level >= s.maxLevel {
        level = s.maxLevel - 1
    }
    return level
}

// Put обновляет профиль абонента (IMSI -> Data).
// Если ключ уже существует, значение перезаписывается.
func (s *SkipList) Put(key, value []byte) error {
	// Создаём копии, чтобы внешние изменения не влияли на данные
    keyCopy := make([]byte, len(key))
    copy(keyCopy, key)
    var valueCopy []byte
    if value != nil {
        valueCopy = make([]byte, len(value))
        copy(valueCopy, value)
    }

	update := make([]*skipListNode, s.maxLevel) // предшественники на каждом уровне
	current := s.head

	// Идём от верхнего уровня вниз, запоминая узлы, после которых нужно вставить
	for i := s.maxLevel - 1; i >= 0; i-- {
		for current.next[i] != nil && bytes.Compare(current.next[i].key, keyCopy) < 0 {
			current = current.next[i]
		}
		update[i] = current
	}

	// Проверяем, существует ли уже такой ключ
	if current.next[0] != nil && bytes.Equal(current.next[0].key, keyCopy) {
		// Обновляем значение
		current.next[0].value = valueCopy
		return nil
	}

	// Генерируем уровень для нового узла
	// newLevel := s.randomLevel()
	newLevel := s.fastRandomLevelBits()

	// Создаём новый узел
	newNode := &skipListNode{
		key:   keyCopy,
		value: valueCopy,
		next:  make([]*skipListNode, newLevel+1),
	}

	// Вставляем узел на всех уровнях от 0 до newLevel
	for i := 0; i <= newLevel; i++ {
		newNode.next[i] = update[i].next[i]
		update[i].next[i] = newNode
	}

	return nil
}

// Get возвращает профиль абонента по ключу.
func (s *SkipList) Get(key []byte) ([]byte, error) {
	current := s.head

	// Идём от верхнего уровня вниз
	for i := s.maxLevel - 1; i >= 0; i-- {
		for current.next[i] != nil && bytes.Compare(current.next[i].key, key) < 0 {
			current = current.next[i]
		}
	}

	// Проверяем следующий узел на уровне 0
	if current.next[0] != nil && bytes.Equal(current.next[0].key, key) {
		if current.next[0].value != nil {
			valueCopy := make([]byte, len(current.next[0].value))
			copy(valueCopy, current.next[0].value)
			return valueCopy, nil
		}
		return nil,nil
	}
	return nil, ErrNotFound
}

// Delete удаляет абонента по ключу.
func (s *SkipList) Delete(key []byte) error {
	update := make([]*skipListNode, s.maxLevel)
	current := s.head

	// Ищем предшественников
	for i := s.maxLevel - 1; i >= 0; i-- {
		for current.next[i] != nil && bytes.Compare(current.next[i].key, key) < 0 {
			current = current.next[i]
		}
		update[i] = current
	}

	// Проверяем, существует ли узел
	target := current.next[0]
	if target == nil || !bytes.Equal(target.key, key) {
		return ErrNotFound
	}

	// Удаляем узел на всех уровнях, где он присутствует
	for i := 0; i < len(target.next); i++ {
		update[i].next[i] = target.next[i]
	}
	return nil
}


// Iterator — упорядоченная итерация по диапазону ключей (Range Scan).
// В HLR используется для выгрузки абонентов по префиксу IMSI.
type Iterator interface {
	Next() (key, value []byte, ok bool, err error)
	Close() error
}

// scanIterator — ленивый итератор для обхода диапазона.
type scanIterator struct {
	current *skipListNode
	end     []byte
	closed  bool
}

// Next возвращает следующий ключ и значение в диапазоне.
func (it *scanIterator) Next() (key, value []byte, ok bool, err error) {
	if it.closed {
		return nil, nil, false, nil
	}
	if it.current == nil {
		return nil, nil, false, nil
	}
	// Проверяем, не вышли ли за end
	if it.end != nil && bytes.Compare(it.current.key, it.end) >= 0 {
		it.Close()
		return nil, nil, false, nil
	}
	key = append([]byte(nil), it.current.key...)
	value = append([]byte(nil), it.current.value...)
	it.current = it.current.next[0]
	return key, value, true, nil
}

// Close освобождает ресурсы итератора.
func (it *scanIterator) Close() error {
	it.closed = true
	it.current = nil
	return nil
}

// Scan возвращает итератор по диапазону [start, end).
// Если start == nil, считается -∞ (начало списка).
// Если end == nil, считается +∞ (конец списка).
func (s *SkipList) Scan(start, end []byte) (Iterator, error) {
	// Находим первый узел, который >= start
	current := s.head
	for i := s.maxLevel - 1; i >= 0; i-- {
		for current.next[i] != nil {
			if start != nil && bytes.Compare(current.next[i].key, start) < 0 {
				current = current.next[i]
			} else {
				break
			}
		}
	}
	// После спуска current указывает на узел перед первым >= start
	var startNode *skipListNode
	if current.next[0] != nil {
		if start == nil || bytes.Compare(current.next[0].key, start) >= 0 {
			startNode = current.next[0]
		}
	}
	return &scanIterator{
		current: startNode,
		end:     end, // end может быть nil (означает до конца)
	}, nil
}

// lsm.go
package lsm // Пакет lsm реализует LSM-дерево (Log-Structured Merge-tree)

import (
	"bytes" 						// Сравнение байтовых срезов
	"errors" 						// Создание ошибок
	"fmt" 							// Форматированный вывод
	"io" 							// Интерфейсы ввода-вывода
	"os" 							// Взаимодействие с операционной системой
	"path/filepath" 				// Работа с путями файлов
	"sort" 							// Сортировка строк
	"sync" 							// Примитивы синхронизации

	"kvschool/internal/bloom"
	"kvschool/internal/skiplist" 
	"kvschool/internal/sstable" 
	"kvschool/internal/wal" 
)

var ErrNotFound = errors.New("lsm: key not found")

type Options struct { 						// Конфигурация движка
	Dir                    string 			// Путь к директории хранения
	MemtableFlushThreshold int 				// Порог размера memtable для сброса на диск (в байтах)
	SyncWrites             bool 			// Синхронизировать WAL после каждой записи?
	L0CompactThreshold     int 				// Количество L0‑файлов для запуска компакшена (по умолчанию 4)
	BloomFalsePositiveRate float64 			// Целевая вероятность ложного срабатывания bloom-фильтра
}

type Engine struct { 						// Основная структура LSM-движка
	opts        Options 					// Конфигурация
	mu          sync.Mutex 					// Мьютекс для потокобезопасного доступа
	mem         *skiplist.SkipList 			// Активная memtable в памяти
	memSize     int 						// Приблизительный размер memtable (сумма длин ключей и значений)
	walWriter   *wal.Writer 				// Писатель WAL
	walFile     *os.File 					// Файл WAL
	sstables    []*sstable.Reader 			// Читатели L0 SSTable (от новых к старым)
	sstFiles    []*os.File 					// Открытые файлы L0
	sstBloom    []*bloom.Filter 			// Bloom-фильтры для L0 (могут быть nil)
	l1          *sstable.Reader 			// Читатель L1 (nil если нет)
	l1File      *os.File 					// Файл L1
	l1Bloom     *bloom.Filter 				// Bloom-фильтр для L1 (может быть nil)
	compactTrigger chan struct{} 			// Канал сигнала компакшена (буфер 1)
	compactDone    chan struct{} 			// Канал для остановки воркера компакшена
	closeWG        sync.WaitGroup 			// Ожидание завершения фонового воркера
	closed         bool 					// Флаг, что движок закрыт
}

// Open открывает или создаёт LSM-хранилище в указанной директории.
func Open(opts Options) (*Engine, error) {
	if err := os.MkdirAll(opts.Dir, 0755); err != nil { return nil, fmt.Errorf("lsm: create dir: %w", err) }
	if opts.L0CompactThreshold <= 0 { opts.L0CompactThreshold = 4 }
	if opts.BloomFalsePositiveRate <= 0 || opts.BloomFalsePositiveRate >= 1 { opts.BloomFalsePositiveRate = 0.01 } // По умолчанию 1%

	e := &Engine{ 								// Инициализация движка
		opts:           opts,
		mem:            skiplist.New(0), 		// Пустая memtable
		compactTrigger: make(chan struct{}, 1), // Канал с буфером 1
		compactDone:    make(chan struct{}), 	// Канал завершения воркера
	}

	sstFiles, err := filepath.Glob(filepath.Join(opts.Dir, "*.sst")) // Загружаем существующие SSTable
	if err != nil { return nil, err }
	sort.Strings(sstFiles) 	// Отсортировать имена по алфавиту (от старых к новым)

	l1Path := filepath.Join(opts.Dir, "level1.sst") 	// Путь к L1
	if _, err := os.Stat(l1Path); err == nil { 			// Если L1 существует
		f, err := os.Open(l1Path) 
		if err != nil { return nil, fmt.Errorf("lsm: open L1: %w", err) }
		fi, err := f.Stat() 							// Размер файла
		if err != nil { f.Close(); return nil, err }
		r, err := sstable.NewReader(f, fi.Size()) 		// Создать читатель
		if err != nil { f.Close(); return nil, fmt.Errorf("lsm: read L1: %w", err) }
		e.l1 = r 										// Сохранить читатель L1
		e.l1File = f 									// Сохранить дескриптор
		e.l1Bloom = e.buildFilter(r)
	}

	for i := len(sstFiles) - 1; i >= 0; i-- {
		fname := sstFiles[i]
		if filepath.Base(fname) == "level1.sst" { continue }
		rd, err := os.Open(fname)
		if err != nil { return nil, fmt.Errorf("lsm: open sst %s: %w", fname, err) }
		fi, err := rd.Stat() // Размер
		if err != nil { rd.Close(); return nil, err }
		r, err := sstable.NewReader(rd, fi.Size()) 		// Читатель
		if err != nil { rd.Close(); return nil, fmt.Errorf("lsm: read sst %s: %w", fname, err) }
		e.sstables = append(e.sstables, r) 				// Добавить читатель
		e.sstFiles = append(e.sstFiles, rd) 			// Добавить дескриптор
		e.sstBloom = append(e.sstBloom, e.buildFilter(r))
	}

	walPath := filepath.Join(opts.Dir, "wal.log")
	walFile, err := os.OpenFile(walPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644) // Открыть/создать WAL
	if err != nil { return nil, fmt.Errorf("lsm: open WAL: %w", err) }
	// Восстановить memtable из WAL
	if err := e.recoverFromWAL(walFile); err != nil { walFile.Close(); return nil, fmt.Errorf("lsm: recover WAL: %w", err) }
	if _, err := walFile.Seek(0, io.SeekEnd); err != nil { walFile.Close(); return nil, err } // Установить позицию в конец
	e.walFile = walFile 							// Сохранить файл
	e.walWriter = wal.NewWriter(walFile)		 	// Создать писатель WAL

	e.closeWG.Add(1) 								// Увеличить счётчик WaitGroup
	go e.compactionWorker() 						// Запустить фоновый компакшен
	return e, nil
}

// recoverFromWAL восстанавливает memtable из записей WAL.
func (e *Engine) recoverFromWAL(f *os.File) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil { return err } // В начало файла
	reader := wal.NewReader(f) // Создать читатель WAL
	for {
		rec, ok, err := reader.Next() // Следующая запись
		if err != nil { return err }
		if !ok { break }
		switch rec.Type {
		case wal.OpPut:
			if err := e.mem.Put(rec.Key, rec.Value); err != nil { return err }
			e.memSize += len(rec.Key) + len(rec.Value)
		case wal.OpDelete:
			if err := e.mem.Put(rec.Key, nil); err != nil { return err } 
			e.memSize += len(rec.Key)
		}
	}
	return nil
}

// flushMemtable сбрасывает memtable в новый SSTable уровня 0 и обрезает WAL.
func (e *Engine) flushMemtable() error {
	nextNum := len(e.sstables) 
	sstName := filepath.Join(e.opts.Dir, fmt.Sprintf("%06d.sst", nextNum))
	f, err := os.Create(sstName)
	if err != nil { return err } 
	writer := sstable.NewWriter(f) 

	iter, err := e.mem.Scan(nil, nil) 		// Итератор по всей memtable
	if err != nil { f.Close(); return err } 
	defer iter.Close()

	var keys [][]byte // Список ключей для bloom-фильтра
	for {
		k, v, ok, err := iter.Next()
		if err != nil { return err }
		if !ok { break }
		keys = append(keys, k)
		if v == nil { if err := writer.Delete(k); err != nil { return err } } else { if err := writer.Put(k, v); err != nil { return err } } // Записать tombstone или значение
	}

	size, err := writer.Finish() // Завершить запись, получить размер данных
	if err != nil { f.Close(); return err }
	if err := f.Close(); err != nil { return err }

	// Обрезка WAL
	if e.walWriter != nil { if err := e.walWriter.Close(); err != nil { return err } } 	// Закрыть писатель
	if e.opts.SyncWrites { if err := e.walFile.Sync(); err != nil { return err } } 		// Синхронизировать WAL
	if err := e.walFile.Truncate(0); err != nil { return err } 							// Обрезать WAL до нуля
	if _, err := e.walFile.Seek(0, io.SeekStart); err != nil { return err } 			// В начало файла
	e.walWriter = wal.NewWriter(e.walFile) 												// Новый писатель

	rd, err := os.Open(sstName) 			// Открыть новый SSTable для чтения
	if err != nil { return err }
	r, err := sstable.NewReader(rd, size) 	// Создать читатель
	if err != nil { rd.Close(); return err }

	var bf *bloom.Filter
	if len(keys) > 0 {
		n := uint64(len(keys))
		m, k, err := bloom.OptimalParams(n, e.opts.BloomFalsePositiveRate)
		if err == nil {
			bf = bloom.New(m, k)
			for _, key := range keys { _ = bf.Add(key) }
		}
	}

	e.sstables, e.sstFiles, e.sstBloom = // Вставить новый SSTable в начало списков
		append([]*sstable.Reader{r}, e.sstables...),
		append([]*os.File{rd}, e.sstFiles...),
		append([]*bloom.Filter{bf}, e.sstBloom...)

	e.mem = skiplist.New(0)
	e.memSize = 0
	return nil
}

// compact сливает все L0 и L1 в новый L1, удаляет старые файлы.
func (e *Engine) compact() error {
	if len(e.sstables) == 0 && e.l1 == nil { return nil }

	var iterators []kvIterator 
	for _, r := range e.sstables {
		it, err := r.Iterator(nil, nil)
		if err != nil { return err }
		iterators = append(iterators, &sstableIterAdapter{it})
	}
	if e.l1 != nil { 
		it, err := e.l1.Iterator(nil, nil)
		if err != nil { return err }
		iterators = append(iterators, &sstableIterAdapter{it})
	}

	mergeIt := newMergeIterator(iterators)
	defer mergeIt.Close()

	tmpPath := filepath.Join(e.opts.Dir, "level1.sst.tmp") 
	f, err := os.Create(tmpPath) 			// создание временного файла
	if err != nil { return err }
	writer := sstable.NewWriter(f)

	var liveKeys [][]byte 					// Ключи для bloom-фильтра
	lastKey := []byte{} 					// Предыдущий ключ для исключения дубликатов

	for {
		k, v, ok, err := mergeIt.Next()
		if err != nil { f.Close(); os.Remove(tmpPath); return err }
		if !ok { break }
		if bytes.Equal(k, lastKey) { continue } // Пропустить дубликат
		lastKey = append(lastKey[:0], k...) // Запомнить новый последний ключ
		if err := writer.Put(k, v); err != nil { f.Close(); os.Remove(tmpPath); return err }
		liveKeys = append(liveKeys, k)
	}

	size, err := writer.Finish() // Завершить запись
	if err != nil { f.Close(); os.Remove(tmpPath); return err }
	if err := f.Close(); err != nil { os.Remove(tmpPath); return err }

	l1Path := filepath.Join(e.opts.Dir, "level1.sst") // Целевой путь L1
	if err := os.Rename(tmpPath, l1Path); err != nil { os.Remove(tmpPath); return err } // Атомарно заменить старый L1 новым

	for _, f := range e.sstFiles { f.Close(); os.Remove(f.Name()) } // Закрыть и удалить все L0-файлы
	e.sstables = nil // Очистить читатели L0
	e.sstFiles = nil // Очистить дескрипторы L0
	e.sstBloom = nil // Очистить bloom-фильтры L0

	if e.l1File != nil { e.l1File.Close(); e.l1File = nil } // Закрыть старый L1 дескриптор
	e.l1 = nil 			// Сбросить читатель L1
	e.l1Bloom = nil 	// Сбросить bloom-фильтр L1

	rd, err := os.Open(l1Path) 
	if err != nil { return err }
	r, err := sstable.NewReader(rd, size) 	// Создать читатель
	if err != nil { rd.Close(); return err }
	e.l1 = r 								// Сохранить читатель
	e.l1File = rd 							// Сохранить дескриптор

	if len(liveKeys) > 0 { 
		n := uint64(len(liveKeys))
		m, k, err := bloom.OptimalParams(n, e.opts.BloomFalsePositiveRate) 
		if err == nil { 
			bf := bloom.New(m, k) 
			for _, key := range liveKeys { _ = bf.Add(key) } 
			e.l1Bloom = bf
		}
	}
	return nil
}

// maybeCompact отправляет сигнал фоновому воркеру, если порог превышен.
func (e *Engine) maybeCompact() {
	if len(e.sstables) >= e.opts.L0CompactThreshold { // Если L0-файлов достаточно
		select {
		case e.compactTrigger <- struct{}{}: // Послать сигнал (неблокирующе)
		default: // Канал заполнен – сигнал уже ждёт
		}
	}
}

// compactionWorker – фоновый процесс, выполняющий компакшен.
func (e *Engine) compactionWorker() {
	defer e.closeWG.Done() // Уменьшить счётчик WaitGroup при выходе
	for {
		select {
		case <-e.compactTrigger: // Пришёл сигнал
			e.mu.Lock()
			if !e.closed && len(e.sstables) >= e.opts.L0CompactThreshold { _ = e.compact() } // Выполнить компакшен, если нужно
			e.mu.Unlock()
		case <-e.compactDone: // Сигнал завершения
			return // Выйти из горутины
		}
	}
}

// applyWrite – общая логика записи (Put или Delete) в WAL и memtable.
func (e *Engine) applyWrite(op wal.OpType, key, value []byte) error {
	if key == nil { return fmt.Errorf("lsm: key cannot be nil") }
	rec := wal.Record{Type: op, Key: key, Value: value} 								// Создать запись WAL
	if err := e.walWriter.Append(rec); err != nil { return err } 						// Добавить в WAL
	if e.opts.SyncWrites { if err := e.walWriter.Flush(); err != nil { return err } } 	// Синхронизировать WAL

	if err := e.mem.Put(key, value); err != nil { return err } 							// Записать в memtable
	if op == wal.OpPut { e.memSize += len(key) + len(value) } else { e.memSize += len(key) } // Увеличить учётный размер

	if e.memSize >= e.opts.MemtableFlushThreshold { 								
		if err := e.flushMemtable(); err != nil { return fmt.Errorf("lsm: flush: %w", err) } // Сбросить на диск
		e.maybeCompact() // Сигнал к возможному компакшену
	}
	return nil
}

// Put вставляет или обновляет значение по ключу.
func (e *Engine) Put(key, value []byte) error {
	e.mu.Lock(); defer e.mu.Unlock()
	return e.applyWrite(wal.OpPut, key, value) // Выполнить вставку
}

// Delete помечает ключ как удалённый (tombstone).
func (e *Engine) Delete(key []byte) error {
	e.mu.Lock(); defer e.mu.Unlock() 
	return e.applyWrite(wal.OpDelete, key, nil) // Выполнить удаление
}

// Get ищет ключ, начиная с memtable, затем L0, затем L1.
func (e *Engine) Get(key []byte) ([]byte, error) {
	e.mu.Lock(); defer e.mu.Unlock() // Захватить мьютекс

	if val, err := e.mem.Get(key); err == nil { 
		if val == nil { return nil, ErrNotFound } 
		return val, nil
	}
	for i, r := range e.sstables { // Поиск по таблицам L0 (от новых к старым)
		if bf := e.sstBloom[i]; bf != nil { // Если bloom-фильтр есть
			if maybe, err := bf.MayContain(key); err != nil { return nil, err } else if !maybe { continue }
		}
		v, found, rt, err := r.Lookup(key) // Поиск в SSTable
		if err != nil { return nil, err } 
		if found {
			if rt == sstable.RecordTypeDelete { return nil, ErrNotFound } 
			return v, nil
		}
	}
	if e.l1 != nil { // Поиск в L1
		if bf := e.l1Bloom; bf != nil {
			if maybe, err := bf.MayContain(key); err != nil { return nil, err } else if !maybe { return nil, ErrNotFound } // Точно нет
		}
		v, found, rt, err := e.l1.Lookup(key) 
		if err != nil { return nil, err }
		if found { 
			if rt == sstable.RecordTypeDelete { return nil, ErrNotFound }
			return v, nil 
		}
	}
	return nil, ErrNotFound // Не найден
}

// ForceFlush принудительно сбрасывает memtable на диск, если она не пуста.
func (e *Engine) ForceFlush() error {
	e.mu.Lock(); defer e.mu.Unlock() // Захватить мьютекс
	if e.memSize > 0 { return e.flushMemtable() } // Сбросить, если есть данные
	return nil
}

// buildFilter строит bloom-фильтр по всем ключам SSTable (nil, если таблица пуста)
func (e *Engine) buildFilter(r *sstable.Reader) *bloom.Filter {
    it, _ := r.Iterator(nil, nil)                                    
    defer it.Close()                                                 
    var keys [][]byte                                                 
    for k, _, ok, _ := it.Next(); ok; k, _, ok, _ = it.Next() { keys = append(keys, k) }
    if len(keys) == 0 { return nil }                                 // Пустая таблица — фильтр не нужен
    m, k, err := bloom.OptimalParams(uint64(len(keys)), e.opts.BloomFalsePositiveRate)
    if err != nil { return nil }                                     
    bf := bloom.New(m, k)                                            
    for _, key := range keys { bf.Add(key) }                         
    return bf                                                        
}

// Iterator – публичный итератор для Scan.
type Iterator struct {
	mi *mergeIterator // Внутренний сливающий итератор
}
func (it *Iterator) Next() ([]byte, []byte, bool, error) { return it.mi.Next() }
func (it *Iterator) Close() error { return it.mi.Close() }

// Scan возвращает итератор по диапазону [start, end).
func (e *Engine) Scan(start, end []byte) (*Iterator, error) {
	e.mu.Lock(); defer e.mu.Unlock()
	iterators := make([]kvIterator, 0, len(e.sstables)+2) // Слайс для итераторов (memtable + L0 + L1)
	memIter, err := e.mem.Scan(start, end) 
	if err != nil { return nil, err }
	iterators = append(iterators, &memtableIterAdapter{memIter}) 
	for _, r := range e.sstables { // Все L0
		sstIter, err := r.Iterator(start, end)
		if err != nil { return nil, err } 
		iterators = append(iterators, &sstableIterAdapter{sstIter}) 
	}
	if e.l1 != nil { // Если есть L1
		l1Iter, err := e.l1.Iterator(start, end)
		if err != nil { return nil, err }
		iterators = append(iterators, &sstableIterAdapter{l1Iter})
	}
	mergeIt := newMergeIterator(iterators) // Создать сливающий итератор
	return &Iterator{mi: mergeIt}, nil // Вернуть публичный итератор
}

// Close корректно останавливает движок: завершает компакшен, закрывает WAL и файлы.
func (e *Engine) Close() error {
	e.mu.Lock()
	e.closed = true // Установить флаг закрытия
	e.mu.Unlock()
	close(e.compactDone) 	// Отправить сигнал завершения воркеру
	e.closeWG.Wait() 		// Дождаться завершения воркера
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.walWriter != nil { e.walWriter.Close() } 
	if e.walFile != nil { e.walFile.Close() } 
	for _, f := range e.sstFiles { f.Close() } 
	if e.l1File != nil { e.l1File.Close() } 
	return nil
}

// kvIterator – внутренний интерфейс итератора по ключам-значениям.
type kvIterator interface {
	Next() ([]byte, []byte, bool, error) // Следующая запись
	Close() error // Освободить ресурсы
}

// memtableIterAdapter адаптирует skiplist.Iterator к kvIterator.
type memtableIterAdapter struct {
	iter skiplist.Iterator
}
func (a *memtableIterAdapter) Next() ([]byte, []byte, bool, error) { return a.iter.Next() } // Делегировать
func (a *memtableIterAdapter) Close() error { return a.iter.Close() } // Закрыть

// sstableIterAdapter адаптирует *sstable.Iter к kvIterator.
type sstableIterAdapter struct {
	iter *sstable.Iter
}
func (a *sstableIterAdapter) Next() ([]byte, []byte, bool, error) { return a.iter.Next() } // Делегировать
func (a *sstableIterAdapter) Close() error { return a.iter.Close() } // Закрыть

// mergeIterator сливает несколько kvIterator в один с сортировкой и удалением дубликатов.
type mergeIterator struct {
	iters   []kvIterator 			// Исходные итераторы
	current []*iterEntry 			// Текущий элемент от каждого (nil если исчерпан)
	err     error 					// Сохранённая ошибка
	lastKey []byte 					// Последний возвращённый ключ (для фильтра дубликатов)
}

// iterEntry – запись с ключом, значением и индексом итератора.
type iterEntry struct {
	key   []byte
	value []byte
	idx   int
}

// newMergeIterator создаёт сливающий итератор и инициализирует первые элементы.
func newMergeIterator(iters []kvIterator) *mergeIterator {
	m := &mergeIterator{
		iters:   iters, 
		current: make([]*iterEntry, len(iters)),
	}
	for i, it := range iters { // Загрузить первый элемент из каждого итератора
		k, v, ok, err := it.Next()
		if err != nil { m.err = err; return m }
		if ok { m.current[i] = &iterEntry{key: k, value: v, idx: i} } // Сохранить
	}
	return m
}

// Next возвращает следующий живой ключ в порядке возрастания, пропуская tombstone и дубликаты.
func (m *mergeIterator) Next() ([]byte, []byte, bool, error) {
	if m.err != nil { return nil, nil, false, m.err }
	for {
		var minEntry *iterEntry
		minIdx := -1
		for i, e := range m.current { // Выбрать минимальный ключ
			if e == nil { continue } // Итератор пуст
			if minEntry == nil || bytes.Compare(e.key, minEntry.key) < 0 { minEntry = e; minIdx = i } // Новый минимум
		}
		if minEntry == nil { return nil, nil, false, nil } // Все итераторы исчерпаны

		k, v, ok, err := m.iters[minIdx].Next() // Продвинуть выбранный итератор
		if err != nil { m.err = err; return nil, nil, false, err }
		if ok { m.current[minIdx] = &iterEntry{key: k, value: v, idx: minIdx} } else { m.current[minIdx] = nil } // Обновить или пометить исчерпанным

		if minEntry.value == nil { m.lastKey = append([]byte(nil), minEntry.key...); continue } // Tombstone – запомнить ключ и пропустить
		if m.lastKey != nil && bytes.Equal(minEntry.key, m.lastKey) { continue } // Дубликат (более старая версия)
		m.lastKey = append([]byte(nil), minEntry.key...) // Запомнить новый последний ключ
		return minEntry.key, minEntry.value, true, nil
	}
}

// Close закрывает все вложенные итераторы.
func (m *mergeIterator) Close() error {
	for _, it := range m.iters { it.Close() } // Закрыть каждый
	return nil
}
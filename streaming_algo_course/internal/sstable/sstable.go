package sstable 				// Пакет sstable реализует формат Sorted String Table

import (
	"bufio" 					// Буферизованный ввод-вывод
	"bytes" 					// Сравнение байтовых срезов
	"encoding/binary" 			// Кодирование чисел в бинарном виде
	"errors" 					// Создание ошибок
	"fmt" 						// Форматированный вывод
	"io" 						// Интерфейсы ввода-вывода
)

const ( 
	magic      = 0x53535442 	// Магическое число (ASCII "SSTB")
	indexStep  = 64 			// Шаг индексации (запись в индекс каждые 64 ключа)
	footerSize = 12 			// Размер футера: indexOffset (8 байт) + magic (4 байта)

	RecordTypePut    byte = 1
	RecordTypeDelete byte = 2
)

type Writer struct {
	buf         *bufio.Writer 	// Буферизованный writer для накопления данных
	offset      int64 			// Текущее смещение в файле (в байтах)
	keysWritten int 			// Счётчик записанных ключей
	index       []indexEntry 	// Индексные записи (ключ -> смещение в файле)
	lastKey     []byte 			// Последний записанный ключ для проверки сортировки
}

type indexEntry struct {
	key    []byte 
	offset int64 				// Смещение в файле, где начинается этот сегмент
}

// Создаем писатель с буферизацией
func NewWriter(w io.Writer) *Writer { return &Writer{buf: bufio.NewWriter(w)} }
func (w *Writer) Put(key, value []byte) error { return w.add(RecordTypePut, key, value) }
func (w *Writer) Delete(key []byte) error { return w.add(RecordTypeDelete, key, nil) }

// add – внутренний метод добавления одной записи.
func (w *Writer) add(rt byte, key, value []byte) error {
	if w.lastKey != nil && bytes.Compare(key, w.lastKey) <= 0 { 									// Проверить строгое возрастание ключей
		return fmt.Errorf("sstable: keys must be strictly increasing: %q <= %q", key, w.lastKey) 	// Ошибка при нарушении порядка
	}
	w.lastKey = append([]byte(nil), key...) // Сохранить копию текущего ключа как последний
	offset := w.offset // Запомнить смещение для возможной индексации
	if err := w.buf.WriteByte(rt); err != nil { return err } // Записать тип записи (1 байт)
	w.offset++ 
	if err := w.writeBytes(key); err != nil { return err }
	if rt == RecordTypePut { if err := w.writeBytes(value); err != nil { return err } }
	if w.keysWritten%indexStep == 0 {
		w.index = append(w.index, indexEntry{key: append([]byte(nil), key...), offset: offset})
	}
	w.keysWritten++
	return nil
}

// writeBytes записывает байтовый срез в виде длина (4 байта LE) + данные.
func (w *Writer) writeBytes(data []byte) error {
	if err := binary.Write(w.buf, binary.LittleEndian, uint32(len(data))); err != nil { return err } // Записать длину
	w.offset += 4 // Учесть 4 байта длины
	_, err := w.buf.Write(data);
	w.offset += int64(len(data));
	return err
}

// Finish завершает запись: дописывает индекс, футер и сбрасывает буфер.
func (w *Writer) Finish() (int64, error) {
	idxOff := w.offset
	if err := binary.Write(w.buf, binary.LittleEndian, uint32(len(w.index))); err != nil { return 0, err } // Количество индексных записей
	w.offset += 4
	for _, e := range w.index { // Для каждой записи индекса
		if err := w.writeBytes(e.key); err != nil { return 0, err } // Ключ индексной записи
		if err := binary.Write(w.buf, binary.LittleEndian, e.offset); err != nil { return 0, err } // Смещение
		w.offset += 8 // 8 байт на смещение
	}
	if err := binary.Write(w.buf, binary.LittleEndian, uint64(idxOff)); err != nil { return 0, err } // Смещение начала индекса (футер)
	w.offset += 8
	if err := binary.Write(w.buf, binary.LittleEndian, uint32(magic)); err != nil { return 0, err } 
	w.offset += 4
	if err := w.buf.Flush(); err != nil { return 0, err }
	return w.offset, nil // Вернуть общий размер файла
}

func (w *Writer) Close() error { return w.buf.Flush() } // Close сбрасывает буфер

type Reader struct {
	ra          io.ReaderAt 				// Источник данных с произвольным доступом
	size        int64 						// Размер файла
	index       []indexEntry 				// Загруженный индекс
	indexOffset int64 						// Байтовое смещение начала индексного блока
}

// NewReader создаем Reader, читая футер и индекс.
func NewReader(ra io.ReaderAt, size int64) (*Reader, error) {
	r := &Reader{ra: ra, size: size}
	if err := r.readFooter(); err != nil { return nil, fmt.Errorf("sstable: footer error: %w", err) } 
	return r, nil
}

// readFooter читает последние footerSize байт, проверяет magic и загружает индекс.
func (r *Reader) readFooter() error {
	if r.size < footerSize { return errors.New("file too small") }
	buf := make([]byte, footerSize)
	if _, err := r.ra.ReadAt(buf, r.size-footerSize); err != nil { return err } 		// Прочитать футер с конца файла
	if m := binary.LittleEndian.Uint32(buf[8:]); m != magic { return fmt.Errorf("bad magic: %x", m) }
	r.indexOffset = int64(binary.LittleEndian.Uint64(buf[0:8])) 						// Получить смещение индекса
	return r.readIndex(r.indexOffset) 													// Загрузить индекс
}

// readIndex загружает индексный блок с заданного смещения.
func (r *Reader) readIndex(offset int64) error {
	sec := io.NewSectionReader(r.ra, offset, r.size-offset-footerSize) 					// Окно данных индекса
	br := bufio.NewReader(sec) 															// Буферизованный читатель
	var num uint32
	if err := binary.Read(br, binary.LittleEndian, &num); err != nil { return err } 	// Количество записей индекса
	r.index = make([]indexEntry, 0, num)
	for i := uint32(0); i < num; i++ {
		var key []byte
		if err := readBytes(br, &key); err != nil { return err } 						// Прочитать ключ индекса
		var off uint64
		if err := binary.Read(br, binary.LittleEndian, &off); err != nil { return err } // Прочитать смещение
		r.index = append(r.index, indexEntry{key: key, offset: int64(off)})
	}
	return nil
}

// Lookup ищет ключ. Возвращает значение (nil если Delete), found, тип записи.
func (r *Reader) Lookup(key []byte) ([]byte, bool, byte, error) {
	startOff := int64(0)
	for i := len(r.index) - 1; i >= 0; i-- {
		if bytes.Compare(r.index[i].key, key) <= 0 { startOff = r.index[i].offset; break } // Нашли, с какого смещения начинать
	}
	sec := io.NewSectionReader(r.ra, startOff, r.indexOffset-startOff) 						// Блок данных от startOff до индекса
	br := bufio.NewReader(sec) 																// Буферизованный читатель
	return r.scanTo(br, key) 																// Линейный поиск ключа
}

// scanTo ищет ключ target линейным сканированием записей.
func (r *Reader) scanTo(br *bufio.Reader, target []byte) ([]byte, bool, byte, error) {
	for {
		rt, err := br.ReadByte() 															// Тип записи
		if err == io.EOF { return nil, false, 0, nil } 										// Конец данных, ключ не найден
		if err != nil { return nil, false, 0, err }
		var k []byte
		if err := readBytes(br, &k); err != nil { return nil, false, 0, err }
		switch cmp := bytes.Compare(k, target); {
		case cmp > 0: return nil, false, 0, nil 											// Ключ больше, дальше искать нет смысла
		case cmp == 0:
			if rt == RecordTypePut {
				var v []byte
				if err := readBytes(br, &v); err != nil { return nil, false, 0, err }
				return v, true, RecordTypePut, nil
			}
			return nil, true, RecordTypeDelete, nil 										// Tombstone
		default: 																			// k < target, пропускаем значение
			if rt == RecordTypePut { if err := skipBytes(br); err != nil { return nil, false, 0, err } }
		}
	}
}

// skipBytes пропускает length-префиксную строку.
func skipBytes(r io.Reader) error {
	var l uint32
	if err := binary.Read(r, binary.LittleEndian, &l); err != nil { return err } 	// Читаем ровно 4 байта из r (битовую длину value)
	_, err := io.CopyN(io.Discard, r, int64(l)) 									// Пропустить l байт
	return err
}

type Iter struct {
	r        *Reader 				// Ссылка на Reader
	start    []byte 				// Нижняя граница (nil – без ограничения)
	end      []byte 				// Верхняя граница (nil – без ограничения)
	buf      *bufio.Reader 			// Буферизованный читатель поверх секции данных
	finished bool 					// Флаг завершения итерации (итератор уже отдал все подходящие записи и больше не может вернуть ни одной новой пары ключ‑значение)
	closed   bool 					// Флаг закрытия вызовом Close()
}

// Iterator создаёт итератор по диапазону [start, end).
func (r *Reader) Iterator(start, end []byte) (*Iter, error) {
	startOff := int64(0) 												// Начальное смещение
	if len(start) > 0 { 												// Если start задан, найти ближайшую индексную запись
		for i := len(r.index) - 1; i >= 0; i-- {
			if bytes.Compare(r.index[i].key, start) <= 0 { startOff = r.index[i].offset; break }
		}
	}
	sec := io.NewSectionReader(r.ra, startOff, r.indexOffset-startOff) // читаем (r.indexOffset-startOff) байт начиная с startOff
	return &Iter{r: r, start: start, end: end, buf: bufio.NewReader(sec)}, nil
}

// Next возвращает следующий живой ключ-значение в диапазоне.
func (it *Iter) Next() (key, value []byte, ok bool, err error) {
	if it.closed || it.finished { return nil, nil, false, nil }
	for {
		rt, err := it.buf.ReadByte() // Тип записи
		if err == io.EOF { it.finished = true; return nil, nil, false, nil }
		if err != nil { it.Close(); return nil, nil, false, err }
		var k []byte
		if err := readBytes(it.buf, &k); err != nil { it.Close(); return nil, nil, false, err }
		// проверка что запись лежит левее [start;end)
		if it.start != nil && bytes.Compare(k, it.start) < 0 {
			if rt == RecordTypePut { if err := skipBytes(it.buf); err != nil { it.Close(); return nil, nil, false, err } } 
			continue
		}
		// проверка что запись лежит правее end
		if it.end != nil && bytes.Compare(k, it.end) >= 0 { it.finished = true; return nil, nil, false, nil }
		if rt == RecordTypeDelete { continue } // Пропустить tombstone
		var v []byte
		if err := readBytes(it.buf, &v); err != nil { it.Close(); return nil, nil, false, err }
		return k, v, true, nil // Вернуть пару
	}
}

// Close закрывает итератор.
func (it *Iter) Close() error { it.closed = true; it.finished = true; return nil }

// readBytes читает length-префиксную строку из r.
func readBytes(r io.Reader, out *[]byte) error {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil { return err } 			// Читаем 4 байта, в которых хранится длина сроки
	*out = make([]byte, length) 																// Выделить буфер
	_, err := io.ReadFull(r, *out) 																// Прочитать ровно length байт
	return err
}
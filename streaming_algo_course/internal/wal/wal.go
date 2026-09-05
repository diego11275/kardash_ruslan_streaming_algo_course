package wal // Пакет wal реализует Write‑Ahead Log

import (
	"bufio" 				// Буферизованный ввод‑вывод
	"encoding/binary" 		// Кодирование чисел в бинарном виде
	"errors" 				// Работа с ошибками
	"fmt" 					// Форматированный вывод
	"io" 					// Интерфейсы ввода‑вывода
)

var ErrNotImplemented = errors.New("wal: функция не реализована") 

type OpType byte // Тип операции в WAL (1 байт)

const (
	OpPut    OpType = 1 
	OpDelete OpType = 2
)

type Record struct { // Одна запись WAL
	Type  OpType
	Key   []byte
	Value []byte
}

type Writer struct { // Писатель WAL
	w *bufio.Writer // Буферизованный writer для накопления записей
}

// Создать писатель с буферизацией
func NewWriter(w io.Writer) *Writer { return &Writer{w: bufio.NewWriter(w)} }

// Append добавляет запись в лог
func (w *Writer) Append(rec Record) error {
	keyLen := len(rec.Key)
	valLen := 0
	if rec.Type == OpPut { valLen = len(rec.Value) }
	// Type+len(key)+len(value) = 1+4+4
	totalLen := 9 + keyLen + valLen
	if err := binary.Write(w.w, binary.LittleEndian, uint32(totalLen)); err != nil { return fmt.Errorf("wal: write totalLen: %w", err) }
	if err := w.w.WriteByte(byte(rec.Type)); err != nil { return fmt.Errorf("wal: write opType: %w", err) }
	if err := binary.Write(w.w, binary.LittleEndian, uint32(keyLen)); err != nil { return fmt.Errorf("wal: write keyLen: %w", err) }
	if _, err := w.w.Write(rec.Key); err != nil { return fmt.Errorf("wal: write key: %w", err) }
	if err := binary.Write(w.w, binary.LittleEndian, uint32(valLen)); err != nil { return fmt.Errorf("wal: write valLen: %w", err) }
	if rec.Type == OpPut { if _, err := w.w.Write(rec.Value); err != nil { return fmt.Errorf("wal: write value: %w", err) } }
	return nil
}

func (w *Writer) Flush() error { return w.w.Flush() }
func (w *Writer) Close() error { return w.Flush() }

type Reader struct {
	r *bufio.Reader
}

// Создать читатель
func NewReader(r io.Reader) *Reader { return &Reader{r: bufio.NewReader(r)} }

// Next читает следующую запись из лога. Возвращает (запись, есть ли ещё, ошибка).
func (r *Reader) Next() (Record, bool, error) {
	var totalLen uint32
	// Прочитать общую длину записи
	if err := binary.Read(r.r, binary.LittleEndian, &totalLen); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) { return Record{}, false, nil }
		return Record{}, false, fmt.Errorf("wal: read totalLen: %w", err)
	}
	data := make([]byte, totalLen)
	if _, err := io.ReadFull(r.r, data); err != nil { return Record{}, false, nil }
	if len(data) < 1 { return Record{}, false, nil }
	offset := 0
	opType := OpType(data[offset]); 
	offset++
	if offset+4 > len(data) { return Record{}, false, nil }
	keyLen := binary.LittleEndian.Uint32(data[offset:]); 
	offset += 4
	if offset+int(keyLen) > len(data) { return Record{}, false, nil }
	key := make([]byte, keyLen)
	copy(key, data[offset:offset+int(keyLen)]); 
	offset += int(keyLen)
	if offset+4 > len(data) { return Record{}, false, nil }
	valLen := binary.LittleEndian.Uint32(data[offset:]); 
	offset += 4
	var value []byte
	if opType == OpPut {
		if offset+int(valLen) > len(data) { return Record{}, false, nil }
		value = make([]byte, valLen)
		copy(value, data[offset:offset+int(valLen)])
	}
	return Record{Type: opType, Key: key, Value: value}, true, nil
}
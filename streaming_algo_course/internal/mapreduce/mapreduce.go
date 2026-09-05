package mapreduce

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	"kvschool/internal/kv"
)

// Mapper превращает строку входа в набор пар (key,value) для промежуточного хранения.
// В курсе value обычно маленькое (например, varint(1) для wordcount).
type Mapper func(line []byte) ([]kv.Pair, error)

// Reducer сворачивает список value по одному логическому key.
type Reducer func(key []byte, values [][]byte) ([]byte, error)

// Run выполняет однопоточный map/reduce.
//
// Алгоритм хранения промежуточных данных:
// - каждая пара (k,v) записывается как отдельная запись с ключом k + 0x00 + seq(8 байт big-endian)
// - затем выполняется Scan по всему диапазону и группировка по префиксу до 0x00
//
// Такой подход намеренно требует упорядоченного итератора и хорошо «стыкуется» с SkipList/LSM.
func Run(ctx context.Context, r io.Reader, st kv.Store, mapFn Mapper, reduceFn Reducer) (kv.Store, error) {
	if mapFn == nil || reduceFn == nil {
		return nil, errors.New("mapreduce: требуются функции mapFn и reduceFn")
	}
	if st == nil {
		return nil, errors.New("mapreduce: требуется store")
	}

	sc := bufio.NewScanner(r)
	// На практике строки могут быть длиннее дефолтного лимита.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 4*1024*1024)

	var seq uint64
	for sc.Scan() {
		pairs, err := mapFn(sc.Bytes())
		if err != nil {
			return nil, fmt.Errorf("mapreduce: ошибка map: %w", err)
		}
		for _, p := range pairs {
			seq++
			rawKey := makeIntermediateKey(p.Key, seq)
			if err := st.Put(ctx, rawKey, p.Value); err != nil {
				return nil, fmt.Errorf("mapreduce: ошибка записи в store: %w", err)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("mapreduce: ошибка чтения ввода: %w", err)
	}

	out := newInMemoryOutput()

	it, err := st.Scan(ctx, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("mapreduce: ошибка сканирования store: %w", err)
	}
	defer it.Close()

	var (
		curKey    []byte
		curValues [][]byte
	)
	flush := func() error {
		if curKey == nil {
			return nil
		}
		v, err := reduceFn(curKey, curValues)
		if err != nil {
			return fmt.Errorf("mapreduce: ошибка reduce: %w", err)
		}
		if err := out.Put(ctx, curKey, v); err != nil {
			return fmt.Errorf("mapreduce: ошибка записи результата: %w", err)
		}
		curKey = nil
		curValues = curValues[:0]
		return nil
	}

	for {
		p, ok, err := it.Next()
		if err != nil {
			return nil, fmt.Errorf("mapreduce: ошибка итерации: %w", err)
		}
		if !ok {
			break
		}

		logicalKey, ok := splitIntermediateKey(p.Key)
		if !ok {
			return nil, fmt.Errorf("mapreduce: некорректный промежуточный ключ: %q", string(p.Key))
		}

		if curKey == nil {
			curKey = append([]byte(nil), logicalKey...)
		} else if !bytes.Equal(curKey, logicalKey) {
			if err := flush(); err != nil {
				return nil, err
			}
			curKey = append([]byte(nil), logicalKey...)
		}
		curValues = append(curValues, append([]byte(nil), p.Value...))
	}
	if err := flush(); err != nil {
		return nil, err
	}

	return out, nil
}

const intermediateSep = byte(0)

func makeIntermediateKey(logicalKey []byte, seq uint64) []byte {
	b := make([]byte, 0, len(logicalKey)+1+8)
	b = append(b, logicalKey...)
	b = append(b, intermediateSep)
	var tmp [8]byte
	binary.BigEndian.PutUint64(tmp[:], seq)
	b = append(b, tmp[:]...)
	return b
}

func splitIntermediateKey(raw []byte) (logical []byte, ok bool) {
	i := bytes.IndexByte(raw, intermediateSep)
	if i <= 0 {
		return nil, false
	}
	// хвост должен быть 8 байт (seq)
	if len(raw) != i+1+8 {
		return nil, false
	}
	return raw[:i], true
}

// WordCountMapper — контрольный map для демо: разбиение строки на слова, для каждого слова value=varint(1).
func WordCountMapper(line []byte) ([]kv.Pair, error) {
	words := strings.Fields(string(line))
	out := make([]kv.Pair, 0, len(words))
	for _, w := range words {
		out = append(out, kv.Pair{Key: []byte(w), Value: encodeVarint(1)})
	}
	return out, nil
}

// SumVarintReducer — контрольный reduce: суммирование varint значений.
func SumVarintReducer(key []byte, values [][]byte) ([]byte, error) {
	var sum int64
	for _, v := range values {
		x, err := decodeVarint(v)
		if err != nil {
			return nil, fmt.Errorf("reduce %q: %w", string(key), err)
		}
		sum += x
	}
	return encodeVarint(sum), nil
}

func encodeVarint(x int64) []byte {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutVarint(buf[:], x)
	return append([]byte(nil), buf[:n]...)
}

func decodeVarint(b []byte) (int64, error) {
	x, n := binary.Varint(b)
	if n <= 0 {
		return 0, errors.New("некорректная кодировка varint")
	}
	return x, nil
}

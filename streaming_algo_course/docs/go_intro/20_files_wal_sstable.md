## Трек второго дня: файлы, WAL, SSTable, LSM (под `make test-day2`)

Связанные материалы:
- Задание: `assignments/day2.md`
- Теория: `docs/day2_lsm_storage.md`
- Заготовки: `internal/wal/wal.go`, `internal/sstable/sstable.go`, `internal/lsm/lsm.go`
- KV-обёртка: `internal/kv/lsmstore/lsmstore.go`, тест: `internal/kv/lsmstore/lsmstore_day2_test.go`
- Шаблон заметки: `docs/note_template.md`

### Что от вас ждём
Сделать KV-хранилище, которое:
- быстро пишет (append/sequential I/O);
- переживает рестарт (crash recovery через WAL);
- читает по диапазону (через SSTable + итератор);
- умеет “подчищать” накопившиеся файлы (compaction), хотя минимально может быть достаточно базового уровня.

### 1) Go I/O: минимальный набор, который реально нужен
- `os.OpenFile`, `os.Create`, `file.Close()` (и обработка ошибок закрытия)
- `bufio.NewWriter` / `bufio.NewReader`
- `io.ReadFull` (чтение “ровно N байт”)
- `io.ReaderAt` (для SSTable, где нужен random access по индексу)

Практическое правило: если вы читаете длину `n`, то payload надо читать через `io.ReadFull`, иначе можно случайно получить частичное чтение и битый парсинг.

### 2) WAL: simplest thing that works
В `internal/wal` вам дан тип:
- `Record{Type, Key, Value}`
- `OpPut` / `OpDelete`

Рекомендуемый формат (чтобы было однозначно парсить):
- `u32 totalLen` (little endian)
- `u8 opType`
- `u32 keyLen` + key bytes
- `u32 valLen` + val bytes (только для Put; для Delete можно писать 0)

Почему length-prefixed для ожидаемых значений удобно:
- можно читать последовательно;
- можно отличать “конец файла” от “битой записи”;
- при крэше в середине записи можно аккуратно остановиться на последней целой записи.

### 3) Crash recovery: что считать “успешным”
Минимальная цель (как в `TestLSMStore_PersistAcrossRestart`):
- записали `Put(a,1)`;
- закрыли;
- открыли заново;
- `Get(a)` вернул `1`.

Значит при `Open(...)` нужно:
- найти WAL в `Options.Dir`;
- прочитать его последовательно и восстановить Memtable.

### 4) SSTable: минимальный скелет
В `internal/sstable` даны `Writer` и `Reader(ReaderAt, size)`.

Минимальный подход (понятный и тестопригодный):
- Data section: много записей подряд (каждая тоже length-prefixed по key/value)
- Sparse index: список “ключ → offset” (например, каждую N-ю запись)
- Footer: фиксированный формат с указателем на начало индекса и магией/версией

Идея sparse-index: при поиске диапазона можно:
- по индексу найти стартовый offset для `start`;
- читать данные последовательно до `end`.

### 5) LSM оркестрация (для второго дня)
В `internal/lsm` есть `Options{Dir, MemtableFlushThreshold}` и `Open`.
Типовой пайплайн:
- `Put/Delete`: сначала append в WAL, потом применить в Memtable
- если Memtable превысила порог → flush в новый SSTable, очистить/ротировать WAL
- `Get`: проверить Memtable, затем SSTable(ы) (свежие важнее старых)
- `Scan`: склеить итераторы Memtable + SSTables (смотрите подробней в `21_iterators_merge_and_compaction.md`)

### 6) Ошибки и “что нельзя”
- не использовать `panic` как управление потоком;
- не игнорировать ошибки I/O;
- не терять контекст ошибок (`fmt.Errorf("...: %w", err)`).

### 7) Контрольные точки (для самопроверки)
- **Crash test (ручной)**: запустить нагрузку, прервать процесс, поднять снова и убедиться, что данные восстановились.
  На ранних стадиях удобно написать маленький тест/утилиту, которая делает несколько `Put`, затем сразу `Open` заново и проверяет `Get`.

- **Важно**: `make test-day2` должен проходить.

### 8) Что написать в заметке (2–5 пунктов)
По `docs/note_template.md`:
- формат WAL (поля, endian, как отличаете “битую хвостовую запись”);
- формат SSTable (что в data, что в index/footer);
- как устроено восстановление при старте;
- как проверяли (тесты/микробенчи).

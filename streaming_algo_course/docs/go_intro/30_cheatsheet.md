## Небольшая шпаргалка

### 1) Команды, которые вы будете запускать часто
- Все тесты:

```bash
go test ./...
```

- По дням:

```bash
make test-day1
make test-day2
make test-day3
```

- Без кеша:

```bash
go test ./... -count=1
```

- Один тест:

```bash
go test ./internal/skiplist -run TestSkipList_ScanOrderAndRange -count=1
```

### 2) `kvtool`
Справка встроена в `cmd/kvtool/main.go`, базовые команды:

```bash
go run ./cmd/kvtool wordcount -in ./testdata/text_small.txt
go run ./cmd/kvtool load -count 10000
go run ./cmd/kvtool load -count 10000 -zipf 1.1
```

Флаг `-store` сейчас точно поддерживает `memmap` и `skiplist` (а `lsm` добавится по мере выполнения практики).

### 3) KV
Подробнее в `internal/kv/kv.go`:
- `ErrNotFound` — отсутствующий ключ;
- `Scan(ctx, start, end)` — диапазон **[start,end)**, `start/end == nil` → \(-∞\)/\( +∞\).

### 4) Сравнение ключей (`[]byte`)
- порядок: `bytes.Compare(a,b)`
- равенство: `bytes.Equal(a,b)`

### 5) Как писать правильно
- добавляйте контекст:

```go
return fmt.Errorf("open sstable: %w", err)
```

### 6) Частые баги, которые можете встретить в первый день
- `Put` не обновляет значение при повторном ключе;
- `Scan` делает `end` включительным (должен быть исключительным);
- `Scan(nil,nil)` не отдаёт все ключи;
- итератор не ленивый (собирает всё в массив).

Дополнительно: `docs/go_intro/10_skiplist_and_iterators.md`

### 7) Частые баги,  которые можете встретить во второй день
- неверный парсинг length-prefixed записей (не использовали `io.ReadFull`);
- не закрыли файл / потеряли ошибку `Close`;
- recovery читает WAL, но не применяет Delete (tombstone);
- итераторы мерджатся в неверном порядке “новизны”.

Дополнительно: `docs/go_intro/20_files_wal_sstable.md` и `docs/go_intro/21_iterators_merge_and_compaction.md`

### 8) Что сдавать как “заметку”
Шаблон: `docs/note_template.md`.
Минимум: что реализовано, семантика `Scan`, формат WAL/SSTable (для второго дня), как проверяли (тесты/бенчи).

### 9) Полезные материалы курса
- День первый, теория: `docs/day1_hls_map_reduce_skiplist.md`
- День второй, теория: `docs/day2_lsm_storage.md`
- День третий, теория: `docs/day3_bloom_cluster_streaming.md`

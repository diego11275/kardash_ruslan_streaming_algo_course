## Что вам понадобится из Go

Этот мини-курс — быстрый “рамп-ап” в Go, достаточный для выполнения задач: `SkipList` (день 1) и `WAL/SSTable/LSM` (день 2), плюс базовая диагностика, тесты и бенчмарки.

### Что вы должны уметь к концу курса
- **Собирать и проверять**: `go test ./...`, `make test-day1`, `make test-day2`.
- **Запускать утилиту**: `go run ./cmd/kvtool ...` и понимать ошибки.
- **Писать идиоматичный Go**: чистые ошибки (без паник в библиотечном коде), аккуратные итераторы, минимум лишних аллокаций на часто вызываемом (горячем) пути.

### Быстрый старт по репозиторию
- **Проверка**:

```bash
go test ./...
make test-day1
make test-day2
```

- **Демо wordcount**:

```bash
go run ./cmd/kvtool wordcount -in ./testdata/text_small.txt
go run ./cmd/kvtool wordcount -in ./testdata/text_small.txt -store memmap
go run ./cmd/kvtool wordcount -in ./testdata/text_small.txt -store skiplist
```

### Порядок чтения
1. `00_git.md` — Git-цикл работы с этим репозиторием.
2. `01_env_setup.md` — окружение Ubuntu + VS Code + Go 1.22.
3. `02_go_basics.md` — базовый Go, достаточный для KV-движков.
4. `03_slices_maps_strings_bytes.md` — строки/байты/слайсы, аллокации, эффективность.
5. `04_testing_and_bench.md` — тесты/бенчмарки и как читать фейлы.
6. `10_skiplist_and_iterators.md` — трек выполнения заданий первого дня.
7. `11_debugging.md` — практичный дебаг/диагностика.
8. `20_files_wal_sstable.md` — трек выполнения заданий второго дня.
9. `21_iterators_merge_and_compaction.md` — merge/compaction на итераторах.
10. `30_cheatsheet.md` — шпаргалка.

### Полезные ссылки внутри репозитория
- Теория заданий первого дня: `docs/day1_hls_map_reduce_skiplist.md`
- Теория day2: `docs/day2_lsm_storage.md`
- Теория day3 (опционально): `docs/day3_bloom_cluster_streaming.md`
- Чеклист ревью: `docs/review_checklist.md`
- Шаблон заметки: `docs/note_template.md`

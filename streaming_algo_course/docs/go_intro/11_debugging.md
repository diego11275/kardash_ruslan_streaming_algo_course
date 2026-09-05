## Мини-набор отладки решений

### 1) Сузить проблему: пакет - тест - место
- Все тесты без кеша:

```bash
go test ./... -count=1
```

- Только day1/day2:

```bash
make test-day1
make test-day2
```

- Один пакет:

```bash
go test ./internal/skiplist -count=1
```

- Один тест:

```bash
go test ./internal/skiplist -run TestSkipList_ScanOrderAndRange -count=1
```

### 2) Логи в тестах
Используйте `t.Logf`, а не `fmt.Println`:
- логи видны при падении или при запуске с `-v`.

```bash
go test ./internal/skiplist -run TestName -count=1 -v
```

### 3) Проверить семантику границ `Scan`
Самое частое:
- `end` ошибочно делают включительным;
- путают “первый >= start” и “первый > start”.

Быстрый способ: в отладочном тесте/логах напечатать первые 3 ключа `Scan(start,end)` и сравнить с ожидаемым.

### 4) Воспроизводимость: seed
Если у вас RNG внутри структуры:
- держите `rand.Rand` как поле структуры;
- инициализируйте `rand.New(rand.NewSource(seed))` в `New(seed)`;
- не используйте глобальный `rand` без необходимости (может вносить “флейки”).

### 5) Когда нужно смотреть аллокации
Если “всё правильно, но медленно”:

```bash
go test ./internal/skiplist -bench . -benchmem
```

Смотрите `allocs/op` и думайте, где можно предвыделить или убрать конвертацию `[]byte -> string` в горячих местах.

### 6) Мини-проверка I/O для решений заданий второго дня
Для WAL/SSTable типовые ошибки:
- забыли закрыть файл (`Close`) или не обработали ошибку `Close`;
- неверно распарсили length-prefixed запись;
- не учли частичное чтение (`io.ReadFull`).

Полезные интерфейсы: `io.Reader`, `io.Writer`, `bufio.Reader/Writer`.

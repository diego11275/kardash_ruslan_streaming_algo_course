## Подготовка окружения (рекомендуемая)

В `go.mod` в этом репозитории указано `go 1.22`, поэтому лучше сразу поставить Go 1.22.x.

### 1) Ubuntu: базовые пакеты

```bash
sudo apt update
sudo apt install -y git curl ca-certificates build-essential
```

Проверить:

```bash
git --version
```

### 2) Установка Go 1.22 (рекомендуется официальный архив)
Пакеты Ubuntu часто отстают по версии Go. Чтобы гарантировать 1.22:

```bash
cd /tmp
curl -fsSLO https://go.dev/dl/go1.22.11.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.11.linux-amd64.tar.gz
```

Добавить Go в PATH (для bash):

```bash
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
source ~/.bashrc
```

Проверить:

```bash
go version
go env GOPATH GOMODCACHE
```

Примечание: если у вас zsh — вместо `~/.bashrc` используйте `~/.zshrc`.

### 3) Клонирование и запуск проверок

```bash
git clone https://gitverse.ru/aloshkarev/ws_algo_2026
cd ws_algo_2026

go test ./...
make test-day1
make test-day2
```

### 4) VS Code: установка и минимальная настройка
Установить VS Code удобнее через snap или deb-пакет (любой вариант подойдет).

#### Расширения
- **Go** (id: `golang.go`) — `gopls`, тесты, форматирование, подсказки.

После установки расширения откройте проект и согласитесь на установку инструментов, которые предложит VS Code (обычно: `gopls`, `dlv`).

#### Форматирование
Убедитесь, что форматирование Go включено на сохранение:
- Format On Save = On
- Go: Format Tool = `gofmt` (или `gofumpt`, но для курса достаточно `gofmt`)

### 5) Запуск `kvtool`
Команды, которые точно есть сейчас:

```bash
go run ./cmd/kvtool wordcount -in ./testdata/text_small.txt
go run ./cmd/kvtool load -count 10000
```

Флаг `-store` (из `cmd/kvtool/main.go`) поддерживает как минимум: `memmap`, `skiplist` (а `lsm` добавится по мере выполнения практики).

### 6) Отладка (Debug) `kvtool` в VS Code
Обычно достаточно конфигурации, где запускается `./cmd/kvtool` с аргументами.

Пример `launch.json` (создайте через “Run and Debug” → “create a launch.json”):

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "kvtool wordcount (memmap)",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/cmd/kvtool",
      "args": ["wordcount", "-in", "${workspaceFolder}/testdata/text_small.txt", "-store", "memmap"]
    }
  ]
}
```

### 7) Возможные проблемы и быстрые фиксы
- **`go: command not found`**: PATH не применился - перезапустите терминал/VS Code, проверьте `echo $PATH`.
- **Версия Go не 1.22**: где-то стоит старая Go (например, `/usr/bin/go`) - проверьте `which go` и порядок PATH.
- **VS Code не видит `gopls`**: откройте Command Palette - “Go: Install/Update Tools”.

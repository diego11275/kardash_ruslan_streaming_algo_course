#!/bin/bash
set -e

echo "Запуск базовых тестов"
go test ./...

echo "= Проверка решений первого дня (реализация структуры SkipList) ="
if go test -tags=day1 ./internal/skiplist/... >/dev/null 2>&1; then
    echo "Тесты первого дня прошли"
else
    echo "Тесты первого дня упали (или не реализованы)"
fi

echo "= Проверка решений второго дня (реализация структуры LSM) ="
if go test -tags=day2 ./internal/lsm/... ./internal/sstable/... >/dev/null 2>&1; then
    echo "Тесты второго дня прошли"
else
    echo "Тесты второго дня упали (или не реализованы)"
fi

echo "= Проверка решений третьего дня (реализация структуры Bloom и Stream) ="
if go test -tags=day3 ./internal/bloom/... ./internal/stream/... >/dev/null 2>&1; then
    echo "Тесты третьего дня прошли"
else
    echo "Тесты третьего дня упали (или не реализованы)"
fi

echo "Готово."

.PHONY: test test-day1 test-day2 test-day3 demo-wordcount

test:
	go test ./...

test-day1:
	go test -tags=day1 ./...

test-day2:
	go test -tags=day2 ./...

test-day3:
	go test -tags=day3 ./...

demo-wordcount:
	go run ./cmd/kvtool wordcount -in ./testdata/text_small.txt


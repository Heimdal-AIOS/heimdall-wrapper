APP:=heimdal

.PHONY: build run test clean

build:
	go build -o bin/$(APP) ./cmd/heimdal

run: build
	./bin/$(APP) --help

test:
	go test ./...

clean:
	rm -rf bin


BIN  := bin/nfe
PKG  := ./...

.PHONY: all build test test-race cover lint tidy clean run

all: build

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/nfe

test:
	go test $(PKG)

test-race:
	go test -race $(PKG)

cover:
	go test -coverprofile=coverage.out $(PKG)
	go tool cover -func=coverage.out | tail -20

lint:
	go vet $(PKG)

tidy:
	go mod tidy

clean:
	rm -rf bin coverage.out

run: build
	$(BIN) $(ARGS)

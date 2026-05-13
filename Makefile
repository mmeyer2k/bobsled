.PHONY: all build test test-race lint clean assets

GO ?= go
BIN ?= ./bin

all: build

assets:
	cp systemd/bobsled@.service assets/bobsled@.service

build: assets
	mkdir -p $(BIN)
	$(GO) build -o $(BIN)/bobsled ./cmd/bobsled
	$(GO) build -o $(BIN)/bobsled-mint ./cmd/bobsled-mint

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

lint:
	$(GO) vet ./...

clean:
	rm -rf $(BIN)

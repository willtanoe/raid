.PHONY: build test test-race check clean install uninstall

PREFIX ?= /usr/local
VERSION ?= 0.2.1
GO ?= go
GOFMT ?= gofmt
LDFLAGS := -s -w -X github.com/willtanoe/raid/internal/raid.Version=$(VERSION)

build:
	mkdir -p bin
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o bin/raid ./cmd/raid

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

check:
	test -z "$$($(GOFMT) -l cmd/raid/*.go internal/raid/*.go)"
	$(GO) vet ./...
	$(GO) test ./...

clean:
	rm -rf bin

install: build
	install -Dm755 bin/raid "$(DESTDIR)$(PREFIX)/bin/raid"

uninstall:
	rm -f "$(DESTDIR)$(PREFIX)/bin/raid"

.PHONY: setup dev build test lint tidy clean mcp

setup:
	go mod download
	cd frontend && npm install

dev:
	$(WAILS) dev $(WAILS_TAGS)

VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo dev)
WAILS ?= $(shell go env GOPATH)/bin/wails
WAILS_TAGS ?= $(if $(filter Linux,$(shell uname -s)),-tags webkit2_41,)

build:
	cd frontend && npm install && cd ..
	$(WAILS) build $(WAILS_TAGS) -ldflags "-X main.version=$(VERSION)"

test:
	go vet ./...
	go test ./... -v
	cd frontend && npm run build

lint:
	go vet ./...
	cd frontend && npx vue-tsc --noEmit

tidy:
	go mod tidy

clean:
	rm -rf build/

mcp:
	go build -o build/bin/imagecaster-mcp ./cmd/imagecaster-mcp

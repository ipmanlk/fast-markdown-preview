# Fast Markdown Preview: local development Makefile.
#
# Common tasks:
#   make build      - build the server binary into server/bin/
#   make run        - build and run the server on a free port
#   make test       - run Go + Python tests
#   make fmt        - format Go code
#   make lint       - vet Go code
#   make clean      - remove build artifacts
#   make release    - dry-run a full GoReleaser build locally (no upload)
#   make install    - copy the plugin into the ST Packages dir (best-effort)

VERSION  ?= dev
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILDTIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILDTIME)

SERVER_DIR := server
BIN_DIR    := $(SERVER_DIR)/bin
BINARY     := $(BIN_DIR)/fast-md-preview

GO        := go
PYTHON    := python3

.PHONY: all build run test test-go test-python fmt lint clean release install help

all: build

build:
	@mkdir -p $(BIN_DIR)
	cd $(SERVER_DIR) && $(GO) build -ldflags="$(LDFLAGS)" -o ../$(BINARY) .

run: build
	./$(BINARY) --port 0 --idle-timeout 0

test: test-go test-python

test-go:
	cd $(SERVER_DIR) && $(GO) test -timeout 60s ./...

test-python:
	$(PYTHON) -m pytest tests/ -v

fmt:
	cd $(SERVER_DIR) && $(GO) fmt ./...

lint:
	cd $(SERVER_DIR) && $(GO) vet ./...

release:
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not installed; see https://goreleaser.com/install/"; exit 1; }
	goreleaser release --snapshot --clean

clean:
	rm -rf $(BIN_DIR)
	rm -f tests/fmp-server-test

install: build
	@echo "Copying plugin files into ST Packages dir (best-effort)..."
	@for d in \
	  "$$HOME/.config/sublime-text/Packages" \
	  "$$HOME/Library/Application Support/Sublime Text/Packages" \
	  "$$APPDATA/Sublime Text/Packages"; do \
	  if [ -d "$$d" ]; then \
	    dest="$$d/FastMarkdownPreview"; \
	    mkdir -p "$$dest/server"; \
	    cp -r fast_markdown_preview.py *.sublime-settings *.sublime-commands *.sublime-keymap Main.sublime-menu package.json dependencies.json .no-sublime-package .python-version "$$dest/"; \
	    cp $(BINARY) "$$dest/server/fast-md-preview"; \
	    chmod +x "$$dest/server/fast-md-preview"; \
	    echo "Installed to $$dest"; \
	  fi; \
	done

help:
	@echo "Fast Markdown Preview targets:"
	@echo "  build    - build the Go server into server/bin/"
	@echo "  run      - build and run the server on a free port"
	@echo "  test     - run Go + Python tests"
	@echo "  fmt      - format Go code"
	@echo "  lint     - vet Go code"
	@echo "  clean    - remove build artifacts"
	@echo "  release  - dry-run a GoReleaser build locally (no upload)"
	@echo "  install  - copy the plugin into the ST Packages dir"

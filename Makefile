BINARY  := catc
MODULE  := github.com/mehedikhan/cat
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PREFIX  ?= /usr/local
BINDIR  := $(PREFIX)/bin
LDFLAGS := -s -w -X main.version=$(VERSION)

PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

.PHONY: all build install uninstall test examples fmt vet dist clean

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/$(BINARY)

# Install into $(PREFIX)/bin. Use `sudo make install` for /usr/local,
# or `make install PREFIX=$$HOME/.local` for a user-level install.
install: build
	@mkdir -p "$(BINDIR)"
	install -m 0755 $(BINARY) "$(BINDIR)/$(BINARY)"
	@echo "installed $(BINDIR)/$(BINARY)"
	@command -v $(BINARY) >/dev/null 2>&1 || \
	  echo "note: $(BINDIR) is not on your PATH"

uninstall:
	rm -f "$(BINDIR)/$(BINARY)"
	@echo "removed $(BINDIR)/$(BINARY)"

test:
	go test ./...

examples: build
	@for f in examples/*.cat; do echo "--- $$f"; ./$(BINARY) run $$f; done

fmt:
	gofmt -l -w ./cmd ./internal

vet:
	go vet ./...

# Cross-compile release archives into dist/.
dist: clean-dist
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
	  os=$${platform%/*}; arch=$${platform#*/}; \
	  ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
	  echo "building $$os/$$arch"; \
	  stage="dist/catc_$(VERSION)_$${os}_$${arch}"; \
	  mkdir -p "$$stage"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
	    go build -trimpath -ldflags "$(LDFLAGS)" -o "$$stage/$(BINARY)$$ext" ./cmd/$(BINARY) || exit 1; \
	  cp README.md HOW_IT_WORKS.md LICENSE "$$stage/"; \
	  cp -r examples "$$stage/"; \
	  if [ "$$os" = "windows" ]; then \
	    (cd dist && zip -qr "catc_$(VERSION)_$${os}_$${arch}.zip" "catc_$(VERSION)_$${os}_$${arch}"); \
	  else \
	    tar -czf "$$stage.tar.gz" -C dist "catc_$(VERSION)_$${os}_$${arch}"; \
	  fi; \
	  rm -rf "$$stage"; \
	done
	@cd dist && shasum -a 256 * > checksums.txt 2>/dev/null || sha256sum * > checksums.txt
	@echo; ls -1 dist

clean-dist:
	rm -rf dist

clean: clean-dist
	rm -f $(BINARY)

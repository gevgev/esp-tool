BINARY     := esp-tool
BUILD_DIR  := bin
CMD_PATH   := ./cmd/esp-tool

# Default install target: one directory up, into the esphome repo
ESPHOME_DIR ?= ../esphome/esphome

ZSH_COMPLETION_DIR ?= /usr/local/share/zsh/site-functions

# Version info embedded via -ldflags, mirroring goreleaser's release builds
# (see .goreleaser.yaml) so `esp-tool --version` reports something useful
# for local/dev builds too, instead of the "dev" fallback.
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null)
DATE       := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build build-windows install install-completions clean test test-race vet test-windows-compile

build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) $(CMD_PATH)

# Cross-compile for Windows (amd64). Produces bin/esp-tool.exe.
# Requires no special toolchain — Go's built-in cross-compiler handles it.
build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY).exe $(CMD_PATH)

# Verify the Windows build compiles without running it (fast CI gate).
test-windows-compile:
	GOOS=windows GOARCH=amd64 go build ./...

install: build
	cp $(BUILD_DIR)/$(BINARY) $(ESPHOME_DIR)/$(BINARY)
	@echo "Installed $(BINARY) → $(ESPHOME_DIR)/$(BINARY)"

install-completions:
	install -d $(ZSH_COMPLETION_DIR)
	install -m 644 completions/_esp-tool $(ZSH_COMPLETION_DIR)/_esp-tool
	@echo "Installed zsh completion → $(ZSH_COMPLETION_DIR)/_esp-tool"
	@echo "Run: autoload -Uz compinit && compinit"

test:
	go test ./...

test-race:
	go test -race -count=1 ./...

vet:
	go vet ./...

clean:
	rm -f $(BUILD_DIR)/$(BINARY)

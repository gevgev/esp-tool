BINARY     := esp-tool
BUILD_DIR  := bin
CMD_PATH   := ./cmd/esp-tool

# Default install target: one directory up, into the esphome repo
ESPHOME_DIR ?= ../esphome/esphome

ZSH_COMPLETION_DIR ?= /usr/local/share/zsh/site-functions

.PHONY: build install install-completions clean test test-race vet

build:
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD_PATH)

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

BINARY     := esp-tool
BUILD_DIR  := bin
CMD_PATH   := ./cmd/esp-tool

# Default install target: one directory up, into the esphome repo
ESPHOME_DIR ?= ../esphome/esphome

.PHONY: build install clean

build:
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD_PATH)

install: build
	cp $(BUILD_DIR)/$(BINARY) $(ESPHOME_DIR)/$(BINARY)
	@echo "Installed $(BINARY) → $(ESPHOME_DIR)/$(BINARY)"

clean:
	rm -f $(BUILD_DIR)/$(BINARY)

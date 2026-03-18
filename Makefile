BINARY   := ios-pilot
INSTALL_DIR := $(HOME)/.local/bin
CMD_PATH := ./cmd/ios-pilot

.PHONY: build test install deploy clean

build:
	go build -o $(BINARY) $(CMD_PATH)

test:
	go test ./... -v

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

deploy: install

clean:
	rm -f $(BINARY)

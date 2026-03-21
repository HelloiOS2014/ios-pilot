BINARY   := ios-pilot
INSTALL_DIR := $(HOME)/.local/bin
CMD_PATH := ./cmd/ios-pilot

.PHONY: build test test-integration install deploy clean

build:
	go build -o $(BINARY) $(CMD_PATH)

test:
	go test ./... -v

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

deploy: install
	@echo ""
	@echo "Binary installed. To also install the Claude Code Skill, run:"
	@echo "  ./install.sh"

test-integration:
	IOS_DEVICE_CONNECTED=1 go test ./test/integration/ -v -timeout 300s -count=1

clean:
	rm -f $(BINARY)

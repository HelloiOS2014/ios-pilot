BINARY   := ios-pilot
INSTALL_DIR := $(HOME)/.local/bin
CMD_PATH := ./cmd/ios-pilot
SKILL_DIR := $(HOME)/.claude/skills/ios-pilot

.PHONY: build test test-integration install install-skill deploy clean

build:
	go build -o $(BINARY) $(CMD_PATH)

test:
	go test ./... -v

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

install-skill:
	mkdir -p $(SKILL_DIR)
	cp skills/ios-pilot/SKILL.md $(SKILL_DIR)/SKILL.md
	@echo "Installed skill to $(SKILL_DIR)/SKILL.md"

deploy: install install-skill

test-integration:
	IOS_DEVICE_CONNECTED=1 go test ./test/integration/ -v -timeout 300s -count=1

clean:
	rm -f $(BINARY)

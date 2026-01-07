BINARY_NAME := claude-env
BUILD_DIR := .
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: all build install uninstall clean docker help

all: build

## Build the binary
build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/claude-env

## Install binary to ~/.local/bin (creates hard link)
install: build
	@mkdir -p $(INSTALL_DIR)
	@ln -f $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"
	@echo "Make sure $(INSTALL_DIR) is in your PATH"

## Remove installed binary
uninstall:
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Removed $(BINARY_NAME) from $(INSTALL_DIR)"

## Build Docker image
docker: build
	@cp Dockerfile internal/embedded/Dockerfile
	./$(BINARY_NAME) build-image --force

## Run tests
test:
	go test ./...

## Clean build artifacts
clean:
	@rm -f $(BUILD_DIR)/$(BINARY_NAME)
	@echo "Cleaned build artifacts"

## Show help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build      Build the binary"
	@echo "  install    Build and install to ~/.local/bin"
	@echo "  uninstall  Remove from ~/.local/bin"
	@echo "  docker     Sync Dockerfile and rebuild Docker image"
	@echo "  test       Run tests"
	@echo "  clean      Remove build artifacts"
	@echo "  help       Show this help"

# Vanilla Continuity Makefile

# Build variables
BINARY=continuity
VERSION=1.0.0
BUILD_DIR=bin
SRC=main.go

# CGO requirements
export C_INCLUDE_PATH=$(HOME)/libudev-temp/usr/include
export CGO_LDFLAGS=-L$(HOME)/lib

.PHONY: all build clean test install uninstall

all: build

build:
@echo "Building $(BINARY)..."
@go build -o $(BUILD_DIR)/$(BINARY) ./$(SRC)
@echo "Build complete: $(BUILD_DIR)/$(BINARY)"

clean:
@echo "Cleaning..."
@go clean -cache
@rm -rf $(BUILD_DIR)
@echo "Clean complete"

test:
@echo "Running tests..."
@go test -v ./...

install: build
@echo "Installing $(BINARY)..."
@sudo install -m 755 $(BUILD_DIR)/$(BINARY) /usr/bin/$(BINARY)
@echo "Installing systemd service..."
@sudo cp extras/systemd/vanilla-continuity.service /etc/systemd/system/
@sudo systemctl daemon-reload
@echo "Installation complete"
@echo "To enable service: sudo systemctl enable --now vanilla-continuity"

uninstall:
@echo "Uninstalling $(BINARY)..."
@sudo systemctl stop vanilla-continuity 2>/dev/null || true
@sudo systemctl disable vanilla-continuity 2>/dev/null || true
@sudo rm -f /usr/bin/$(BINARY)
@sudo rm -f /etc/systemd/system/vanilla-continuity.service
@sudo systemctl daemon-reload
@echo "Uninstall complete"

help:
@echo "Vanilla Continuity Makefile"
@echo ""
@echo "Usage:"
@echo "  make build      - Build the binary"
@echo "  make clean      - Clean build artifacts"
@echo "  make test       - Run tests"
@echo "  make install    - Install binary and systemd service"
@echo "  make uninstall  - Uninstall binary and systemd service"
@echo "  make help       - Show this help"

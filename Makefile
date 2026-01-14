# Makefile

APP_NAME = godis
GO = go
GOFLAGS = 
LDFLAGS = -ldflags="-extldflags=-static"
SRC = cmd/main.go
BINARY = $(APP_NAME).exe
BUILD_DIR = build
DIST_DIR = dist
VERSION = 1.0.0

GREEN = \033[0;32m
YELLOW = \033[1;33m
RED = \033[0;31m
NC = \033[0m 

check-env:
	@echo -e "$(YELLOW)Checking environment...$(NC)"
	@which $(GO) > /dev/null || (echo -e "$(RED)Error: Go not found$(NC)" && exit 1)
	@if [ -z "$$MSYSTEM" ]; then \
		echo -e "$(RED)Warning: Not in MSYS2 environment. Please run from MSYS2 MinGW64.$(NC)"; \
		exit 1; \
	else \
		echo -e "$(GREEN)Environment: $$MSYSTEM$(NC)"; \
	fi
	@if [ ! -f "/mingw64/lib/libportaudio.a" ]; then \
		echo -e "$(RED)Error: libportaudio.a not found. Install with: pacman -S mingw-w64-x86_64-portaudio$(NC)"; \
		exit 1; \
	else \
		echo -e "$(GREEN)libportaudio.a found$(NC)"; \
	fi

deps: check-env
	@echo -e "$(YELLOW)Checking Go dependencies...$(NC)"
	@if [ -f "go.mod" ]; then \
		$(GO) mod download; \
		echo -e "$(GREEN)Go dependencies updated$(NC)"; \
	else \
		echo -e "$(YELLOW)go.mod not found$(NC)"; \
	fi

build: deps
	@echo -e "$(YELLOW)Building $(APP_NAME)...$(NC)"
	@export CGO_ENABLED=1 && \
	 export GOOS=windows && \
	 export GOARCH=amd64 && \
	 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(SRC)
	@echo -e "$(GREEN)Built: $(BUILD_DIR)/$(BINARY)$(NC)"

quick:
	@mkdir -p $(BUILD_DIR)
	@export CGO_ENABLED=1 && \
	 export GOOS=windows && \
	 export GOARCH=amd64 && \
	 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(SRC)
	@echo -e "$(GREEN)Quick build completed$(NC)"

# Checking static linking
check-static: build
	@echo -e "$(YELLOW)Checking static linking...$(NC)"
	@if command -v objdump > /dev/null; then \
		count=$$(objdump -p $(BUILD_DIR)/$(BINARY) | grep "DLL Name" | wc -l); \
		if [ $$count -eq 0 ]; then \
			echo -e "$(GREEN)✓ Fully static binary$(NC)"; \
		else \
			echo -e "$(RED)✗ DLL dependencies found:$(NC)"; \
			objdump -p $(BUILD_DIR)/$(BINARY) | grep "DLL Name"; \
		fi; \
	elif command -v ldd > /dev/null; then \
		echo -e "Checking via ldd:"; \
		ldd $(BUILD_DIR)/$(BINARY) 2>/dev/null || echo -e "$(GREEN)✓ Static or minimal dependencies$(NC)"; \
	else \
		echo -e "$(YELLOW)Unable to check dependencies (objdump or ldd required)$(NC)"; \
	fi

run: build
	@echo -e "$(YELLOW)Starting application...$(NC)"
	@./$(BUILD_DIR)/$(BINARY)

clean:
	@echo -e "$(YELLOW)Cleaning...$(NC)"
	@rm -rf $(BUILD_DIR) $(DIST_DIR) *.exe
	@$(GO) clean -cache
	@echo -e "$(GREEN)Cleanup completed$(NC)"

# Installation (copy to system folder)
install: build
	@echo -e "$(YELLOW)Installing to /usr/local/bin...$(NC)"
	@if [ -f "$(BUILD_DIR)/$(BINARY)" ]; then \
		cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(APP_NAME).exe; \
		echo -e "$(GREEN)Installed: /usr/local/bin/$(APP_NAME).exe$(NC)"; \
	else \
		echo -e "$(RED)Error: binary not found$(NC)"; \
	fi

rebuild: clean quick
	@./$(BUILD_DIR)/$(BINARY)

help:
	@echo -e "$(GREEN)Available targets:$(NC)"
	@echo -e "  $(YELLOW)make build$(NC)     - Standard build"
	@echo -e "  $(YELLOW)make quick$(NC)     - Quick build (no checks)"
	@echo -e "  $(YELLOW)make run$(NC)       - Build and run"
	@echo -e "  $(YELLOW)make check-static$(NC) - Check static linking"
	@echo -e "  $(YELLOW)make clean$(NC)     - Cleanup"
	@echo -e "  $(YELLOW)make install$(NC)   - Install to system"
	@echo -e "  $(YELLOW)make deps$(NC)      - Install dependencies"
	@echo -e "  $(YELLOW)make help$(NC)      - Show this help"

all: build
static: check-static

.DEFAULT_GOAL := help

.PHONY: all build clean run test deps help release debug install check-static portable quick
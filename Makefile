# Makefile

APP_NAME := ukrainian_call_center_scam_goev
BIN_DIR  := .bin
CMD_DIR  := ./cmd

GO := go

# Флаги сборки: меньше размер, без путей, без CGO => максимально переносимо
LDFLAGS := -s -w
GCFLAGS :=
ASMFLAGS :=

OUT_LINUX_AMD64 := $(BIN_DIR)/$(APP_NAME)-linux-amd64
OUT_LINUX_ARM64 := $(BIN_DIR)/$(APP_NAME)-linux-arm64

.PHONY: all build build-linux build-linux-arm64 run clean test

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/$(APP_NAME) $(CMD_DIR)

# Статический Linux amd64 бинарник (максимально "portable" по зависимостям)
build-linux:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" \
	-o $(OUT_LINUX_AMD64) $(CMD_DIR)

# Статический Linux arm64 бинарник (для ARM серверов/одноплатников)
build-linux-arm64:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" \
	-o $(OUT_LINUX_ARM64) $(CMD_DIR)

run: build
	./$(BIN_DIR)/$(APP_NAME)

test:
	$(GO) test ./...

clean:
	@rm -rf $(BIN_DIR)
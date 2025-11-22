.PHONY: build run clean test install dev lint format

# 变量定义
BINARY_NAME=antigravity
BUILD_DIR=build
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO_FLAGS=-ldflags="-X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)'"

# 默认目标
all: clean build

# 编译
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@echo "Embedding public files..."
	@mkdir -p internal/embed/public
	@cp -r public/* internal/embed/public/ 2>/dev/null || true
	@go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/antigravity
	@echo "Build complete!"

# 交叉编译
build-linux:
	@echo "Building for Linux..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/antigravity

build-windows:
	@echo "Building for Windows..."
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/antigravity

build-darwin:
	@echo "Building for macOS..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/antigravity
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/antigravity

build-all: build-linux build-windows build-darwin

# 运行
run: build
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME) serve

# 开发模式（自动重载）
dev:
	@echo "Running in development mode..."
	@go run ./cmd/antigravity serve --config ./config.yaml

# 测试
test:
	@echo "Running tests..."
	@go test -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html

# 代码检查
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

# 代码格式化
format:
	@echo "Formatting code..."
	@go fmt ./...
	@goimports -w .

# 清理
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

# 安装
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# 依赖管理
deps:
	@echo "Downloading dependencies..."
	@go mod download

tidy:
	@echo "Tidying dependencies..."
	@go mod tidy

# 初始化项目目录
init-dirs:
	@echo "Creating project directories..."
	@mkdir -p data/accounts data/keys data/usage logs uploads
	@echo "Directories created."

# 显示帮助
help:
	@echo "Antigravity API Proxy - Makefile commands"
	@echo ""
	@echo "Usage:"
	@echo "  make build          - Build the binary"
	@echo "  make run            - Build and run the server"
	@echo "  make dev            - Run in development mode"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage"
	@echo "  make lint           - Run linter"
	@echo "  make format         - Format code"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make install        - Install binary to /usr/local/bin"
	@echo "  make build-all      - Build for all platforms"
	@echo "  make init-dirs      - Initialize project directories"
	@echo "  make help           - Show this help"

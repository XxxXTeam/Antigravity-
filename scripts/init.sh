#!/bin/bash

# Antigravity API Proxy - 初始化脚本
# 用于快速设置Go版本项目

set -e

echo "================================"
echo "Antigravity API Proxy 初始化"
echo "================================"
echo ""

# 检查Go版本
echo "检查Go环境..."
if ! command -v go &> /dev/null; then
    echo "错误: 未找到Go，请先安装Go 1.21或更高版本"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "✓ Go版本: $GO_VERSION"
echo ""

# 创建必要目录
echo "创建项目目录..."
mkdir -p data/accounts
mkdir -p data/keys
mkdir -p data/usage
mkdir -p logs
mkdir -p uploads

# 创建.gitkeep文件
touch data/.gitkeep
touch logs/.gitkeep
touch uploads/.gitkeep

echo "✓ 目录创建完成"
echo ""

# 复制配置文件模板
if [ ! -f "config.yaml" ] && [ ! -f "data/config.yaml" ]; then
    echo "创建配置文件..."
    cp config.yaml.example config.yaml 2>/dev/null || true
    echo "✓ 配置文件创建完成: config.yaml"
    echo "  请编辑该文件设置管理员密码和其他选项"
else
    echo "✓ 配置文件已存在"
fi
echo ""

# 下载依赖
echo "下载Go依赖..."
go mod download
echo "✓ 依赖下载完成"
echo ""

# 编译项目
echo "编译项目..."
make build
echo "✓ 编译完成: build/antigravity"
echo ""

echo "================================"
echo "初始化完成！"
echo "================================"
echo ""
echo "下一步:"
echo "  1. 编辑 config.yaml 设置管理员密码"
echo "  2. 运行 make run 或 ./build/antigravity serve"
echo "  3. 访问 http://localhost:8045/admin"
echo ""
echo "更多命令:"
echo "  make help     - 查看所有Make命令"
echo "  antigravity help - 查看CLI帮助"
echo ""

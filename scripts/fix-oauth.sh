#!/bin/bash

# OAuth 2.0 快速修复脚本

echo "======================================"
echo "Google OAuth 2.0 配置修复向导"
echo "======================================"
echo ""

echo "检测到的问题："
echo "1. JSON解析错误 - 已修复（改进错误处理）"
echo "2. Google OAuth 2.0 策略不合规"
echo ""

echo "修复步骤："
echo ""
echo "【步骤 1/3】检查OAuth配置"
echo "----------------------------------------"
echo "客户端ID: 1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
echo "固定端口: 8888"
echo "回调地址: http://localhost:8888/oauth-callback"
echo ""

read -p "是否继续修复? (y/n): " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "已取消"
    exit 0
fi

echo ""
echo "【步骤 2/3】Google Cloud Console 配置"
echo "----------------------------------------"
echo "请按以下步骤操作："
echo ""
echo "1. 打开浏览器访问："
echo "   https://console.cloud.google.com/apis/credentials"
echo ""
echo "2. 找到客户端ID: 1071006060591-tmhssin2h21lcre235vtolojh4g403ep"
echo ""
echo "3. 点击编辑，在 '已获授权的重定向 URI' 中添加："
echo "   http://localhost:8888/oauth-callback"
echo ""
echo "4. 点击'保存'"
echo ""
echo "5. 等待1-2分钟让配置生效"
echo ""

read -p "已完成Google Cloud Console配置? (y/n): " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo ""
    echo "请完成配置后重新运行此脚本"
    exit 0
fi

echo ""
echo "【步骤 3/3】启动OAuth授权流程"
echo "----------------------------------------"
echo "即将使用固定端口 8888 启动OAuth服务器..."
echo ""

sleep 2

# 启动OAuth服务器
node scripts/oauth-server.js --callback-port 8888

echo ""
echo "======================================"
echo "修复完成!"
echo "======================================"
echo ""
echo "如果仍然遇到问题，请查看 OAUTH_SETUP.md 获取详细指南"
echo ""

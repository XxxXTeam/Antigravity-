#!/bin/bash
# 测试API Key验证

echo "=== 测试 API Key 认证 ==="
echo ""

# 测试1: 无API Key
echo "测试1: 无 API Key（应该失败）"
curl -s -X POST http://localhost:8045/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "gemini-2.0-flash-exp", "messages": [{"role": "user", "content": "test"}]}' | jq .
echo ""
echo "---"
echo ""

# 测试2: 错误的API Key
echo "测试2: 错误的 API Key（应该失败）"
curl -s -X POST http://localhost:8045/v1/chat/completions \
  -H "Authorization: Bearer wrong-key" \
  -H "Content-Type: application/json" \
  -d '{"model": "gemini-2.0-flash-exp", "messages": [{"role": "user", "content": "test"}]}' | jq .
echo ""
echo "---"
echo ""

# 测试3: 配置文件中的API Key (sk-123)
echo "测试3: 配置文件的 API Key 'sk-123'（应该成功）"
curl -s -X POST http://localhost:8045/v1/chat/completions \
  -H "Authorization: Bearer sk-123" \
  -H "Content-Type: application/json" \
  -d '{"model": "gemini-2.0-flash-exp", "messages": [{"role": "user", "content": "Hello"}]}' | jq .
echo ""
echo "---"
echo ""

echo "测试完成！请检查服务器日志以查看详细信息。"

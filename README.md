# Antigravity to OpenAI API 代理服务

将 Google Antigravity API 转换为 OpenAI 兼容格式的代理服务，支持流式响应、工具调用和多账号管理。

本项目提供两种实现：
- **Node.js 版本** (`js/` 目录)：轻量级，适合快速部署
- **Go 版本** (`go/` 目录)：高性能，适合生产环境

## 目录

- [功能特性](#功能特性)
- [环境要求](#环境要求)
- [快速开始](#快速开始)
- [API 使用](#api-使用)
- [多账号管理](#多账号管理)
- [配置说明](#配置说明)
- [开发命令](#开发命令)
- [项目结构](#项目结构)
- [部署教程](#部署教程)
- [故障排除](#故障排除)
- [致谢](#致谢)
- [注意事项](#注意事项)
- [License](#license)

## 功能特性

- ✅ OpenAI API 兼容格式
- ✅ 流式和非流式响应
- ✅ 工具调用（Function Calling）支持
- ✅ 多账号自动轮换
- ✅ Token 自动刷新
- ✅ API Key 认证
- ✅ 思维链（Thinking）输出
- ✅ 图片输入支持（Base64 编码）

## 环境要求

### Node.js 版本
- Node.js >= 18.0.0

### Go 版本
- Go >= 1.24.0

## 快速开始

### Node.js 版本

### 1. 安装依赖

```bash
cd js
npm install
```

### 2. 配置文件

编辑 `config.json` 配置服务器和 API 参数：

```json
{
  "server": {
    "port": 8045,
    "host": "0.0.0.0"
  },
  "security": {
    "apiKey": "sk-text"
  }
}
```


### 4. 启动服务

```bash
cd js
npm start
```

服务将在 `http://localhost:8045` 启动。

### Go 版本

#### 1. 安装依赖

```bash
cd go
go mod download
```

#### 2. 配置文件

编辑 `config.yaml` 配置服务器和 API 参数。

#### 3. 获取 Token

```bash
make login
```

#### 4. 构建并启动

```bash
# 构建
make build

# 运行
./bin/antigravity-proxy
```

服务将在配置的端口启动（默认 8045）。

## API 使用

### 获取模型列表

```bash
curl http://localhost:8045/v1/models \
  -H "Authorization: Bearer sk-text"
```

### 聊天补全（流式）

```bash
curl http://localhost:8045/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-text" \
  -d '{
    "model": "gemini-2.0-flash-exp",
    "messages": [{"role": "user", "content": "你好"}],
    "stream": true
  }'
```

### 聊天补全（非流式）

```bash
curl http://localhost:8045/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-text" \
  -d '{
    "model": "gemini-2.0-flash-exp",
    "messages": [{"role": "user", "content": "你好"}],
    "stream": false
  }'
```

### 工具调用示例

```bash
curl http://localhost:8045/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-text" \
  -d '{
    "model": "gemini-2.0-flash-exp",
    "messages": [{"role": "user", "content": "北京天气怎么样"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "获取天气信息",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {"type": "string", "description": "城市名称"}
          }
        }
      }
    }]
  }'
```

### 图片输入示例

支持 Base64 编码的图片输入，兼容 OpenAI 的多模态格式：

```bash
curl http://localhost:8045/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-text" \
  -d '{
    "model": "gemini-2.0-flash-exp",
    "messages": [{
      "role": "user",
      "content": [
        {"type": "text", "text": "这张图片里有什么？"},
        {
          "type": "image_url",
          "image_url": {
            "url": "data:image/jpeg;base64,/9j/4AAQSkZJRg..."
          }
        }
      ]
    }],
    "stream": true
  }'
```

支持的图片格式：
- JPEG/JPG (`data:image/jpeg;base64,...`)
- PNG (`data:image/png;base64,...`)
- GIF (`data:image/gif;base64,...`)
- WebP (`data:image/webp;base64,...`)

## 多账号管理

`data/accounts.json` 支持多个账号，服务会自动轮换使用：

```json
[
  {
    "access_token": "ya29.xxx",
    "refresh_token": "1//xxx",
    "expires_in": 3599,
    "timestamp": 1234567890000,
    "enable": true
  },
  {
    "access_token": "ya29.yyy",
    "refresh_token": "1//yyy",
    "expires_in": 3599,
    "timestamp": 1234567890000,
    "enable": true
  }
]
```

- `enable: false` 可禁用某个账号
- Token 过期会自动刷新
- 刷新失败（403）会自动禁用并切换下一个账号

## 配置说明

### config.json

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `server.port` | 服务端口 | 8045 |
| `server.host` | 监听地址 | 0.0.0.0 |
| `security.apiKey` | API 认证密钥 | sk-text |
| `security.maxRequestSize` | 最大请求体大小 | 50mb |
| `defaults.temperature` | 默认温度参数 | 1 |
| `defaults.top_p` | 默认 top_p | 0.85 |
| `defaults.top_k` | 默认 top_k | 50 |
| `defaults.max_tokens` | 默认最大 token 数 | 8096 |
| `systemInstruction` | 系统提示词 | - |

## 开发命令

```bash
# 启动服务
npm start

# 开发模式（自动重启）
npm run dev

# 登录获取 Token
npm run login
```

## 项目结构

```
.
├── data/
│   └── accounts.json       # Token 存储（自动生成）
├── scripts/
│   └── oauth-server.js     # OAuth 登录服务
├── src/
│   ├── api/
│   │   └── client.js       # API 调用逻辑
│   ├── auth/
│   │   └── token_manager.js # Token 管理
│   ├── config/
│   │   └── config.js       # 配置加载
│   ├── server/
│   │   └── index.js        # 主服务器
│   └── utils/
│       ├── logger.js       # 日志模块
│       └── utils.js        # 工具函数
├── config.json             # 配置文件
└── package.json            # 项目配置
```

## 注意事项

1. 首次使用需要运行 `npm run login` (Node.js) 或 `make login` (Go) 获取 Token
2. `data/accounts.json` 包含敏感信息，请勿泄露
3. API Key 可在配置文件中自定义
4. 支持多账号轮换，提高可用性
5. Token 会自动刷新，无需手动维护

## 部署教程

### 使用 Docker 部署（推荐）

#### Node.js 版本

创建 `Dockerfile`：

```dockerfile
FROM node:18-alpine

WORKDIR /app

COPY js/package*.json ./
RUN npm install --production

COPY js/ .

EXPOSE 8045

CMD ["npm", "start"]
```

构建并运行：

```bash
# 构建镜像
docker build -t antigravity-proxy:latest .

# 运行容器
docker run -d \
  --name antigravity-proxy \
  -p 8045:8045 \
  -v $(pwd)/js/config.json:/app/config.json \
  -v $(pwd)/js/data:/app/data \
  antigravity-proxy:latest
```

#### Go 版本

创建 `Dockerfile`：

```dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go/ .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o antigravity-proxy ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/antigravity-proxy .

EXPOSE 8045

CMD ["./antigravity-proxy"]
```

构建并运行：

```bash
# 构建镜像
docker build -t antigravity-proxy-go:latest -f Dockerfile.go .

# 运行容器
docker run -d \
  --name antigravity-proxy-go \
  -p 8045:8045 \
  -v $(pwd)/go/config.yaml:/root/config.yaml \
  -v $(pwd)/go/data:/root/data \
  antigravity-proxy-go:latest
```

### 使用 Docker Compose 部署

创建 `docker-compose.yml`：

```yaml
version: '3.8'

services:
  antigravity-proxy:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8045:8045"
    volumes:
      - ./js/config.json:/app/config.json
      - ./js/data:/app/data
    restart: unless-stopped
    environment:
      - NODE_ENV=production
```

启动服务：

```bash
docker-compose up -d
```

### 使用 PM2 部署（Node.js 版本）

安装 PM2：

```bash
npm install -g pm2
```

创建 `ecosystem.config.js`：

```javascript
module.exports = {
  apps: [{
    name: 'antigravity-proxy',
    script: './src/server/index.js',
    cwd: './js',
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '1G',
    env: {
      NODE_ENV: 'production'
    }
  }]
}
```

启动服务：

```bash
# 启动
pm2 start ecosystem.config.js

# 查看状态
pm2 status

# 查看日志
pm2 logs antigravity-proxy

# 停止服务
pm2 stop antigravity-proxy

# 设置开机自启
pm2 startup
pm2 save
```

### 使用 Systemd 部署（Linux）

#### Node.js 版本

创建服务文件 `/etc/systemd/system/antigravity-proxy.service`：

```ini
[Unit]
Description=Antigravity to OpenAI API Proxy
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/Antigravity-/js
ExecStart=/usr/bin/node /path/to/Antigravity-/js/src/server/index.js
Restart=always
RestartSec=10
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=antigravity-proxy

[Install]
WantedBy=multi-user.target
```

#### Go 版本

创建服务文件 `/etc/systemd/system/antigravity-proxy.service`：

```ini
[Unit]
Description=Antigravity to OpenAI API Proxy (Go)
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/Antigravity-/go
ExecStart=/path/to/Antigravity-/go/bin/antigravity-proxy
Restart=always
RestartSec=10
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=antigravity-proxy

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
# 重载配置
sudo systemctl daemon-reload

# 启动服务
sudo systemctl start antigravity-proxy

# 设置开机自启
sudo systemctl enable antigravity-proxy

# 查看状态
sudo systemctl status antigravity-proxy

# 查看日志
sudo journalctl -u antigravity-proxy -f
```

### 使用反向代理（Nginx）

如果需要配置 HTTPS 或域名访问，可以使用 Nginx 作为反向代理：

```nginx
server {
    listen 80;
    server_name your-domain.com;

    # 重定向到 HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    # SSL 证书配置
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8045;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 流式响应支持
        proxy_buffering off;
        proxy_read_timeout 300s;
    }
}
```

### 云平台部署

#### Railway

1. Fork 本仓库
2. 在 [Railway](https://railway.app) 创建新项目
3. 连接你的 GitHub 仓库
4. 设置启动命令：`cd js && npm install && npm start`
5. 配置环境变量和端口
6. 部署完成

#### Heroku

1. 安装 Heroku CLI
2. 创建应用：
```bash
heroku create your-app-name
```

3. 推送代码：
```bash
git subtree push --prefix js heroku main
```

4. 配置环境变量：
```bash
heroku config:set NODE_ENV=production
```

#### Vercel / Netlify

这两个平台主要用于静态网站和 Serverless Functions，不太适合本项目的长连接需求。建议使用 Docker 或 VPS 部署。

## 故障排除

### Token 刷新失败

如果遇到 Token 刷新失败（403 错误）：

1. 检查 `refresh_token` 是否有效
2. 重新运行 `npm run login` 或 `make login` 获取新 Token
3. 确保账号状态正常，未被封禁

### 端口占用

如果端口已被占用：

1. 修改 `config.json` 或 `config.yaml` 中的端口配置
2. 或者停止占用该端口的进程：
```bash
# Linux/Mac
lsof -ti:8045 | xargs kill -9

# Windows
netstat -ano | findstr :8045
taskkill /PID <PID> /F
```

### 请求超时

如果遇到请求超时：

1. 检查网络连接是否正常
2. 尝试切换到其他账号
3. 增加请求超时时间配置
4. 检查防火墙设置

### 内存占用过高

如果服务占用内存过高：

1. 使用 PM2 的 `max_memory_restart` 限制内存
2. 减少并发请求数量
3. 定期重启服务
4. 考虑使用 Go 版本（更低内存占用）

## 致谢

本项目受到以下项目的启发和参考：

- [antigravity2api-nodejs](https://github.com/liuw1535/antigravity2api-nodejs) - Node.js 实现的 Antigravity API 转换服务
- [Antigravity-](https://github.com/lwl2005/Antigravity-) - 原始 Antigravity 代理项目

感谢这些项目的开发者们的贡献和灵感！

## License

MIT

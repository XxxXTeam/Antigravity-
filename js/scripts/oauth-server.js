import http from 'http';
import https from 'https';
import { URL } from 'url';
import crypto from 'crypto';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import log from '../src/utils/logger.js';
import accountFileManager from '../src/auth/account_file_manager.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const CLIENT_ID = '1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com';
const CLIENT_SECRET = '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';
const STATE = crypto.randomUUID();

const SCOPES = [
  'https://www.googleapis.com/auth/cloud-platform',
  'https://www.googleapis.com/auth/userinfo.email',
  'https://www.googleapis.com/auth/userinfo.profile',
  'https://www.googleapis.com/auth/cclog',
  'https://www.googleapis.com/auth/experimentsandconfigs'
];

// 解析命令行参数
const args = process.argv.slice(2);
let callbackHost = null;
let callbackPort = null;

for (let i = 0; i < args.length; i++) {
  if (args[i] === '--callback-host' && args[i + 1]) {
    callbackHost = args[i + 1];
  } else if (args[i] === '--callback-port' && args[i + 1]) {
    callbackPort = parseInt(args[i + 1]);
  }
}

function generateAuthUrl(redirectUri) {
  const params = new URLSearchParams({
    access_type: 'offline',
    client_id: CLIENT_ID,
    prompt: 'consent',
    redirect_uri: redirectUri,
    response_type: 'code',
    scope: SCOPES.join(' '),
    state: STATE
  });
  return `https://accounts.google.com/o/oauth2/v2/auth?${params.toString()}`;
}

function exchangeCodeForToken(code, redirectUri) {
  return new Promise((resolve, reject) => {
    const postData = new URLSearchParams({
      code: code,
      client_id: CLIENT_ID,
      redirect_uri: redirectUri,
      grant_type: 'authorization_code'
    });

    if (CLIENT_SECRET) {
      postData.append('client_secret', CLIENT_SECRET);
    }

    const data = postData.toString();

    const options = {
      hostname: 'oauth2.googleapis.com',
      path: '/token',
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Content-Length': Buffer.byteLength(data)
      }
    };

    const req = https.request(options, (res) => {
      let body = '';
      res.on('data', chunk => body += chunk);
      res.on('end', () => {
        if (res.statusCode === 200) {
          resolve(JSON.parse(body));
        } else {
          reject(new Error(`HTTP ${res.statusCode}: ${body}`));
        }
      });
    });

    req.on('error', reject);
    req.write(data);
    req.end();
  });
}

// 获取账号信息
async function getAccountInfo(accessToken) {
  return new Promise((resolve, reject) => {
    const options = {
      hostname: 'www.googleapis.com',
      path: '/oauth2/v2/userinfo',
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${accessToken}`
      }
    };

    const req = https.request(options, (res) => {
      let body = '';
      res.on('data', chunk => body += chunk);
      res.on('end', () => {
        if (res.statusCode === 200) {
          const data = JSON.parse(body);
          resolve({
            email: data.email,
            name: data.name || data.email
          });
        } else {
          resolve({ email: 'unknown@example.com', name: 'Unknown' });
        }
      });
    });

    req.on('error', () => resolve({ email: 'unknown@example.com', name: 'Unknown' }));
    req.end();
  });
}

// 获取模型列表
async function fetchModels(accessToken) {
  try {
    const response = await fetch('https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:fetchAvailableModels', {
      method: 'POST',
      headers: {
        'Host': 'daily-cloudcode-pa.sandbox.googleapis.com',
        'User-Agent': 'antigravity/1.11.3 windows/amd64',
        'Authorization': `Bearer ${accessToken}`,
        'Content-Type': 'application/json',
        'Accept-Encoding': 'gzip'
      },
      body: JSON.stringify({})
    });

    if (response.ok) {
      const data = await response.json();
      const models = {};

      for (const modelId of Object.keys(data.models || {})) {
        models[modelId] = {
          id: modelId,
          object: 'model',
          owned_by: 'google'
        };
      }

      return models;
    }
  } catch (error) {
    log.warn('获取模型列表失败:', error.message);
  }

  return {};
}

const server = http.createServer(async (req, res) => {
  const port = server.address().port;

  // 优先使用传入的 callback host，否则使用请求的 host，最后才用 localhost
  let host;
  if (callbackHost) {
    host = callbackHost;
  } else {
    // 尝试从请求头中获取真实的访问地址
    const requestHost = req.headers['x-forwarded-host'] || req.headers.host;
    if (requestHost && !requestHost.includes('localhost') && !requestHost.includes('127.0.0.1')) {
      const protocol = req.headers['x-forwarded-proto'] || 'http';
      host = `${protocol}://${requestHost}`;
      log.info(`使用请求的实际地址: ${host}`);
    } else {
      host = `http://localhost:${port}`;
    }
  }
  const redirectUri = `${host}/oauth-callback`;

  const url = new URL(req.url, host);

  if (url.pathname === '/oauth-callback') {
    const code = url.searchParams.get('code');
    const error = url.searchParams.get('error');

    if (code) {
      log.info('收到授权码，正在交换 Token...');
      try {
        const tokenData = await exchangeCodeForToken(code, redirectUri);

        // 获取账号信息
        log.info('获取账号信息...');
        const accountInfo = await getAccountInfo(tokenData.access_token);

        // 获取模型列表
        log.info('获取模型列表...');
        const models = await fetchModels(tokenData.access_token);

        // 使用账号文件管理器创建账号
        await accountFileManager.initialize();
        const accountId = await accountFileManager.createAccount(accountInfo.email, {
          name: accountInfo.name,
          access_token: tokenData.access_token,
          refresh_token: tokenData.refresh_token,
          expires_in: tokenData.expires_in,
          timestamp: Date.now(),
          enable: true,
          models,
          lastRefresh: Date.now(),
          refreshStatus: 'success'
        });

        log.info(`Token 已保存: ${accountId}`);
        log.info(`账号: ${accountInfo.email}`);
        log.info(`模型数量: ${Object.keys(models).length}`);

        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        res.end(`
          <h1>授权成功！</h1>
          <p>账号: ${accountInfo.email}</p>
          <p>账号ID: ${accountId}</p>
          <p>模型数量: ${Object.keys(models).length}</p>
          <p>Token 已保存，可以关闭此页面。</p>
        `);

        setTimeout(() => server.close(), 1000);
      } catch (err) {
        log.error('Token 交换失败:', err.message);

        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        res.end(`<h1>Token 获取失败</h1><p>${err.message}</p>`);

        setTimeout(() => server.close(), 1000);
      }
    } else {
      log.error('授权失败:', error || '未收到授权码');
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end('<h1>授权失败</h1>');
      setTimeout(() => server.close(), 1000);
    }
  } else {
    res.writeHead(404);
    res.end('Not Found');
  }
});

// 监听指定端口或随机端口
const listenPort = callbackPort || 0;
server.listen(listenPort, () => {
  const port = server.address().port;
  const host = callbackHost || `http://localhost:${port}`;
  const redirectUri = `${host}/oauth-callback`;
  const authUrl = generateAuthUrl(redirectUri);

  log.info(`OAuth服务器运行在端口: ${port}`);
  if (callbackHost) {
    log.info(`外部访问地址: ${callbackHost}`);
    log.info(`回调地址: ${redirectUri}`);
  } else {
    log.info(`本地访问: http://localhost:${port}`);
    log.info(`回调地址: ${redirectUri}`);
  }
  log.info('请在浏览器中打开以下链接进行登录：');
  console.log(`\n${authUrl}\n`);
  log.info('等待授权回调...');
});

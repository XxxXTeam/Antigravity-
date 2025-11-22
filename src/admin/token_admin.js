import fs from 'fs/promises';
import AdmZip from 'adm-zip';
import path from 'path';
import { spawn } from 'child_process';
import https from 'https';
import logger from '../utils/logger.js';
import accountFileManager from '../auth/account_file_manager.js';

const CLIENT_ID = '1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com';
const CLIENT_SECRET = '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';

// 读取所有账号 (使用新的账号文件管理器)
export async function loadAccounts() {
  try {
    const accounts = await accountFileManager.loadAllAccounts();
    return accounts;
  } catch (error) {
    logger.error('加载账号失败:', error);
    return [];
  }
}

// 删除账号
export async function deleteAccount(accountId) {
  try {
    await accountFileManager.deleteAccount(accountId);
    logger.info(`账号 ${accountId} 已删除`);
    return true;
  } catch (error) {
    logger.error(`删除账号失败:`, error);
    throw error;
  }
}

// 启用/禁用账号
export async function toggleAccount(accountId, enable) {
  try {
    const account = await accountFileManager.readAccount(accountId);
    account.enable = enable;
    await accountFileManager.writeAccount(accountId, account);
    logger.info(`账号 ${accountId} 已${enable ? '启用' : '禁用'}`);
    return true;
  } catch (error) {
    logger.error(`切换账号状态失败:`, error);
    throw error;
  }
}

// 触发登录流程 (支持动态回调 host)
export async function triggerLogin(callbackHost, callbackPort = 8888) {
  return new Promise((resolve, reject) => {
    logger.info('启动登录流程...');
    logger.info(`回调地址: ${callbackHost || 'localhost'}`);

    const loginScript = path.join(process.cwd(), 'scripts', 'oauth-server.js');

    // 传递回调 host 和端口作为参数
    const args = [loginScript];
    if (callbackHost) {
      args.push('--callback-host', callbackHost);
    }
    // 使用固定端口以便配置防火墙和反向代理
    args.push('--callback-port', callbackPort.toString());

    const child = spawn('node', args, {
      stdio: 'pipe',
      shell: false // 改为 false 以提高安全性
    });

    let authUrl = '';
    let output = '';

    child.stdout.on('data', (data) => {
      const text = data.toString();
      output += text;

      // 提取授权 URL
      const urlMatch = text.match(/(https:\/\/accounts\.google\.com\/o\/oauth2\/v2\/auth\?[^\s]+)/);
      if (urlMatch) {
        authUrl = urlMatch[1];
      }

      logger.info(text.trim());
    });

    child.stderr.on('data', (data) => {
      logger.error(data.toString().trim());
    });

    child.on('close', (code) => {
      if (code === 0) {
        logger.info('登录流程完成');
        resolve({ success: true, authUrl, message: '登录成功' });
      } else {
        reject(new Error('登录流程失败'));
      }
    });

    // 5 秒后返回授权 URL，不等待完成
    setTimeout(() => {
      if (authUrl) {
        resolve({ success: true, authUrl, message: '请在浏览器中完成授权' });
      }
    }, 5000);

    child.on('error', (error) => {
      reject(error);
    });
  });
}

// 获取账号统计信息
export async function getAccountStats() {
  const accounts = await loadAccounts();
  return {
    total: accounts.length,
    enabled: accounts.filter(a => a.enable !== false).length,
    disabled: accounts.filter(a => a.enable === false).length
  };
}

// 获取 Google 账号信息
export async function getAccountName(accessToken) {
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
          resolve({ email: 'Unknown', name: 'Unknown' });
        }
      });
    });

    req.on('error', () => resolve({ email: 'Unknown', name: 'Unknown' }));
    req.end();
  });
}

// 从回调链接手动添加 Token
export async function addTokenFromCallback(callbackUrl) {
  try {
    // 解析回调链接
    const url = new URL(callbackUrl);
    const code = url.searchParams.get('code');

    if (!code) {
      throw new Error('回调链接中没有找到授权码 (code)');
    }

    logger.info(`正在使用授权码换取 Token...`);

    // 使用授权码换取 Token
    const tokenData = await exchangeCodeForToken(code, url.origin);

    // 获取账号信息
    logger.info('获取账号信息...');
    const accountInfo = await getAccountName(tokenData.access_token);

    // 获取模型列表
    logger.info('获取模型列表...');
    const models = await fetchModels(tokenData.access_token);

    // 创建账号文件
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

    logger.info(`Token 已成功保存: ${accountId}`);
    return { success: true, message: 'Token 已成功添加', accountId };
  } catch (error) {
    logger.error('添加 Token 失败:', error);
    throw error;
  }
}

// 交换授权码为 Token
function exchangeCodeForToken(code, origin) {
  return new Promise((resolve, reject) => {
    const redirectUri = `${origin}/oauth-callback`;

    const postData = new URLSearchParams({
      code: code,
      client_id: CLIENT_ID,
      client_secret: CLIENT_SECRET,
      redirect_uri: redirectUri,
      grant_type: 'authorization_code'
    }).toString();

    const options = {
      hostname: 'oauth2.googleapis.com',
      path: '/token',
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Content-Length': Buffer.byteLength(postData)
      }
    };

    const req = https.request(options, (res) => {
      let body = '';
      res.on('data', chunk => body += chunk);
      res.on('end', () => {
        if (res.statusCode === 200) {
          resolve(JSON.parse(body));
        } else {
          logger.error(`Token 交换失败: ${body}`);
          reject(new Error(`Token 交换失败: ${res.statusCode} - ${body}`));
        }
      });
    });

    req.on('error', reject);
    req.write(postData);
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
    logger.warn('获取模型列表失败:', error.message);
  }

  return {};
}

// 批量导入 Token
export async function importTokens(filePath) {
  try {
    logger.info('开始导入 Token...');

    // 检查是否是 ZIP 文件
    if (filePath.endsWith('.zip') || true) {
      const zip = new AdmZip(filePath);
      const zipEntries = zip.getEntries();

      // 查找 tokens.json
      const tokensEntry = zipEntries.find(entry => entry.entryName === 'tokens.json');
      if (!tokensEntry) {
        throw new Error('ZIP 文件中没有找到 tokens.json');
      }

      const tokensContent = tokensEntry.getData().toString('utf8');
      const importedTokens = JSON.parse(tokensContent);

      // 验证数据格式
      if (!Array.isArray(importedTokens)) {
        throw new Error('tokens.json 格式错误：应该是一个数组');
      }

      // 添加新账号
      let addedCount = 0;
      for (const token of importedTokens) {
        try {
          // 检查是否已存在 (通过 email)
          const existingAccounts = await accountFileManager.loadAllAccounts();
          const exists = existingAccounts.some(acc => acc.email === token.email);

          if (!exists) {
            await accountFileManager.createAccount(token.email, {
              name: token.name || token.email,
              access_token: token.access_token,
              refresh_token: token.refresh_token,
              expires_in: token.expires_in,
              timestamp: token.timestamp || Date.now(),
              enable: token.enable !== false,
              models: token.models || {},
              lastRefresh: token.timestamp || Date.now(),
              refreshStatus: 'success'
            });
            addedCount++;
          }
        } catch (error) {
          logger.warn(`导入账号 ${token.email} 失败:`, error.message);
        }
      }

      // 清理上传的文件
      try {
        await fs.unlink(filePath);
      } catch (e) {
        logger.warn('清理上传文件失败:', e);
      }

      logger.info(`成功导入 ${addedCount} 个 Token 账号`);
      return {
        success: true,
        count: addedCount,
        total: importedTokens.length,
        skipped: importedTokens.length - addedCount,
        message: `成功导入 ${addedCount} 个 Token 账号${importedTokens.length - addedCount > 0 ? `，跳过 ${importedTokens.length - addedCount} 个重复账号` : ''}`
      };
    }
  } catch (error) {
    logger.error('导入 Token 失败:', error);
    // 清理上传的文件
    try {
      await fs.unlink(filePath);
    } catch (e) { }
    throw error;
  }
}

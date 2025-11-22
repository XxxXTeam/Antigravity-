import express from 'express';
import multer from 'multer';
import archiver from 'archiver';
import { createKey, loadKeys, deleteKey, updateKeyRateLimit, getKeyStats } from './key_manager.js';
import { getRecentLogs, clearLogs, addLog } from './log_manager.js';
import { getSystemStatus, incrementRequestCount } from './monitor.js';
import { loadAccounts, deleteAccount, toggleAccount, triggerLogin, getAccountStats, addTokenFromCallback, getAccountName, importTokens } from './token_admin.js';
import { createSession, validateSession, destroySession, verifyPassword, adminAuth } from './session.js';
import { loadSettings, saveSettings } from './settings_manager.js';
import tokenManager from '../auth/token_manager.js';
import accountFileManager from '../auth/account_file_manager.js';
import usageTracker from '../utils/usage_tracker.js';
import { getAccountModels } from '../api/client.js';
import logger from '../utils/logger.js';

// 配置文件上传
const upload = multer({ dest: 'uploads/' });

const router = express.Router();

// 登录接口（不需要认证）
router.post('/login', async (req, res) => {
  try {
    const { password } = req.body;
    if (!password) {
      return res.status(400).json({ error: '请输入密码' });
    }

    if (verifyPassword(password)) {
      const token = createSession();
      await addLog('info', '管理员登录成功');
      res.json({ success: true, token });
    } else {
      await addLog('warn', '管理员登录失败：密码错误');
      res.status(401).json({ error: '密码错误' });
    }
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 登出接口
router.post('/logout', (req, res) => {
  const token = req.headers['x-admin-token'];
  if (token) {
    destroySession(token);
  }
  res.json({ success: true });
});

// 验证会话接口
router.get('/verify', (req, res) => {
  const token = req.headers['x-admin-token'];
  if (validateSession(token)) {
    res.json({ valid: true });
  } else {
    res.status(401).json({ valid: false });
  }
});

// 以下所有路由需要认证
router.use(adminAuth);

// 生成新密钥
router.post('/keys/generate', async (req, res) => {
  try {
    const { name, rateLimit } = req.body;
    const newKey = await createKey(name, rateLimit);
    await addLog('success', `密钥已生成: ${name || '未命名'}`);
    res.json({ success: true, key: newKey.key, name: newKey.name, rateLimit: newKey.rateLimit });
  } catch (error) {
    await addLog('error', `生成密钥失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 获取所有密钥
router.get('/keys', async (req, res) => {
  try {
    const keys = await loadKeys();
    // 返回密钥列表（隐藏部分字符）
    const safeKeys = keys.map(k => ({
      ...k,
      key: k.key.substring(0, 10) + '...' + k.key.substring(k.key.length - 4)
    }));
    res.json(keys); // 在管理界面显示完整密钥
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 删除密钥
router.delete('/keys/:key', async (req, res) => {
  try {
    const { key } = req.params;
    await deleteKey(key);
    await addLog('warn', `密钥已删除: ${key.substring(0, 10)}...`);
    res.json({ success: true });
  } catch (error) {
    await addLog('error', `删除密钥失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 更新密钥频率限制
router.patch('/keys/:key/ratelimit', async (req, res) => {
  try {
    const { key } = req.params;
    const { rateLimit } = req.body;
    await updateKeyRateLimit(key, rateLimit);
    await addLog('info', `密钥频率限制已更新: ${key.substring(0, 10)}...`);
    res.json({ success: true });
  } catch (error) {
    await addLog('error', `更新频率限制失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 获取密钥统计
router.get('/keys/stats', async (req, res) => {
  try {
    const stats = await getKeyStats();
    res.json(stats);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 获取日志
router.get('/logs', async (req, res) => {
  try {
    const limit = parseInt(req.query.limit) || 100;
    const logs = await getRecentLogs(limit);
    res.json(logs);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 清空日志
router.delete('/logs', async (req, res) => {
  try {
    await clearLogs();
    await addLog('info', '日志已清空');
    res.json({ success: true });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 获取系统状态
router.get('/status', async (req, res) => {
  try {
    const status = getSystemStatus();
    res.json(status);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// Token 管理路由

// 获取所有账号
router.get('/tokens', async (req, res) => {
  try {
    const accounts = await loadAccounts();
    // 隐藏敏感信息，只返回必要字段
    const safeAccounts = accounts
      .filter(acc => {
        if (!acc.accountId) {
          logger.warn(`账号缺少 accountId: ${acc.email}`);
          return false;
        }
        return true;
      })
      .map((acc) => ({
        accountId: acc.accountId,
        email: acc.email,
        name: acc.name,
        access_token: acc.access_token?.substring(0, 20) + '...',
        refresh_token: acc.refresh_token ? 'exists' : 'none',
        expires_in: acc.expires_in,
        timestamp: acc.timestamp,
        enable: acc.enable !== false,
        created: new Date(acc.timestamp).toLocaleString(),
        modelCount: Object.keys(acc.models || {}).length,
        lastRefresh: acc.lastRefresh,
        refreshStatus: acc.refreshStatus,
        usage: acc.usage,
        errorTracking: acc.errorTracking
      }));
    res.json(safeAccounts);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 删除账号
router.delete('/tokens/:accountId', async (req, res) => {
  try {
    const { accountId } = req.params;
    await deleteAccount(accountId);
    await addLog('warn', `Token 账号 ${accountId} 已删除`);
    res.json({ success: true });
  } catch (error) {
    await addLog('error', `删除 Token 失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 启用/禁用账号
router.patch('/tokens/:accountId', async (req, res) => {
  try {
    const { accountId } = req.params;
    const { enable } = req.body;
    await toggleAccount(accountId, enable);
    await addLog('info', `Token 账号 ${accountId} 已${enable ? '启用' : '禁用'}`);
    res.json({ success: true });
  } catch (error) {
    await addLog('error', `切换 Token 状态失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 触发登录流程
router.post('/tokens/login', async (req, res) => {
  try {
    // 智能检测真实的外部访问地址
    // 1. 优先使用前端传递的 host（如果有）
    let callbackHost = req.body?.callbackHost;
    
    if (!callbackHost) {
      // 2. 尝试从代理头获取真实地址
      const forwardedProto = req.headers['x-forwarded-proto'] || req.protocol || 'http';
      const forwardedHost = req.headers['x-forwarded-host'] || req.headers['x-real-ip'];
      const host = forwardedHost || req.get('host');
      
      // 3. 构建完整的回调地址
      // 移除端口部分的 /admin 路径（如果存在）
      const cleanHost = host.replace(/\/.*$/, '');
      callbackHost = `${forwardedProto}://${cleanHost}`;
      
      // 如果是本地地址，添加OAuth回调端口
      if (cleanHost.includes('localhost') || cleanHost.includes('127.0.0.1')) {
        callbackHost = `${forwardedProto}://${cleanHost.split(':')[0]}:8888`;
      }
    }

    await addLog('info', `开始 Google OAuth 登录流程`);
    await addLog('info', `外部访问地址: ${callbackHost}`);
    await addLog('info', `OAuth回调地址: ${callbackHost}/oauth-callback`);
    
    const result = await triggerLogin(callbackHost);
    res.json(result);
  } catch (error) {
    await addLog('error', `登录失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 获取 Token 统计
router.get('/tokens/stats', async (req, res) => {
  try {
    const stats = await getAccountStats();
    res.json(stats);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 获取 Token 使用统计（轮询信息）
router.get('/tokens/usage', async (req, res) => {
  try {
    const usageStats = tokenManager.getUsageStats();
    res.json(usageStats);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 手动添加 Token（通过回调链接）
router.post('/tokens/callback', async (req, res) => {
  try {
    const { callbackUrl } = req.body;
    if (!callbackUrl) {
      return res.status(400).json({ error: '请提供回调链接' });
    }
    await addLog('info', '正在通过回调链接添加 Token...');
    const result = await addTokenFromCallback(callbackUrl);
    await addLog('success', 'Token 已通过回调链接成功添加');
    res.json(result);
  } catch (error) {
    await addLog('error', `添加 Token 失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 获取账号详细信息（包括名称）
router.post('/tokens/details', async (req, res) => {
  try {
    const { accountIds } = req.body;
    
    // 验证 accountIds，如果为空则返回空数组
    if (!accountIds || !Array.isArray(accountIds)) {
      logger.warn('收到无效的 accountIds 参数:', accountIds);
      return res.json([]);
    }
    
    const details = [];

    for (const accountId of accountIds) {
      try {
        const account = await accountFileManager.readAccount(accountId);
        details.push({
          accountId: account.accountId,
          email: account.email,
          name: account.name,
          access_token: account.access_token,
          refresh_token: account.refresh_token,
          expires_in: account.expires_in,
          timestamp: account.timestamp,
          enable: account.enable !== false,
          models: account.models,
          usage: account.usage,
          errorTracking: account.errorTracking,
          lastRefresh: account.lastRefresh,
          refreshStatus: account.refreshStatus
        });
      } catch (error) {
        logger.warn(`获取账号 ${accountId} 详情失败:`, error.message);
      }
    }

    res.json(details);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 批量导出 Token (ZIP格式)
router.post('/tokens/export', async (req, res) => {
  try {
    const { accountIds } = req.body;
    const exportData = [];

    for (const accountId of accountIds) {
      try {
        const account = await accountFileManager.readAccount(accountId);
        exportData.push({
          accountId: account.accountId,
          email: account.email,
          name: account.name,
          access_token: account.access_token,
          refresh_token: account.refresh_token,
          expires_in: account.expires_in,
          timestamp: account.timestamp,
          created: new Date(account.timestamp).toLocaleString(),
          enable: account.enable !== false,
          models: account.models,
          usage: account.usage
        });
      } catch (error) {
        logger.warn(`导出账号 ${accountId} 失败:`, error.message);
      }
    }

    await addLog('info', `批量导出了 ${exportData.length} 个 Token 账号`);

    // 创建 ZIP 文件
    const archive = archiver('zip', { zlib: { level: 9 } });
    const timestamp = new Date().toISOString().split('T')[0];

    res.attachment(`tokens_export_${timestamp}.zip`);
    res.setHeader('Content-Type', 'application/zip');

    archive.pipe(res);

    // 添加 tokens.json 文件到 ZIP
    archive.append(JSON.stringify(exportData, null, 2), { name: 'tokens.json' });

    await archive.finalize();
  } catch (error) {
    await addLog('error', `批量导出失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 批量导入 Token (ZIP格式)
router.post('/tokens/import', upload.single('file'), async (req, res) => {
  try {
    if (!req.file) {
      return res.status(400).json({ error: '请上传文件' });
    }

    await addLog('info', '正在导入 Token 账号...');
    const result = await importTokens(req.file.path);
    await addLog('success', `成功导入 ${result.count} 个 Token 账号`);
    res.json(result);
  } catch (error) {
    await addLog('error', `导入失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 并发刷新所有 Token
router.post('/tokens/refresh-all', async (req, res) => {
  try {
    await addLog('info', '开始并发刷新所有 Token...');
    const results = await tokenManager.refreshAllTokens();
    const successCount = results.filter(r => r.success).length;
    const failCount = results.filter(r => !r.success).length;
    await addLog('success', `并发刷新完成: 成功 ${successCount}, 失败 ${failCount}`);
    res.json({ success: true, results, successCount, failCount });
  } catch (error) {
    await addLog('error', `并发刷新失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 获取账号模型列表
router.get('/tokens/models/:accountId', async (req, res) => {
  try {
    const { accountId } = req.params;
    const models = await getAccountModels(accountId);
    res.json(models);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 使用统计路由

// 获取总体使用统计
router.get('/usage/summary', async (req, res) => {
  try {
    const summary = await usageTracker.getSummary();
    res.json(summary);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 获取特定账号的使用统计
router.get('/usage/account/:accountId', async (req, res) => {
  try {
    const { accountId } = req.params;
    const usage = await usageTracker.getAccountUsage(accountId);
    res.json(usage);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 获取使用历史
router.get('/usage/history', async (req, res) => {
  try {
    const filters = {
      accountId: req.query.accountId,
      startTime: req.query.startTime ? parseInt(req.query.startTime) : undefined,
      endTime: req.query.endTime ? parseInt(req.query.endTime) : undefined,
      limit: req.query.limit ? parseInt(req.query.limit) : 100
    };
    const history = await usageTracker.getHistory(filters);
    res.json(history);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 获取时间段内的使用统计
router.get('/usage/period/:hours', async (req, res) => {
  try {
    const hours = parseInt(req.params.hours) || 24;
    const stats = await usageTracker.getUsageForPeriod(hours);
    res.json(stats);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 导出使用数据
router.get('/usage/export', async (req, res) => {
  try {
    const format = req.query.format || 'json';

    if (format === 'csv') {
      const csv = await usageTracker.exportCSV();
      res.setHeader('Content-Type', 'text/csv');
      res.setHeader('Content-Disposition', `attachment; filename="usage_export_${Date.now()}.csv"`);
      res.send(csv);
    } else {
      const json = await usageTracker.exportJSON();
      res.setHeader('Content-Type', 'application/json');
      res.setHeader('Content-Disposition', `attachment; filename="usage_export_${Date.now()}.json"`);
      res.send(json);
    }

    await addLog('info', `使用数据已导出 (${format})`);
  } catch (error) {
    await addLog('error', `导出使用数据失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 重置使用统计
router.post('/usage/reset', async (req, res) => {
  try {
    await usageTracker.reset();
    await addLog('warn', '使用统计已重置');
    res.json({ success: true, message: '使用统计已重置' });
  } catch (error) {
    await addLog('error', `重置使用统计失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

// 获取系统设置
router.get('/settings', async (req, res) => {
  try {
    const settings = await loadSettings();
    res.json(settings);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// 保存系统设置
router.post('/settings', async (req, res) => {
  try {
    const result = await saveSettings(req.body);
    await addLog('success', '系统设置已更新');
    res.json(result);
  } catch (error) {
    await addLog('error', `保存设置失败: ${error.message}`);
    res.status(500).json({ error: error.message });
  }
});

export default router;
export { incrementRequestCount, addLog };

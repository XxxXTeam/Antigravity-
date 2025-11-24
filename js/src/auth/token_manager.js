import { log } from '../utils/logger.js';
import accountFileManager from './account_file_manager.js';

const CLIENT_ID = '1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com';
const CLIENT_SECRET = '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';

class TokenManager {
  constructor() {
    this.accounts = [];
    this.currentIndex = 0;
    this.lastLoadTime = 0;
    this.loadInterval = 60000; // 1分钟内不重复加载
    this.refreshInterval = null;
    this.usageStats = new Map(); // Token 使用统计
    this.init();
  }

  async init() {
    await accountFileManager.initialize();
    await this.loadTokens();
    this.startBackgroundRefresh();
  }

  /**
   * Load tokens from account files
   */
  async loadTokens() {
    try {
      // 避免频繁加载，1分钟内使用缓存
      if (Date.now() - this.lastLoadTime < this.loadInterval && this.accounts.length > 0) {
        return;
      }

      log.info('正在加载账号...');
      const allAccounts = await accountFileManager.loadAllAccounts();

      // Filter enabled accounts not in cooldown
      this.accounts = allAccounts.filter(account => {
        if (account.enable === false) return false;
        if (accountFileManager.isInCooldown(account)) {
          log.warn(`账号 ${account.accountId} 处于冷却期，跳过`);
          return false;
        }
        return true;
      });

      this.currentIndex = 0;
      this.lastLoadTime = Date.now();
      log.info(`成功加载 ${this.accounts.length} 个可用账号`);

      // 触发垃圾回收（如果可用）
      if (global.gc) {
        global.gc();
      }
    } catch (error) {
      log.error('加载账号失败:', error.message);
      this.accounts = [];
    }
  }

  /**
   * Check if token is expired
   */
  isExpired(account) {
    if (!account.timestamp || !account.expires_in) return true;
    const expiresAt = account.timestamp + (account.expires_in * 1000);
    return Date.now() >= expiresAt - 300000; // 5 minutes buffer
  }

  /**
   * Refresh single token
   */
  async refreshToken(account) {
    log.info(`正在刷新账号: ${account.accountId}`);
    const body = new URLSearchParams({
      client_id: CLIENT_ID,
      client_secret: CLIENT_SECRET,
      grant_type: 'refresh_token',
      refresh_token: account.refresh_token
    });

    try {
      const response = await fetch('https://oauth2.googleapis.com/token', {
        method: 'POST',
        headers: {
          'Host': 'oauth2.googleapis.com',
          'User-Agent': 'Go-http-client/1.1',
          'Content-Length': body.toString().length.toString(),
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept-Encoding': 'gzip'
        },
        body: body.toString()
      });

      if (response.ok) {
        const data = await response.json();
        account.access_token = data.access_token;
        account.expires_in = data.expires_in;
        account.timestamp = Date.now();
        account.lastRefresh = Date.now();
        account.refreshStatus = 'success';

        // Fetch and update models
        await this.updateAccountModels(account);

        // Update error tracking - success
        await accountFileManager.updateErrorTracking(account.accountId, { success: true });

        // Save to file
        await accountFileManager.writeAccount(account.accountId, account);

        log.info(`账号 ${account.accountId} 刷新成功`);
        return account;
      } else {
        const errorText = await response.text();
        throw { statusCode: response.status, message: errorText };
      }
    } catch (error) {
      log.error(`账号 ${account.accountId} 刷新失败:`, error.message);

      // Update error tracking
      await accountFileManager.updateErrorTracking(account.accountId, {
        success: false,
        error: error.message
      });

      throw error;
    }
  }

  /**
   * Fetch and update models for account
   */
  async updateAccountModels(account) {
    try {
      const response = await fetch('https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:fetchAvailableModels', {
        method: 'POST',
        headers: {
          'Host': 'daily-cloudcode-pa.sandbox.googleapis.com',
          'User-Agent': 'antigravity/1.11.3 windows/amd64',
          'Authorization': `Bearer ${account.access_token}`,
          'Content-Type': 'application/json',
          'Accept-Encoding': 'gzip'
        },
        body: JSON.stringify({})
      });

      if (response.ok) {
        const data = await response.json();
        account.models = {};

        for (const modelId of Object.keys(data.models || {})) {
          account.models[modelId] = {
            id: modelId,
            object: 'model',
            owned_by: 'google'
          };
        }

        log.info(`账号 ${account.accountId} 模型列表已更新: ${Object.keys(account.models).length} 个模型`);
      }
    } catch (error) {
      log.warn(`获取账号 ${account.accountId} 模型列表失败:`, error.message);
    }
  }

  /**
   * Refresh all tokens concurrently
   */
  async refreshAllTokens() {
    log.info('开始并发刷新所有账号...');
    const allAccounts = await accountFileManager.loadAllAccounts();
    const enabledAccounts = allAccounts.filter(acc => acc.enable !== false);

    const refreshPromises = enabledAccounts.map(async (account) => {
      try {
        await this.refreshToken(account);
        return { accountId: account.accountId, success: true };
      } catch (error) {
        return { accountId: account.accountId, success: false, error: error.message };
      }
    });

    const results = await Promise.all(refreshPromises);

    const successCount = results.filter(r => r.success).length;
    const failCount = results.filter(r => !r.success).length;

    log.info(`并发刷新完成: 成功 ${successCount}, 失败 ${failCount}`);

    // Reload accounts after refresh
    await this.loadTokens();

    return results;
  }

  /**
   * Start background refresh scheduler
   */
  startBackgroundRefresh() {
    // Refresh every 30 minutes
    const interval = 30 * 60 * 1000;

    this.refreshInterval = setInterval(async () => {
      log.info('执行定时刷新任务...');
      await this.refreshAllTokens();
    }, interval);

    log.info('后台刷新调度器已启动 (每30分钟)');
  }

  /**
   * Stop background refresh scheduler
   */
  stopBackgroundRefresh() {
    if (this.refreshInterval) {
      clearInterval(this.refreshInterval);
      this.refreshInterval = null;
      log.info('后台刷新调度器已停止');
    }
  }

  /**
   * Get next available token with retry logic
   */
  async getToken() {
    if (this.accounts.length === 0) {
      await this.loadTokens();
      if (this.accounts.length === 0) {
        return null;
      }
    }

    const maxAttempts = this.accounts.length;

    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      const account = this.accounts[this.currentIndex];
      const accountIndex = this.currentIndex;

      try {
        // Check if expired and refresh if needed
        if (this.isExpired(account)) {
          await this.refreshToken(account);
        }

        // Move to next account for rotation
        this.currentIndex = (this.currentIndex + 1) % this.accounts.length;

        // Record usage
        this.recordUsage(account);
        log.info(`🔄 轮询使用账号 #${accountIndex} (${account.email}) (总请求: ${this.getTokenRequests(account)})`);

        return account;
      } catch (error) {
        log.error(`账号 ${accountIndex} (${account.accountId}) 获取失败:`, error.message);

        // Move to next account
        this.currentIndex = (this.currentIndex + 1) % this.accounts.length;

        // Reload accounts to exclude failed ones in cooldown
        if (attempt < maxAttempts - 1) {
          await this.loadTokens();
        }
      }
    }

    log.error('所有账号均不可用');
    return null;
  }

  /**
   * Get token with automatic retry on error
   * @param {number} maxRetries - Maximum number of retries
   */
  async getTokenWithRetry(maxRetries = 3) {
    for (let retry = 0; retry < maxRetries; retry++) {
      const token = await this.getToken();
      if (token) {
        return token;
      }

      if (retry < maxRetries - 1) {
        log.warn(`获取账号失败，重试 ${retry + 1}/${maxRetries - 1}`);
        await new Promise(resolve => setTimeout(resolve, 1000 * (retry + 1))); // Exponential backoff
      }
    }

    throw new Error('没有可用的账号，请添加账号或检查账号状态');
  }

  /**
   * Disable account
   */
  async disableAccount(account) {
    log.warn(`禁用账号: ${account.accountId}`);
    account.enable = false;
    await accountFileManager.writeAccount(account.accountId, account);
    await this.loadTokens();
  }

  /**
   * Disable current token (for compatibility)
   */
  async disableCurrentToken(token) {
    const found = this.accounts.find(acc => acc.access_token === token.access_token);
    if (found) {
      await this.disableAccount(found);
    }
  }

  /**
   * Record token usage
   */
  recordUsage(account) {
    const key = account.accountId;
    if (!this.usageStats.has(key)) {
      this.usageStats.set(key, { requests: 0, lastUsed: null });
    }
    const stats = this.usageStats.get(key);
    stats.requests++;
    stats.lastUsed = Date.now();
  }

  /**
   * Get token request count
   */
  getTokenRequests(account) {
    const stats = this.usageStats.get(account.accountId);
    return stats ? stats.requests : 0;
  }

  /**
   * Get usage statistics
   */
  getUsageStats() {
    const stats = [];
    this.accounts.forEach((account, index) => {
      const usage = this.usageStats.get(account.accountId) || { requests: 0, lastUsed: null };
      stats.push({
        index,
        accountId: account.accountId,
        email: account.email,
        requests: usage.requests,
        lastUsed: usage.lastUsed ? new Date(usage.lastUsed).toISOString() : null,
        isCurrent: index === this.currentIndex
      });
    });
    return {
      totalAccounts: this.accounts.length,
      currentIndex: this.currentIndex,
      totalRequests: Array.from(this.usageStats.values()).reduce((sum, s) => sum + s.requests, 0),
      accounts: stats
    };
  }

  /**
   * Cleanup resources
   */
  destroy() {
    this.stopBackgroundRefresh();
  }
}

const tokenManager = new TokenManager();
export default tokenManager;

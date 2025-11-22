import fs from 'fs/promises';
import path from 'path';
import crypto from 'crypto';
import logger from '../utils/logger.js';

const ACCOUNTS_DIR = path.join(process.cwd(), 'data', 'accounts');

/**
 * Account File Manager
 * Handles individual account file operations with unique filenames
 */
class AccountFileManager {
    constructor(accountsDirectory = ACCOUNTS_DIR) {
        this.accountsDir = accountsDirectory;
        this.fileLocks = new Map(); // Simple in-memory file locks
    }

    /**
     * Initialize accounts directory
     */
    async initialize() {
        try {
            await fs.access(this.accountsDir);
        } catch {
            await fs.mkdir(this.accountsDir, { recursive: true });
            logger.info(`创建账号目录: ${this.accountsDir}`);
        }
    }

    /**
     * Generate unique filename for account
     * Format: {email}_{randomString}.json
     * @param {string} email - Account email
     * @returns {string} Unique filename
     */
    generateFilename(email) {
        const randomString = crypto.randomBytes(4).toString('hex'); // 8 characters
        const sanitizedEmail = email.replace(/[^a-zA-Z0-9@._-]/g, '_');
        return `${sanitizedEmail}_${randomString}.json`;
    }

    /**
     * Generate account ID from filename
     * @param {string} filename - Account filename
     * @returns {string} Account ID (filename without .json)
     */
    getAccountId(filename) {
        return filename.replace('.json', '');
    }

    /**
     * Get full path for account file
     * @param {string} accountId - Account ID or filename
     * @returns {string} Full file path
     */
    getFilePath(accountId) {
        const filename = accountId.endsWith('.json') ? accountId : `${accountId}.json`;
        return path.join(this.accountsDir, filename);
    }

    /**
     * Acquire lock for file operation
     * @param {string} accountId - Account ID
     */
    async acquireLock(accountId) {
        while (this.fileLocks.get(accountId)) {
            await new Promise(resolve => setTimeout(resolve, 10));
        }
        this.fileLocks.set(accountId, true);
    }

    /**
     * Release lock for file operation
     * @param {string} accountId - Account ID
     */
    releaseLock(accountId) {
        this.fileLocks.delete(accountId);
    }

    /**
     * Read account file
     * @param {string} accountId - Account ID
     * @returns {Object} Account data
     */
    async readAccount(accountId) {
        await this.acquireLock(accountId);
        try {
            const filePath = this.getFilePath(accountId);
            const data = await fs.readFile(filePath, 'utf-8');
            return JSON.parse(data);
        } catch (error) {
            if (error.code === 'ENOENT') {
                throw new Error(`账号文件不存在: ${accountId}`);
            }
            throw error;
        } finally {
            this.releaseLock(accountId);
        }
    }

    /**
     * Write account file
     * @param {string} accountId - Account ID
     * @param {Object} accountData - Account data
     */
    async writeAccount(accountId, accountData) {
        await this.acquireLock(accountId);
        try {
            await this.initialize(); // Ensure directory exists
            const filePath = this.getFilePath(accountId);
            await fs.writeFile(filePath, JSON.stringify(accountData, null, 2), 'utf-8');
        } finally {
            this.releaseLock(accountId);
        }
    }

    /**
     * Create new account file
     * @param {string} email - Account email
     * @param {Object} accountData - Account data
     * @returns {string} Account ID
     */
    async createAccount(email, accountData) {
        const filename = this.generateFilename(email);
        const accountId = this.getAccountId(filename);

        const fullAccountData = {
            accountId,
            email,
            ...accountData,
            usage: {
                totalTokens: 0,
                inputTokens: 0,
                outputTokens: 0,
                requestCount: 0,
                lastUsed: null
            },
            errorTracking: {
                consecutiveFailures: 0,
                lastError: null,
                lastErrorTime: null,
                failedUntil: null
            }
        };

        await this.writeAccount(accountId, fullAccountData);
        logger.info(`创建账号文件: ${filename}`);
        return accountId;
    }

    /**
     * Delete account file
     * @param {string} accountId - Account ID
     */
    async deleteAccount(accountId) {
        await this.acquireLock(accountId);
        try {
            const filePath = this.getFilePath(accountId);
            await fs.unlink(filePath);
            logger.info(`删除账号文件: ${accountId}`);
        } finally {
            this.releaseLock(accountId);
        }
    }

    /**
     * List all account files
     * @returns {Array<string>} Array of account IDs
     */
    async listAccounts() {
        try {
            await this.initialize();
            const files = await fs.readdir(this.accountsDir);
            return files
                .filter(file => file.endsWith('.json'))
                .map(file => this.getAccountId(file));
        } catch (error) {
            logger.error('列出账号文件失败:', error);
            return [];
        }
    }

    /**
     * Load all accounts
     * @returns {Array<Object>} Array of account data
     */
    async loadAllAccounts() {
        const accountIds = await this.listAccounts();
        const accounts = [];

        for (const accountId of accountIds) {
            try {
                const account = await this.readAccount(accountId);
                
                // 为缺少 accountId 的旧账号文件添加 accountId
                if (!account.accountId) {
                    account.accountId = accountId;
                    await this.writeAccount(accountId, account);
                    logger.info(`为旧账号文件添加 accountId: ${accountId}`);
                }
                
                // 确保 usage 和 errorTracking 字段存在
                if (!account.usage) {
                    account.usage = {
                        totalTokens: 0,
                        inputTokens: 0,
                        outputTokens: 0,
                        requestCount: 0,
                        lastUsed: null
                    };
                }
                if (!account.errorTracking) {
                    account.errorTracking = {
                        consecutiveFailures: 0,
                        lastError: null,
                        lastErrorTime: null,
                        failedUntil: null
                    };
                }
                
                accounts.push(account);
            } catch (error) {
                logger.error(`加载账号失败 ${accountId}:`, error.message);
            }
        }

        return accounts;
    }

    /**
     * Validate account data structure
     * @param {Object} accountData - Account data to validate
     * @returns {boolean} True if valid
     */
    validateAccount(accountData) {
        const requiredFields = [
            'accountId',
            'email',
            'access_token',
            'refresh_token',
            'expires_in',
            'timestamp'
        ];

        for (const field of requiredFields) {
            if (!accountData[field]) {
                logger.warn(`账号数据缺少必需字段: ${field}`);
                return false;
            }
        }

        // Validate usage structure
        if (accountData.usage) {
            const usageFields = ['totalTokens', 'inputTokens', 'outputTokens', 'requestCount'];
            for (const field of usageFields) {
                if (typeof accountData.usage[field] !== 'number') {
                    logger.warn(`账号使用统计字段无效: ${field}`);
                    return false;
                }
            }
        }

        return true;
    }

    /**
     * Update account usage statistics
     * @param {string} accountId - Account ID
     * @param {Object} usage - Usage data {totalTokens, inputTokens, outputTokens}
     */
    async updateUsage(accountId, usage) {
        const account = await this.readAccount(accountId);

        if (!account.usage) {
            account.usage = {
                totalTokens: 0,
                inputTokens: 0,
                outputTokens: 0,
                requestCount: 0,
                lastUsed: null
            };
        }

        account.usage.totalTokens += usage.totalTokens || 0;
        account.usage.inputTokens += usage.inputTokens || 0;
        account.usage.outputTokens += usage.outputTokens || 0;
        account.usage.requestCount += 1;
        account.usage.lastUsed = Date.now();

        await this.writeAccount(accountId, account);
    }

    /**
     * Update account error tracking
     * @param {string} accountId - Account ID
     * @param {Object} errorInfo - Error information
     */
    async updateErrorTracking(accountId, errorInfo) {
        const account = await this.readAccount(accountId);

        if (!account.errorTracking) {
            account.errorTracking = {
                consecutiveFailures: 0,
                lastError: null,
                lastErrorTime: null,
                failedUntil: null
            };
        }

        if (errorInfo.success) {
            // Reset on success
            account.errorTracking.consecutiveFailures = 0;
            account.errorTracking.failedUntil = null;
        } else {
            // Increment failures
            account.errorTracking.consecutiveFailures += 1;
            account.errorTracking.lastError = errorInfo.error;
            account.errorTracking.lastErrorTime = Date.now();

            // Set cooldown period (5 minutes)
            account.errorTracking.failedUntil = Date.now() + (5 * 60 * 1000);

            // Auto-disable after 3 consecutive failures
            if (account.errorTracking.consecutiveFailures >= 3) {
                account.enable = false;
                logger.warn(`账号 ${accountId} 因连续失败3次已自动禁用`);
            }
        }

        await this.writeAccount(accountId, account);
    }

    /**
     * Check if account is in cooldown period
     * @param {Object} account - Account data
     * @returns {boolean} True if in cooldown
     */
    isInCooldown(account) {
        if (!account.errorTracking || !account.errorTracking.failedUntil) {
            return false;
        }
        return Date.now() < account.errorTracking.failedUntil;
    }
}

// Export singleton instance
const accountFileManager = new AccountFileManager();
export default accountFileManager;

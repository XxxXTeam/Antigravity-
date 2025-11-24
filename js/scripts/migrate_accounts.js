#!/usr/bin/env node

/**
 * Migration Script: Convert old accounts.json to new individual account files
 * 
 * Usage:
 *   node scripts/migrate_accounts.js           # Run migration
 *   node scripts/migrate_accounts.js --dry-run # Preview without making changes
 *   node scripts/migrate_accounts.js --rollback # Restore from backup
 */

import fs from 'fs/promises';
import path from 'path';
import https from 'https';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const OLD_ACCOUNTS_FILE = path.join(process.cwd(), 'data', 'accounts.json');
const BACKUP_FILE = path.join(process.cwd(), 'data', 'accounts.json.backup');
const NEW_ACCOUNTS_DIR = path.join(process.cwd(), 'data', 'accounts');

const isDryRun = process.argv.includes('--dry-run');
const isRollback = process.argv.includes('--rollback');

/**
 * Get account email from Google API
 */
async function getAccountEmail(accessToken) {
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
        req.setTimeout(5000, () => {
            req.destroy();
            resolve({ email: 'unknown@example.com', name: 'Unknown' });
        });
        req.end();
    });
}

/**
 * Generate unique filename
 */
async function generateFilename(email, index) {
    const crypto = await import('crypto');
    const randomString = crypto.randomBytes(4).toString('hex');
    const sanitizedEmail = email.replace(/[^a-zA-Z0-9@._-]/g, '_');
    return `${sanitizedEmail}_${randomString}.json`;
}

const CLIENT_ID = '1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com';
const CLIENT_SECRET = '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';

/**
 * Check if token is expired
 */
function isTokenExpired(account) {
    if (!account.timestamp || !account.expires_in) return true;
    const expiresAt = account.timestamp + (account.expires_in * 1000);
    return Date.now() >= expiresAt - 300000; // 5 minutes buffer
}

/**
 * Refresh token
 */
async function refreshToken(account) {
    return new Promise((resolve, reject) => {
        const postData = new URLSearchParams({
            client_id: CLIENT_ID,
            client_secret: CLIENT_SECRET,
            grant_type: 'refresh_token',
            refresh_token: account.refresh_token
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
                    const data = JSON.parse(body);
                    resolve({
                        access_token: data.access_token,
                        refresh_token: account.refresh_token,
                        expires_in: data.expires_in,
                        timestamp: Date.now()
                    });
                } else {
                    reject(new Error(`Token refresh failed: ${res.statusCode} - ${body}`));
                }
            });
        });

        req.on('error', reject);
        req.write(postData);
        req.end();
    });
}

/**
 * Fetch models for account
 */
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
        console.warn('获取模型列表失败:', error.message);
    }

    return {};
}

/**
 * Rollback migration
 */
async function rollback() {
    console.log('🔄 开始回滚迁移...\n');

    try {
        // Check if backup exists
        await fs.access(BACKUP_FILE);

        // Restore backup
        await fs.copyFile(BACKUP_FILE, OLD_ACCOUNTS_FILE);
        console.log('✅ 已从备份恢复 accounts.json');

        // Remove new accounts directory
        try {
            const files = await fs.readdir(NEW_ACCOUNTS_DIR);
            for (const file of files) {
                if (file.endsWith('.json')) {
                    await fs.unlink(path.join(NEW_ACCOUNTS_DIR, file));
                }
            }
            await fs.rmdir(NEW_ACCOUNTS_DIR);
            console.log('✅ 已删除新账号目录');
        } catch (error) {
            console.warn('⚠️  删除新账号目录失败:', error.message);
        }

        console.log('\n✅ 回滚完成！');
    } catch (error) {
        console.error('❌ 回滚失败:', error.message);
        process.exit(1);
    }
}

/**
 * Main migration function
 */
async function migrate() {
    console.log('🚀 开始迁移账号数据...\n');

    if (isDryRun) {
        console.log('⚠️  DRY RUN 模式 - 不会进行实际修改\n');
    }

    try {
        // 1. Read old accounts file
        console.log('📖 读取旧账号文件...');
        const oldData = await fs.readFile(OLD_ACCOUNTS_FILE, 'utf-8');
        const accounts = JSON.parse(oldData);
        console.log(`✅ 找到 ${accounts.length} 个账号\n`);

        // 2. Create backup
        if (!isDryRun) {
            console.log('💾 创建备份...');
            await fs.copyFile(OLD_ACCOUNTS_FILE, BACKUP_FILE);
            console.log(`✅ 备份已保存到: ${BACKUP_FILE}\n`);
        }

        // 3. Create new accounts directory
        if (!isDryRun) {
            try {
                await fs.mkdir(NEW_ACCOUNTS_DIR, { recursive: true });
                console.log(`✅ 创建账号目录: ${NEW_ACCOUNTS_DIR}\n`);
            } catch (error) {
                if (error.code !== 'EEXIST') throw error;
            }
        }

        // 4. Migrate each account
        console.log('🔄 开始迁移账号...\n');
        const results = [];

        for (let i = 0; i < accounts.length; i++) {
            let account = accounts[i];
            console.log(`[${i + 1}/${accounts.length}] 迁移账号...`);

            try {
                // Check if token is expired and refresh if needed
                if (isTokenExpired(account)) {
                    console.log('  🔄 Token已过期，正在刷新...');
                    try {
                        const refreshedToken = await refreshToken(account);
                        account = { ...account, ...refreshedToken };
                        console.log('  ✅ Token刷新成功');
                    } catch (error) {
                        console.warn(`  ⚠️  Token刷新失败: ${error.message}`);
                        console.log('  ℹ️  将使用现有Token继续（可能失败）');
                    }
                }

                // Fetch email
                console.log('  📧 获取邮箱地址...');
                const accountInfo = await getAccountEmail(account.access_token);
                console.log(`  ✅ 邮箱: ${accountInfo.email}`);

                // Fetch models
                console.log('  📦 获取模型列表...');
                const models = await fetchModels(account.access_token);
                console.log(`  ✅ 找到 ${Object.keys(models).length} 个模型`);

                // Generate filename
                const crypto = await import('crypto');
                const randomString = crypto.randomBytes(4).toString('hex');
                const sanitizedEmail = accountInfo.email.replace(/[^a-zA-Z0-9@._-]/g, '_');
                const filename = `${sanitizedEmail}_${randomString}.json`;
                const accountId = filename.replace('.json', '');

                // Create new account object
                const newAccount = {
                    accountId,
                    email: accountInfo.email,
                    name: accountInfo.name,
                    access_token: account.access_token,
                    refresh_token: account.refresh_token,
                    expires_in: account.expires_in,
                    timestamp: account.timestamp,
                    enable: account.enable !== false,
                    models,
                    lastRefresh: account.timestamp,
                    refreshStatus: 'success',
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

                // Write to file
                if (!isDryRun) {
                    const filePath = path.join(NEW_ACCOUNTS_DIR, filename);
                    await fs.writeFile(filePath, JSON.stringify(newAccount, null, 2), 'utf-8');
                    console.log(`  ✅ 已创建: ${filename}\n`);
                } else {
                    console.log(`  ℹ️  将创建: ${filename}\n`);
                }

                results.push({
                    success: true,
                    email: accountInfo.email,
                    filename
                });

            } catch (error) {
                console.error(`  ❌ 失败:`, error.message, '\n');
                results.push({
                    success: false,
                    error: error.message
                });
            }
        }

        // 5. Summary
        console.log('\n📊 迁移摘要:');
        console.log('─'.repeat(50));
        const successCount = results.filter(r => r.success).length;
        const failCount = results.filter(r => !r.success).length;
        console.log(`✅ 成功: ${successCount}`);
        console.log(`❌ 失败: ${failCount}`);
        console.log('─'.repeat(50));

        if (isDryRun) {
            console.log('\n⚠️  这是 DRY RUN，没有进行实际修改');
            console.log('   运行不带 --dry-run 参数以执行实际迁移');
        } else {
            console.log('\n✅ 迁移完成！');
            console.log(`\n💡 提示:`);
            console.log(`   - 旧文件备份: ${BACKUP_FILE}`);
            console.log(`   - 新账号目录: ${NEW_ACCOUNTS_DIR}`);
            console.log(`   - 如需回滚，运行: node scripts/migrate_accounts.js --rollback`);
        }

    } catch (error) {
        console.error('\n❌ 迁移失败:', error.message);
        console.error(error.stack);
        process.exit(1);
    }
}

// Run migration
if (isRollback) {
    rollback();
} else {
    migrate();
}

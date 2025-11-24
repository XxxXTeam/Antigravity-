import fs from 'fs/promises';
import path from 'path';
import logger from './logger.js';

const USAGE_FILE = path.join(process.cwd(), 'data', 'usage_history.json');

/**
 * Usage Tracker
 * Tracks token usage statistics across all accounts
 */
class UsageTracker {
    constructor() {
        this.history = [];
        this.loaded = false;
    }

    /**
     * Load usage history from file
     */
    async load() {
        try {
            const data = await fs.readFile(USAGE_FILE, 'utf-8');
            this.history = JSON.parse(data);
            this.loaded = true;
        } catch (error) {
            if (error.code !== 'ENOENT') {
                logger.error('加载使用统计失败:', error);
            }
            this.history = [];
            this.loaded = true;
        }
    }

    /**
     * Save usage history to file
     */
    async save() {
        try {
            const dir = path.dirname(USAGE_FILE);
            await fs.mkdir(dir, { recursive: true });
            await fs.writeFile(USAGE_FILE, JSON.stringify(this.history, null, 2), 'utf-8');
        } catch (error) {
            logger.error('保存使用统计失败:', error);
        }
    }

    /**
     * Record usage for an account
     * @param {string} accountId - Account ID
     * @param {Object} usage - Usage data
     */
    async recordUsage(accountId, usage) {
        if (!this.loaded) await this.load();

        const record = {
            accountId,
            timestamp: Date.now(),
            totalTokens: usage.totalTokens || 0,
            inputTokens: usage.inputTokens || 0,
            outputTokens: usage.outputTokens || 0,
            model: usage.model || 'unknown'
        };

        this.history.push(record);

        // Keep only last 10000 records to prevent file from growing too large
        if (this.history.length > 10000) {
            this.history = this.history.slice(-10000);
        }

        await this.save();
    }

    /**
     * Get usage summary for all accounts
     * @returns {Object} Summary statistics
     */
    async getSummary() {
        if (!this.loaded) await this.load();

        const summary = {
            totalTokens: 0,
            inputTokens: 0,
            outputTokens: 0,
            requestCount: this.history.length,
            byAccount: {}
        };

        for (const record of this.history) {
            summary.totalTokens += record.totalTokens;
            summary.inputTokens += record.inputTokens;
            summary.outputTokens += record.outputTokens;

            if (!summary.byAccount[record.accountId]) {
                summary.byAccount[record.accountId] = {
                    totalTokens: 0,
                    inputTokens: 0,
                    outputTokens: 0,
                    requestCount: 0
                };
            }

            summary.byAccount[record.accountId].totalTokens += record.totalTokens;
            summary.byAccount[record.accountId].inputTokens += record.inputTokens;
            summary.byAccount[record.accountId].outputTokens += record.outputTokens;
            summary.byAccount[record.accountId].requestCount += 1;
        }

        return summary;
    }

    /**
     * Get usage for specific account
     * @param {string} accountId - Account ID
     * @returns {Object} Account usage statistics
     */
    async getAccountUsage(accountId) {
        if (!this.loaded) await this.load();

        const accountRecords = this.history.filter(r => r.accountId === accountId);

        const usage = {
            accountId,
            totalTokens: 0,
            inputTokens: 0,
            outputTokens: 0,
            requestCount: accountRecords.length,
            history: accountRecords.slice(-100) // Last 100 requests
        };

        for (const record of accountRecords) {
            usage.totalTokens += record.totalTokens;
            usage.inputTokens += record.inputTokens;
            usage.outputTokens += record.outputTokens;
        }

        return usage;
    }

    /**
     * Get usage history with filters
     * @param {Object} filters - Filter options
     * @returns {Array} Filtered history
     */
    async getHistory(filters = {}) {
        if (!this.loaded) await this.load();

        let filtered = [...this.history];

        if (filters.accountId) {
            filtered = filtered.filter(r => r.accountId === filters.accountId);
        }

        if (filters.startTime) {
            filtered = filtered.filter(r => r.timestamp >= filters.startTime);
        }

        if (filters.endTime) {
            filtered = filtered.filter(r => r.timestamp <= filters.endTime);
        }

        if (filters.limit) {
            filtered = filtered.slice(-filters.limit);
        }

        return filtered;
    }

    /**
     * Export usage data as CSV
     * @returns {string} CSV formatted data
     */
    async exportCSV() {
        if (!this.loaded) await this.load();

        const headers = ['Timestamp', 'Account ID', 'Model', 'Total Tokens', 'Input Tokens', 'Output Tokens'];
        const rows = [headers.join(',')];

        for (const record of this.history) {
            const row = [
                new Date(record.timestamp).toISOString(),
                record.accountId,
                record.model,
                record.totalTokens,
                record.inputTokens,
                record.outputTokens
            ];
            rows.push(row.join(','));
        }

        return rows.join('\n');
    }

    /**
     * Export usage data as JSON
     * @returns {string} JSON formatted data
     */
    async exportJSON() {
        if (!this.loaded) await this.load();
        return JSON.stringify(this.history, null, 2);
    }

    /**
     * Reset all usage statistics
     */
    async reset() {
        this.history = [];
        await this.save();
        logger.info('使用统计已重置');
    }

    /**
     * Get usage statistics for a time period
     * @param {number} hours - Number of hours to look back
     * @returns {Object} Usage statistics
     */
    async getUsageForPeriod(hours = 24) {
        if (!this.loaded) await this.load();

        const startTime = Date.now() - (hours * 60 * 60 * 1000);
        const records = this.history.filter(r => r.timestamp >= startTime);

        const stats = {
            period: `${hours} hours`,
            totalTokens: 0,
            inputTokens: 0,
            outputTokens: 0,
            requestCount: records.length,
            byModel: {},
            byAccount: {}
        };

        for (const record of records) {
            stats.totalTokens += record.totalTokens;
            stats.inputTokens += record.inputTokens;
            stats.outputTokens += record.outputTokens;

            // By model
            if (!stats.byModel[record.model]) {
                stats.byModel[record.model] = {
                    totalTokens: 0,
                    inputTokens: 0,
                    outputTokens: 0,
                    requestCount: 0
                };
            }
            stats.byModel[record.model].totalTokens += record.totalTokens;
            stats.byModel[record.model].inputTokens += record.inputTokens;
            stats.byModel[record.model].outputTokens += record.outputTokens;
            stats.byModel[record.model].requestCount += 1;

            // By account
            if (!stats.byAccount[record.accountId]) {
                stats.byAccount[record.accountId] = {
                    totalTokens: 0,
                    inputTokens: 0,
                    outputTokens: 0,
                    requestCount: 0
                };
            }
            stats.byAccount[record.accountId].totalTokens += record.totalTokens;
            stats.byAccount[record.accountId].inputTokens += record.inputTokens;
            stats.byAccount[record.accountId].outputTokens += record.outputTokens;
            stats.byAccount[record.accountId].requestCount += 1;
        }

        return stats;
    }
}

// Export singleton instance
const usageTracker = new UsageTracker();
export default usageTracker;

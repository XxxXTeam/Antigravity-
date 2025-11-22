import tokenManager from '../auth/token_manager.js';
import accountFileManager from '../auth/account_file_manager.js';
import usageTracker from '../utils/usage_tracker.js';
import config from '../config/config.js';
import logger from '../utils/logger.js';

/**
 * Generate assistant response with automatic retry and usage tracking
 */
export async function generateAssistantResponse(requestBody, callback, maxRetries = 3) {
  let lastError = null;
  let usageData = {
    totalTokens: 0,
    inputTokens: 0,
    outputTokens: 0
  };

  for (let attempt = 0; attempt < maxRetries; attempt++) {
    let token = null;

    try {
      token = await tokenManager.getToken();

      if (!token) {
        throw new Error('没有可用的token，请添加账号');
      }

      logger.info(`[尝试 ${attempt + 1}/${maxRetries}] 使用账号: ${token.email}`);

      const url = config.api.url;

      const response = await fetch(url, {
        method: 'POST',
        headers: {
          'Host': config.api.host,
          'User-Agent': config.api.userAgent,
          'Authorization': `Bearer ${token.access_token}`,
          'Content-Type': 'application/json',
          'Accept-Encoding': 'gzip'
        },
        body: JSON.stringify(requestBody)
      });

      if (!response.ok) {
        const errorText = await response.text();
        const error = new Error(`API请求失败 (${response.status}): ${errorText}`);
        error.statusCode = response.status;

        // Handle specific error codes
        if (response.status === 403) {
          logger.warn(`账号 ${token.accountId} 返回403，标记错误并切换账号`);
          await accountFileManager.updateErrorTracking(token.accountId, {
            success: false,
            error: `403 Forbidden: ${errorText}`
          });
          throw error;
        } else if (response.status === 429) {
          logger.warn(`账号 ${token.accountId} 触发速率限制，等待后重试`);
          await accountFileManager.updateErrorTracking(token.accountId, {
            success: false,
            error: `429 Rate Limit: ${errorText}`
          });
          // Exponential backoff for rate limits
          const waitTime = Math.min(1000 * Math.pow(2, attempt), 10000);
          await new Promise(resolve => setTimeout(resolve, waitTime));
          throw error;
        }

        throw error;
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let thinkingStarted = false;
      let toolCalls = [];

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value);
        const lines = chunk.split('\n').filter(line => line.startsWith('data: '));

        for (const line of lines) {
          const jsonStr = line.slice(6);
          
          // 跳过空行或无效行
          if (!jsonStr || jsonStr.trim() === '') {
            continue;
          }
          
          try {
            const data = JSON.parse(jsonStr);
            const parts = data.response?.candidates?.[0]?.content?.parts;

            // Extract usage metadata if available
            if (data.response?.usageMetadata) {
              const metadata = data.response.usageMetadata;
              usageData.totalTokens = metadata.totalTokenCount || 0;
              usageData.inputTokens = metadata.promptTokenCount || 0;
              usageData.outputTokens = metadata.candidatesTokenCount || 0;
            }

            if (parts) {
              for (const part of parts) {
                if (part.thought === true) {
                  // 思考内容直接发送，不添加<think>标签
                  thinkingStarted = true;
                  callback({ type: 'thinking', content: part.text || '' });
                } else if (part.text !== undefined) {
                  // 结束思考模式
                  thinkingStarted = false;
                  callback({ type: 'text', content: part.text });
                } else if (part.functionCall) {
                  toolCalls.push({
                    id: part.functionCall.id,
                    type: 'function',
                    function: {
                      name: part.functionCall.name,
                      arguments: JSON.stringify(part.functionCall.args)
                    }
                  });
                }
              }
            }

            // 当遇到 finishReason 时，发送所有收集的工具调用
            if (data.response?.candidates?.[0]?.finishReason && toolCalls.length > 0) {
              callback({ type: 'tool_calls', tool_calls: toolCalls });
              toolCalls = [];
            }
          } catch (e) {
            // 记录JSON解析错误（仅在调试模式）
            if (process.env.DEBUG) {
              logger.warn(`JSON解析失败: ${e.message}, 原始数据: ${jsonStr.substring(0, 100)}...`);
            }
            // 继续处理下一行
            continue;
          }
        }
      }

      // Track usage statistics
      if (usageData.totalTokens > 0) {
        await accountFileManager.updateUsage(token.accountId, usageData);
        await usageTracker.recordUsage(token.accountId, {
          ...usageData,
          model: requestBody.model || 'unknown'
        });
        logger.info(`使用统计: 总计 ${usageData.totalTokens} tokens (输入: ${usageData.inputTokens}, 输出: ${usageData.outputTokens})`);
      }

      // Mark account as successful
      await accountFileManager.updateErrorTracking(token.accountId, { success: true });

      // Return usage data
      return usageData;

    } catch (error) {
      lastError = error;
      logger.error(`请求失败 (尝试 ${attempt + 1}/${maxRetries}):`, error.message);

      // If this is the last attempt, throw the error
      if (attempt === maxRetries - 1) {
        break;
      }

      // Wait before retry (exponential backoff)
      const waitTime = Math.min(500 * Math.pow(2, attempt), 5000);
      logger.info(`等待 ${waitTime}ms 后重试...`);
      await new Promise(resolve => setTimeout(resolve, waitTime));
    }
  }

  // All retries failed
  throw lastError || new Error('请求失败，已达到最大重试次数');
}

/**
 * Get available models from all accounts
 */
export async function getAvailableModels() {
  const token = await tokenManager.getToken();

  if (!token) {
    throw new Error('没有可用的token，请添加账号');
  }

  // If token has cached models, return them
  if (token.models && Object.keys(token.models).length > 0) {
    return {
      object: 'list',
      data: Object.values(token.models)
    };
  }

  // Otherwise fetch from API
  try {
    const response = await fetch(config.api.modelsUrl, {
      method: 'POST',
      headers: {
        'Host': config.api.host,
        'User-Agent': config.api.userAgent,
        'Authorization': `Bearer ${token.access_token}`,
        'Content-Type': 'application/json',
        'Accept-Encoding': 'gzip'
      },
      body: JSON.stringify({})
    });

    const data = await response.json();

    return {
      object: 'list',
      data: Object.keys(data.models).map(id => ({
        id,
        object: 'model',
        created: Math.floor(Date.now() / 1000),
        owned_by: 'google'
      }))
    };
  } catch (error) {
    logger.error('获取模型列表失败:', error);
    throw error;
  }
}

/**
 * Get models for specific account
 */
export async function getAccountModels(accountId) {
  try {
    const account = await accountFileManager.readAccount(accountId);

    if (account.models && Object.keys(account.models).length > 0) {
      return {
        object: 'list',
        data: Object.values(account.models)
      };
    }

    return {
      object: 'list',
      data: []
    };
  } catch (error) {
    logger.error(`获取账号 ${accountId} 模型列表失败:`, error);
    throw error;
  }
}

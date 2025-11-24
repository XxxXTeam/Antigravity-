package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/antigravity/api-proxy/internal/logger"
	"github.com/antigravity/api-proxy/internal/models"
	"github.com/antigravity/api-proxy/internal/oauth"
	"github.com/antigravity/api-proxy/internal/storage"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ==================== 管理员认证 ====================

func (s *Server) adminLogin(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// 验证密码
	if req.Password != s.cfg.Security.AdminPassword {
		s.logger.Warn("Failed login attempt")
		c.JSON(401, gin.H{"error": "Invalid password"})
		return
	}

	// 生成简单的token（实际应使用JWT）
	token := generateToken(req.Password)

	s.logger.Info("Admin logged in successfully")
	c.JSON(200, gin.H{
		"success": true,
		"token":   token,
	})
}

func (s *Server) adminLogout(c *gin.Context) {
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) adminVerify(c *gin.Context) {
	token := c.GetHeader("X-Admin-Token")
	if token == "" {
		c.JSON(401, gin.H{"valid": false})
		return
	}

	expectedToken := generateToken(s.cfg.Security.AdminPassword)

	if token != expectedToken {
		c.JSON(401, gin.H{"valid": false})
		return
	}

	c.JSON(200, gin.H{"valid": true})
}

// ==================== Token 管理 ====================

func (s *Server) listTokens(c *gin.Context) {
	accountsDir := s.cfg.Storage.AccountsDir

	// 读取所有账号文件
	entries, err := os.ReadDir(accountsDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(200, gin.H{"data": []interface{}{}})
			return
		}
		s.logger.Error("Failed to read accounts directory", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to read accounts"})
		return
	}

	var tokens []map[string]interface{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// 读取账号文件
		filePath := filepath.Join(accountsDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			s.logger.Warn("Failed to read account file", zap.String("file", entry.Name()), zap.Error(err))
			continue
		}

		var account map[string]interface{}
		if err := json.Unmarshal(data, &account); err != nil {
			s.logger.Warn("Failed to parse account file", zap.String("file", entry.Name()), zap.Error(err))
			continue
		}

		// 计算模型数量
		modelCount := 0
		if models, ok := account["models"].(map[string]interface{}); ok {
			modelCount = len(models)
		}
		account["modelCount"] = modelCount

		// 添加创建时间（使用timestamp字段）
		if timestamp, ok := account["timestamp"].(float64); ok {
			account["created"] = time.Unix(int64(timestamp/1000), 0).Format("2006-01-02 15:04:05")
		} else {
			account["created"] = "Unknown"
		}

		tokens = append(tokens, account)
	}

	// 确保返回数组，即使为空
	if tokens == nil {
		tokens = []map[string]interface{}{}
	}

	// 直接返回数组，而不是包装在data字段中
	c.JSON(200, tokens)
}

func (s *Server) triggerOAuthLogin(c *gin.Context) {
	// 使用主服务器端口作为OAuth回调端口（共享端口）
	serverPort := s.cfg.Server.Port

	// 创建OAuth客户端，使用服务器端口
	client := oauth.NewClient(serverPort, s.cfg.Storage.AccountsDir, s.logger)

	// 生成授权URL
	state := generateRandomString(32)
	authURL := client.GetAuthCodeURL(state)

	s.logger.Info("OAuth login triggered",
		zap.String("state", state),
		zap.String("callback", fmt.Sprintf("http://localhost:%d/oauth-callback", serverPort)))

	c.JSON(200, gin.H{
		"url":     authURL,
		"state":   state,
		"message": "Opening authorization window...",
	})
}

func (s *Server) addTokenFromCallback(c *gin.Context) {
	var req struct {
		CallbackUrl string `json:"callbackUrl" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// Parse the URL to get the code
	parsedURL, err := url.Parse(req.CallbackUrl)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid callback URL"})
		return
	}

	code := parsedURL.Query().Get("code")
	if code == "" {
		c.JSON(400, gin.H{"error": "No code found in callback URL"})
		return
	}

	// Create OAuth client
	client := oauth.NewClient(s.cfg.Server.Port, s.cfg.Storage.AccountsDir, s.logger)

	// Exchange code for token
	token, err := client.GetOAuthConfig().Exchange(context.Background(), code)
	if err != nil {
		s.logger.Error("Failed to exchange code", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to exchange code for token"})
		return
	}

	// Get user info
	userInfo, err := client.GetUserInfo(token.AccessToken)
	if err != nil {
		s.logger.Error("Failed to get user info", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to get user info"})
		return
	}

	// Save account
	account, err := client.SaveAccountFromToken(token, userInfo)
	if err != nil {
		s.logger.Error("Failed to save account", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to save account"})
		return
	}

	s.logger.Info("Account added successfully",
		zap.String("email", account.Email),
		zap.String("account_id", account.AccountID))

	c.JSON(200, gin.H{
		"success": true,
		"account": gin.H{
			"id":    account.AccountID,
			"email": account.Email,
			"name":  account.Name,
		},
	})
}

func (s *Server) toggleToken(c *gin.Context) {
	accountID := c.Param("id")

	var req struct {
		Enable bool `json:"enable"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// 读取账号文件
	filePath := filepath.Join(s.cfg.Storage.AccountsDir, accountID+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		c.JSON(404, gin.H{"error": "Account not found"})
		return
	}

	var account map[string]interface{}
	if err := json.Unmarshal(data, &account); err != nil {
		c.JSON(500, gin.H{"error": "Failed to parse account"})
		return
	}

	// 更新enable状态
	account["enable"] = req.Enable

	// 写回文件
	updatedData, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to serialize account"})
		return
	}

	if err := os.WriteFile(filePath, updatedData, 0644); err != nil {
		c.JSON(500, gin.H{"error": "Failed to save account"})
		return
	}

	s.logger.Info("Token toggled",
		zap.String("account_id", accountID),
		zap.Bool("enable", req.Enable))

	c.JSON(200, gin.H{"success": true})
}

func (s *Server) deleteToken(c *gin.Context) {
	accountID := c.Param("id")

	filePath := filepath.Join(s.cfg.Storage.AccountsDir, accountID+".json")
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			c.JSON(404, gin.H{"error": "Account not found"})
			return
		}
		c.JSON(500, gin.H{"error": "Failed to delete account"})
		return
	}

	s.logger.Info("Token deleted", zap.String("account_id", accountID))
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) getTokenStats(c *gin.Context) {
	// 统计Token使用情况
	accountsDir := s.cfg.Storage.AccountsDir
	entries, _ := os.ReadDir(accountsDir)

	enabled := 0
	disabled := 0

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			// 读取文件检查enable状态
			filePath := filepath.Join(accountsDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err == nil {
				var account map[string]interface{}
				if json.Unmarshal(data, &account) == nil {
					if enable, ok := account["enable"].(bool); ok && enable {
						enabled++
					} else {
						disabled++
					}
				}
			}
		}
	}

	c.JSON(200, gin.H{
		"enabled":  enabled,
		"disabled": disabled,
	})
}

func (s *Server) getTokenUsage(c *gin.Context) {
	// 获取 Token 轮询使用统计
	accountsDir := s.cfg.Storage.AccountsDir
	entries, _ := os.ReadDir(accountsDir)

	var tokenStats []gin.H
	totalRequests := 0
	currentIndex := 0 // TODO: Track actual round-robin index if implementing load balancing

	for i, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			filePath := filepath.Join(accountsDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err == nil {
				var account map[string]interface{}
				if json.Unmarshal(data, &account) == nil {
					requests := 0
					var lastUsed interface{}

					// Extract usage info if available
					if usage, ok := account["usage"].(map[string]interface{}); ok {
						if reqCount, ok := usage["requestCount"].(float64); ok {
							requests = int(reqCount)
							totalRequests += requests
						}
						lastUsed = usage["lastUsed"]
					}

					tokenStats = append(tokenStats, gin.H{
						"index":     i,
						"requests":  requests,
						"lastUsed":  lastUsed,
						"isCurrent": i == currentIndex,
					})
				}
			}
		}
	}

	c.JSON(200, gin.H{
		"totalTokens":   len(tokenStats),
		"currentIndex":  currentIndex,
		"totalRequests": totalRequests,
		"tokens":        tokenStats,
	})
}

func (s *Server) getUsageSummary(c *gin.Context) {
	// 获取使用统计摘要
	accountsDir := s.cfg.Storage.AccountsDir
	entries, _ := os.ReadDir(accountsDir)

	totalRequests := 0
	totalTokens := 0
	inputTokens := 0
	outputTokens := 0
	activeAccounts := 0

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			totalTokens++
			filePath := filepath.Join(accountsDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err == nil {
				var account map[string]interface{}
				if json.Unmarshal(data, &account) == nil {
					if enable, ok := account["enable"].(bool); ok && enable {
						activeAccounts++
					}
					
					// Aggregate usage if available
					if usage, ok := account["usage"].(map[string]interface{}); ok {
						if total, ok := usage["total_requests"].(float64); ok {
							totalRequests += int(total)
						}
						if input, ok := usage["input_tokens"].(float64); ok {
							inputTokens += int(input)
						}
						if output, ok := usage["output_tokens"].(float64); ok {
							outputTokens += int(output)
						}
					}
				}
			}
		}
	}

	c.JSON(200, gin.H{
		"totalRequests":  totalRequests,
		"totalTokens":    totalTokens,
		"inputTokens":    inputTokens,
		"outputTokens":   outputTokens,
		"activeAccounts": activeAccounts,
	})
}

func (s *Server) getUsageHistory(c *gin.Context) {
	// Get usage history for the last 30 days
	history, err := s.usageStore.GetUsageHistory(30)
	if err != nil {
		s.logger.Error("Failed to get usage history", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to get usage history"})
		return
	}

	// Group by date for aggregated view
	dateMap := make(map[string]*storage.UsageRecord)
	for _, record := range history {
		if existing, ok := dateMap[record.Date]; ok {
			existing.TotalTokens += record.TotalTokens
			existing.InputTokens += record.InputTokens
			existing.OutputTokens += record.OutputTokens
			existing.RequestCount += record.RequestCount
		} else {
			copy := record
			dateMap[record.Date] = &copy
		}
	}

	// Convert to array
	var result []gin.H
	for _, record := range dateMap {
		result = append(result, gin.H{
			"date":         record.Date,
			"totalTokens":  record.TotalTokens,
			"inputTokens":  record.InputTokens,
			"outputTokens": record.OutputTokens,
			"requestCount": record.RequestCount,
		})
	}

	if result == nil {
		result = []gin.H{}
	}

	c.JSON(200, result)
}

func (s *Server) getUsage(c *gin.Context) {
	// 获取真实的系统使用情况
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(200, gin.H{
		"memoryAlloc": fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024),
		"memoryTotal": fmt.Sprintf("%.2f MB", float64(m.TotalAlloc)/1024/1024),
		"memorySys":   fmt.Sprintf("%.2f MB", float64(m.Sys)/1024/1024),
		"numGC":       m.NumGC,
	})
}

// ==================== 密钥管理 ====================

func (s *Server) listKeys(c *gin.Context) {
	keys, err := s.keyStore.List()
	if err != nil {
		s.logger.Error("Failed to list keys", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to list keys"})
		return
	}

	// Convert to response format
	var response []gin.H
	for _, key := range keys {
		response = append(response, gin.H{
			"key":        key.Key,
			"name":       key.Name,
			"createdAt":  key.CreatedAt,
			"lastUsed":   key.LastUsed,
			"usageCount": key.UsageCount,
		})
	}

	if response == nil {
		response = []gin.H{}
	}

	c.JSON(200, response)
}

func (s *Server) generateKey(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		req.Name = "Default Key"
	}

	// Generate a new key
	keyString := fmt.Sprintf("sk-antigravity-%s", generateRandomString(32))
	now := time.Now().UnixMilli()

	apiKey := &models.APIKey{
		Key:        keyString,
		Name:       req.Name,
		CreatedAt:  now,
		UsageCount: 0,
	}

	// Save the key
	if err := s.keyStore.Save(apiKey); err != nil {
		s.logger.Error("Failed to save key", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to generate key"})
		return
	}

	s.logger.Info("API key generated", zap.String("key", keyString), zap.String("name", req.Name))

	c.JSON(200, gin.H{
		"key":       keyString,
		"name":      req.Name,
		"createdAt": now,
		"message":   "Key generated successfully. Save it securely!",
	})
}

func (s *Server) deleteKey(c *gin.Context) {
	keyString := c.Param("key")

	if err := s.keyStore.Delete(keyString); err != nil {
		if os.IsNotExist(err) {
			c.JSON(404, gin.H{"error": "Key not found"})
			return
		}
		s.logger.Error("Failed to delete key", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to delete key"})
		return
	}

	s.logger.Info("API key deleted", zap.String("key", keyString))
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) getKeyStats(c *gin.Context) {
	keys, err := s.keyStore.List()
	if err != nil {
		s.logger.Error("Failed to get key stats", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to get stats"})
		return
	}

	totalKeys := len(keys)
	totalRequests := int64(0)

	for _, key := range keys {
		totalRequests += key.UsageCount
	}

	c.JSON(200, gin.H{
		"totalKeys":     totalKeys,
		"totalRequests": totalRequests,
	})
}

// ==================== 日志和监控 ====================

func (s *Server) getLogs(c *gin.Context) {
	limit := 100
	// Parse limit from query if needed, but for now default to 100
	logs := logger.GlobalBuffer.GetRecent(limit)
	c.JSON(200, logs)
}

func (s *Server) clearLogs(c *gin.Context) {
	logger.GlobalBuffer.Clear()
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) getSystemStatus(c *gin.Context) {
	// 获取真实的系统状态
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Format memory usage
	memoryUsage := fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024)

	// CPU usage (placeholder - Go doesn't easily provide this without external libs)
	cpuUsage := "0"

	// Uptime (placeholder - requires tracking server start time)
	uptime := "0h 0m"

	// System info
	nodeVersion := runtime.Version() // Go version
	platform := runtime.GOOS
	pid := os.Getpid()
	systemMemory := fmt.Sprintf("%.2f GB", float64(m.Sys)/1024/1024/1024)

	c.JSON(200, gin.H{
		"cpu":          cpuUsage,
		"memory":       memoryUsage,
		"uptime":       uptime,
		"requests":     0, // TODO: Track total requests
		"idle":         "活跃",
		"idleTime":     0,
		"nodeVersion":  nodeVersion,
		"platform":     platform,
		"pid":          pid,
		"systemMemory": systemMemory,
	})
}

// ==================== 设置 ====================

func (s *Server) getSettings(c *gin.Context) {
	c.JSON(200, gin.H{
		"server": gin.H{
			"port": s.cfg.Server.Port,
			"host": s.cfg.Server.Host,
		},
		"security": gin.H{
			"apiKey":         s.cfg.Security.APIKey,
			"adminPassword":  s.cfg.Security.AdminPassword,
			"maxRequestSize": "50mb", // TODO: Add to config if needed
		},
		"defaults": gin.H{
			"temperature": 1.0, // TODO: Add default model parameters to config
			"top_p":       0.85,
			"top_k":       50,
			"max_tokens":  8096,
		},
		"systemInstruction": "", // TODO: Add to config if needed
	})
}

func (s *Server) saveSettings(c *gin.Context) {
	c.JSON(200, gin.H{"success": true})
}

// ==================== 工具函数 ====================

func generateToken(password string) string {
	// 使用固定盐值生成token，确保重启后token保持一致
	h := sha256.New()
	h.Write([]byte("antigravity-admin-" + password))
	return hex.EncodeToString(h.Sum(nil))
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}

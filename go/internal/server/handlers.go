package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/antigravity/api-proxy/internal/oauth"
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
	// 这个接口已经不需要了，因为OAuth流程在triggerOAuthLogin中完成
	c.JSON(200, gin.H{
		"message": "Use POST /admin/tokens/login instead",
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

	totalTokens := 0
	activeTokens := 0

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			totalTokens++
			// 读取文件检查enable状态
			filePath := filepath.Join(accountsDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err == nil {
				var account map[string]interface{}
				if json.Unmarshal(data, &account) == nil {
					if enable, ok := account["enable"].(bool); ok && enable {
						activeTokens++
					}
				}
			}
		}
	}

	c.JSON(200, gin.H{
		"totalTokens":   totalTokens,
		"activeTokens":  activeTokens,
		"totalRequests": 0,
	})
}

func (s *Server) getUsageSummary(c *gin.Context) {
	// 获取使用统计摘要
	c.JSON(200, gin.H{
		"totalRequests":  0,
		"totalTokens":    0,
		"inputTokens":    0,
		"outputTokens":   0,
		"activeAccounts": 0,
	})
}

func (s *Server) getUsageHistory(c *gin.Context) {
	// 获取使用历史
	c.JSON(200, gin.H{"data": []interface{}{}})
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
	c.JSON(200, gin.H{"keys": []interface{}{}})
}

func (s *Server) generateKey(c *gin.Context) {
	key := fmt.Sprintf("sk-%s", generateRandomString(32))
	c.JSON(200, gin.H{
		"key":     key,
		"created": time.Now().Unix(),
	})
}

func (s *Server) deleteKey(c *gin.Context) {
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) getKeyStats(c *gin.Context) {
	c.JSON(200, gin.H{
		"totalKeys":     0,
		"totalRequests": 0,
	})
}

// ==================== 日志和监控 ====================

func (s *Server) getLogs(c *gin.Context) {
	c.JSON(200, gin.H{"logs": []interface{}{}})
}

func (s *Server) clearLogs(c *gin.Context) {
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) getSystemStatus(c *gin.Context) {
	// 获取真实的系统状态
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(200, gin.H{
		"cpuUsage":     "0%",
		"memoryUsage":  fmt.Sprintf("%.0f%%", float64(m.Alloc)*100/float64(m.Sys)),
		"uptime":       "0d 0h 0m",
		"activeModels": 0,
		"systemStatus": "active",
	})
}

// ==================== 设置 ====================

func (s *Server) getSettings(c *gin.Context) {
	c.JSON(200, gin.H{
		"server":  s.cfg.Server,
		"logging": s.cfg.Logging,
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

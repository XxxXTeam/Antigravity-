package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/antigravity/api-proxy/internal/config"
	"github.com/antigravity/api-proxy/internal/oauth"
	"github.com/antigravity/api-proxy/internal/storage"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Server represents the API server
type Server struct {
	cfg         *config.Config
	logger      *zap.Logger
	router      *gin.Engine
	oauthClient *oauth.Client
	keyStore    *storage.KeyStore
	usageStore  *storage.UsageStore
}

// New creates a new server instance
func New(cfg *config.Config, logger *zap.Logger) (*Server, error) {
	// 设置Gin模式
	gin.SetMode(cfg.Server.Mode)

	s := &Server{
		cfg:    cfg,
		logger: logger,
		router: gin.New(),
	}

	// Initialize storage
	s.keyStore = storage.NewKeyStore(cfg.Storage.KeysDir)
	s.usageStore = storage.NewUsageStore(cfg.Storage.UsageDir)

	// Initialize OAuth client (uses server port for callback)
	s.oauthClient = oauth.NewClient(cfg.Server.Port, cfg.Storage.AccountsDir, logger)
	s.oauthClient.StartBackgroundRefresh()

	// 设置中间件
	s.setupMiddleware()

	// 设置路由
	s.setupRoutes()

	return s, nil
}

// Router returns the gin engine
func (s *Server) Router() *gin.Engine {
	return s.router
}

func (s *Server) setupMiddleware() {
	// Recovery middleware
	s.router.Use(gin.Recovery())

	// Logger middleware
	s.router.Use(s.loggerMiddleware())

	// CORS middleware
	if s.cfg.Security.EnableCORS {
		s.router.Use(s.corsMiddleware())
	}
}

func (s *Server) setupRoutes() {
	// 根路径返回简单状态
	s.router.GET("/", func(c *gin.Context) {
		c.String(200, "ok")
	})

	// 健康检查
	s.router.GET("/health", s.healthCheck)
	s.router.GET("/ping", s.ping)

	// OpenAI兼容 API - 需要API Key认证
	api := s.router.Group("/v1")
	api.Use(s.apiKeyAuthMiddleware())
	{
		api.POST("/chat/completions", s.chatCompletions)
		api.GET("/models", s.listModels)
	}

	// 管理后台API
	admin := s.router.Group("/admin")
	{
		// 认证
		admin.POST("/login", s.adminLogin)
		admin.POST("/logout", s.adminLogout)
		admin.GET("/verify", s.adminVerify)

		// 需要认证的路由
		auth := admin.Group("/")
		auth.Use(s.adminAuthMiddleware())
		{
			// Token管理
			auth.GET("/tokens", s.listTokens)
			auth.POST("/tokens/login", s.triggerOAuthLogin)
			auth.POST("/tokens/callback", s.addTokenFromCallback)
			auth.PATCH("/tokens/:id", s.toggleToken)
			auth.DELETE("/tokens/:id", s.deleteToken)
			auth.GET("/tokens/stats", s.getTokenStats)
			auth.GET("/tokens/usage", s.getTokenUsage)

			// 密钥管理
			auth.GET("/keys", s.listKeys)
			auth.POST("/keys/generate", s.generateKey)
			auth.DELETE("/keys/:key", s.deleteKey)
			auth.GET("/keys/stats", s.getKeyStats)

			// 日志
			auth.GET("/logs", s.getLogs)
			auth.DELETE("/logs", s.clearLogs)

			// 监控
			auth.GET("/status", s.getSystemStatus)

			// 设置
			auth.GET("/settings", s.getSettings)
			auth.POST("/settings", s.saveSettings)

			// 使用统计
			auth.GET("/usage/summary", s.getUsageSummary)
			auth.GET("/usage/history", s.getUsageHistory)
		}
	}

	// OAuth回调路由（与主服务器共享端口）
	s.router.GET("/oauth-callback", s.handleOAuthCallback)

	// 静态文件（管理后台前端）- 放在 /ui 路径
	s.setupStaticFiles()
}

// 基础handlers
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}

func (s *Server) ping(c *gin.Context) {
	c.JSON(200, gin.H{"message": "pong"})
}

// API handlers - chatCompletions 在 proxy.go 中实现

func (s *Server) listModels(c *gin.Context) {
	accountsDir := s.cfg.Storage.AccountsDir

	// 用map去重模型
	modelsMap := make(map[string]gin.H)

	// 读取所有账号文件
	entries, err := os.ReadDir(accountsDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}

			filePath := filepath.Join(accountsDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			var account map[string]interface{}
			if err := json.Unmarshal(data, &account); err != nil {
				continue
			}

			// 提取models字段
			if models, ok := account["models"].(map[string]interface{}); ok {
				for modelID, modelData := range models {
					if model, ok := modelData.(map[string]interface{}); ok {
						modelsMap[modelID] = gin.H{
							"id":       modelID,
							"object":   "model",
							"owned_by": model["owned_by"],
						}
					}
				}
			}
		}
	}

	// 转换为数组
	var modelsList []gin.H
	for _, model := range modelsMap {
		modelsList = append(modelsList, model)
	}

	// 确保至少返回空数组
	if modelsList == nil {
		modelsList = []gin.H{}
	}

	c.JSON(200, gin.H{
		"object": "list",
		"data":   modelsList,
	})
}

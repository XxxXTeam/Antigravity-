package server

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// loggerMiddleware logs HTTP requests
func (s *Server) loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		clientIP := c.ClientIP()

		s.logger.Info("HTTP Request",
			zap.String("method", method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", statusCode),
			zap.Duration("latency", latency),
			zap.String("client_ip", clientIP),
		)
	}
}

// corsMiddleware handles CORS
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// 检查是否允许该来源
		allowed := false
		for _, allowedOrigin := range s.cfg.Security.AllowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			if origin != "" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			}
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Admin-Token")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// apiKeyAuthMiddleware validates API key for API requests
func (s *Server) apiKeyAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get API key from Authorization header
		authHeader := c.GetHeader("Authorization")
		
		if authHeader == "" {
			c.JSON(401, gin.H{
				"error": gin.H{
					"message": "Missing Authorization header",
					"type":    "invalid_request_error",
					"code":    "missing_api_key",
				},
			})
			c.Abort()
			return
		}

		// Extract Bearer token
		apiKey := ""
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			apiKey = authHeader[7:]
		} else {
			apiKey = authHeader
		}


		// First, check if it matches the static API key from config (backward compatibility)
		if s.cfg.Security.APIKey != "" && apiKey == s.cfg.Security.APIKey {
			s.logger.Info("API request authenticated with config API key",
				zap.String("client_ip", c.ClientIP()))
			c.Set("api_key_source", "config")
			c.Next()
			return
		}

		// Log for debugging if config key doesn't match
		if s.cfg.Security.APIKey != "" {
			s.logger.Debug("Config API key check failed",
				zap.String("config_key_prefix", maskAPIKey(s.cfg.Security.APIKey)),
				zap.String("provided_key_prefix", maskAPIKey(apiKey)))
		}

		// Second, validate against dynamic API keys from keyStore
		key, err := s.keyStore.Load(apiKey)
		if err != nil {
			s.logger.Warn("Invalid API key attempt",
				zap.String("key_prefix", maskAPIKey(apiKey)),
				zap.String("client_ip", c.ClientIP()))
			
			c.JSON(401, gin.H{
				"error": gin.H{
					"message": "Invalid API key",
					"type":    "invalid_request_error",
					"code":    "invalid_api_key",
				},
			})
			c.Abort()
			return
		}

		// Update usage for dynamic keys
		key.UpdateUsage()
		if err := s.keyStore.Save(key); err != nil {
			s.logger.Error("Failed to update key usage", zap.Error(err))
		}

		// Store key in context for later use
		c.Set("api_key", key)
		c.Set("api_key_source", "database")
		
		c.Next()
	}
}

// adminAuthMiddleware checks admin authentication
func (s *Server) adminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Admin-Token")

		if token == "" {
			c.JSON(401, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		// Validate token against the expected admin token
		expectedToken := generateToken(s.cfg.Security.AdminPassword)
		if token != expectedToken {
			s.logger.Warn("Invalid admin token attempt",
				zap.String("client_ip", c.ClientIP()))
			c.JSON(401, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// maskAPIKey returns a masked version of the API key for logging
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

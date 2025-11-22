package server

import (
	"context"
	"fmt"

	"github.com/antigravity/api-proxy/internal/oauth"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// handleOAuthCallback 处理OAuth回调（与主服务器共享端口）
func (s *Server) handleOAuthCallback(c *gin.Context) {
	code := c.Query("code")
	_ = c.Query("state") // state验证在实际应用中应该做，这里简化处理

	if code == "" {
		errorMsg := c.Query("error")
		s.logger.Error("OAuth callback error", zap.String("error", errorMsg))
		errorHTML := fmt.Sprintf(`<html>
<head><title>授权失败</title></head>
<body style="font-family: Arial; padding: 50px; text-align: center;">
	<h1>❌ 授权失败</h1>
	<p>错误: %s</p>
	<p><a href="/ui/index.html">返回管理面板</a></p>
</body>
</html>`, errorMsg)
		c.Data(200, "text/html; charset=utf-8", []byte(errorHTML))
		return
	}

	// 创建OAuth客户端处理回调
	client := oauth.NewClient(s.cfg.Server.Port, s.cfg.Storage.AccountsDir, s.logger)

	// 交换code获取token
	token, err := client.GetOAuthConfig().Exchange(context.Background(), code)
	if err != nil {
		s.logger.Error("Failed to exchange code", zap.Error(err))
		errorHTML := `<html>
<head><title>授权失败</title></head>
<body style="font-family: Arial; padding: 50px; text-align: center;">
	<h1>❌ 授权失败</h1>
	<p>无法获取访问令牌</p>
	<p><a href="/ui/index.html">返回管理面板</a></p>
</body>
</html>`
		c.Data(200, "text/html; charset=utf-8", []byte(errorHTML))
		return
	}

	// 获取用户信息
	userInfo, err := client.GetUserInfo(token.AccessToken)
	if err != nil {
		s.logger.Error("Failed to get user info", zap.Error(err))
		errorHTML := `<html>
<head><title>授权失败</title></head>
<body style="font-family: Arial; padding: 50px; text-align: center;">
	<h1>❌ 授权失败</h1>
	<p>无法获取用户信息</p>
	<p><a href="/ui/index.html">返回管理面板</a></p>
</body>
</html>`
		c.Data(200, "text/html; charset=utf-8", []byte(errorHTML))
		return
	}

	// 保存账号
	account, err := client.SaveAccountFromToken(token, userInfo)
	if err != nil {
		s.logger.Error("Failed to save account", zap.Error(err))
		errorHTML := `<html>
<head><title>保存失败</title></head>
<body style="font-family: Arial; padding: 50px; text-align: center;">
	<h1>⚠️ 保存失败</h1>
	<p>无法保存账号信息</p>
	<p><a href="/ui/index.html">返回管理面板</a></p>
</body>
</html>`
		c.Data(200, "text/html; charset=utf-8", []byte(errorHTML))
		return
	}

	s.logger.Info("OAuth login successful",
		zap.String("email", account.Email),
		zap.String("account_id", account.AccountID),
		zap.Int("models", len(account.Models)))

	// 返回成功页面（自动关闭）
	successHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>授权成功</title>
    <style>
        body { font-family: Arial, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); }
        .container { background: white; padding: 40px; border-radius: 10px; box-shadow: 0 10px 40px rgba(0,0,0,0.2); text-align: center; }
        .success { color: #27ae60; font-size: 48px; margin-bottom: 20px; }
        h1 { color: #2c3e50; margin: 0 0 10px 0; }
        p { color: #7f8c8d; }
    </style>
</head>
<body>
    <div class="container">
        <div class="success">✓</div>
        <h1>授权成功！</h1>
        <p>账号: <strong>%s</strong></p>
        <p>邮箱: <strong>%s</strong></p>
        <p>可用模型: <strong>%d</strong> 个</p>
        <p>该窗口将在 3 秒后自动关闭...</p>
    </div>
    <script>
        setTimeout(() => window.close(), 3000);
    </script>
</body>
</html>`, account.Name, account.Email, len(account.Models))
	c.Data(200, "text/html; charset=utf-8", []byte(successHTML))
}

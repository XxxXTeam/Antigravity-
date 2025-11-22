package server

import (
	"net/http"
	"os"

	"github.com/antigravity/api-proxy/internal/embed"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// setupStaticFiles 设置静态文件服务
// 优先使用嵌入的文件，如果不存在则使用外部public目录
// 静态文件放在根路径，API保持在 /admin 路径，避免冲突
func (s *Server) setupStaticFiles() {
	// 尝试使用嵌入的文件系统
	if embed.HasEmbeddedFiles() {
		s.logger.Info("Using embedded public files")
		publicFS, err := embed.GetPublicFS()
		if err == nil {
			// 静态文件放在 /ui 路径下
			s.router.StaticFS("/ui", http.FS(publicFS))
			return
		}
		s.logger.Warn("Failed to load embedded files", zap.Error(err))
	}

	// 回退到外部目录
	if _, err := os.Stat("./public"); err == nil {
		s.logger.Info("Using external public directory")
		s.router.Static("/ui", "./public")
		return
	}

	s.logger.Warn("No public files found (embedded or external)")
	// 提供一个简单的fallback页面
	s.router.GET("/ui", func(c *gin.Context) {
		c.HTML(404, "", gin.H{})
		c.Writer.WriteString(`
			<html>
			<head><title>Admin Panel Not Found</title></head>
			<body style="font-family: Arial; padding: 50px; text-align: center;">
				<h1>❌ Admin Panel Not Found</h1>
				<p>The admin panel files are not embedded in this build.</p>
				<p>Please rebuild with: <code>make build</code></p>
				<p>API endpoints are available at: <code>/admin/*</code></p>
			</body>
			</html>
		`)
	})
}

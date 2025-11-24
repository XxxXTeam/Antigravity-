package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/antigravity/api-proxy/internal/config"
	"github.com/antigravity/api-proxy/internal/logger"
	"github.com/antigravity/api-proxy/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the API proxy server",
	Long:  `Start the Antigravity API proxy server with all services`,
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().String("host", "0.0.0.0", "server host")
	serveCmd.Flags().Int("port", 8045, "server port")
	serveCmd.Flags().String("mode", "release", "server mode (debug/release/test)")

	viper.BindPFlag("server.host", serveCmd.Flags().Lookup("host"))
	viper.BindPFlag("server.port", serveCmd.Flags().Lookup("port"))
	viper.BindPFlag("server.mode", serveCmd.Flags().Lookup("mode"))
}

func runServe(cmd *cobra.Command, args []string) error {
	// 加载或创建配置
	cfg, err := config.LoadOrCreate()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 初始化日志
	log, err := logger.New(cfg.Logging)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer log.Sync()

	// 初始化目录结构
	if err := initDirectories(cfg); err != nil {
		log.Error("Failed to initialize directories", zap.Error(err))
		return err
	}

	log.Info("Starting Antigravity API Proxy",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port),
	)

	// Log API key configuration status
	if cfg.Security.APIKey != "" {
		log.Info("Config API key is set",
			zap.String("key_prefix", maskAPIKey(cfg.Security.APIKey)))
	} else {
		log.Info("No config API key set, will use dynamic keys only")
	}

	// 创建服务器
	srv, err := server.New(cfg, log)
	if err != nil {
		log.Error("Failed to create server", zap.Error(err))
		return err
	}

	// 启动HTTP服务器
	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      srv.Router(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 优雅关闭
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("Server started", zap.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed", zap.Error(err))
		}
	}()

	<-stop
	log.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", zap.Error(err))
		return err
	}

	log.Info("Server stopped gracefully")
	return nil
}

func initDirectories(cfg *config.Config) error {
	dirs := []string{
		cfg.Storage.DataDir,
		cfg.Storage.AccountsDir,
		cfg.Storage.KeysDir,
		cfg.Storage.UsageDir,
		cfg.Storage.LogsDir,
		"./uploads",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// maskAPIKey returns a masked version of the API key for logging
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

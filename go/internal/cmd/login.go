package cmd

import (
	"fmt"

	"github.com/antigravity/api-proxy/internal/config"
	"github.com/antigravity/api-proxy/internal/logger"
	"github.com/antigravity/api-proxy/internal/oauth"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// runLogin 执行OAuth登录流程
func runLogin(cmd *cobra.Command, args []string) error {
	// 加载配置
	cfg, err := config.LoadOrCreate()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 初始化开发模式日志（控制台输出，包含debug级别）
	log, err := logger.NewDevelopment()
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer log.Sync()

	log.Info("Debug logging enabled for OAuth flow")

	// 初始化目录
	if err := initDirectories(cfg); err != nil {
		log.Error("Failed to initialize directories", zap.Error(err))
		return err
	}

	log.Info("Starting OAuth login flow...")
	log.Info("Press Ctrl+C to cancel")

	// 创建OAuth客户端（使用server port作为回调端口）
	client := oauth.NewClient(cfg.Server.Port, cfg.Storage.AccountsDir, log)
	account, err := client.StartLoginFlow()
	if err != nil {
		log.Error("OAuth login failed", zap.Error(err))
		return err
	}

	log.Info("OAuth login successful!",
		zap.String("email", account.Email),
		zap.String("account_id", account.AccountID),
		zap.Int("models", len(account.Models)),
	)

	fmt.Println("\n✅ Login successful!")
	fmt.Printf("   Email: %s\n", account.Email)
	fmt.Printf("   Account ID: %s\n", account.AccountID)
	fmt.Printf("   Models: %d\n", len(account.Models))
	fmt.Println("\nAccount has been saved. You can now start the server:")
	fmt.Println("   ./antigravity")

	return nil
}

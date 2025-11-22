package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	OAuth    OAuthConfig    `mapstructure:"oauth"`
	Security SecurityConfig `mapstructure:"security"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Storage  StorageConfig  `mapstructure:"storage"`

	// 以下配置内置在代码中，不暴露在配置文件
	TokenRefresh TokenRefreshConfig // 始终启用，使用默认值
	RateLimit    RateLimitConfig    // 内部使用
	Monitoring   MonitoringConfig   // 内部使用
	Defaults     DefaultsConfig     // 内部使用
	Antigravity  AntigravityConfig  // 内置配置
}

type ServerConfig struct {
	Host           string        `mapstructure:"host"`
	Port           int           `mapstructure:"port"`
	Mode           string        `mapstructure:"mode"`
	ReadTimeout    time.Duration `mapstructure:"read_timeout"`
	WriteTimeout   time.Duration `mapstructure:"write_timeout"`
	MaxRequestSize string        `mapstructure:"max_request_size"`
}

type OAuthConfig struct {
	// ClientID, ClientSecret, Scopes, RedirectURL 内置在代码中，不暴露在配置文件
	// OAuth回调使用主服务器端口和 /oauth-callback 路由
}

type SecurityConfig struct {
	AdminPassword  string   `mapstructure:"admin_password"`
	APIKey         string   `mapstructure:"api_key"`
	EnableCORS     bool     `mapstructure:"enable_cors"`
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

type LoggingConfig struct {
	Level         string `mapstructure:"level"`
	Format        string `mapstructure:"format"`
	Output        string `mapstructure:"output"`
	ConsoleOutput bool   `mapstructure:"console_output"`
	MaxSize       int    `mapstructure:"max_size"`
	MaxBackups    int    `mapstructure:"max_backups"`
	MaxAge        int    `mapstructure:"max_age"`
	Compress      bool   `mapstructure:"compress"`
}

type StorageConfig struct {
	DataDir     string `mapstructure:"data_dir"`
	AccountsDir string `mapstructure:"accounts_dir"`
	KeysDir     string `mapstructure:"keys_dir"`
	UsageDir    string `mapstructure:"usage_dir"`
	LogsDir     string `mapstructure:"logs_dir"`
}

type TokenRefreshConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Interval   time.Duration `mapstructure:"interval"`
	RetryCount int           `mapstructure:"retry_count"`
	RetryDelay time.Duration `mapstructure:"retry_delay"`
}

type RateLimitConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	RequestsPerMinute int  `mapstructure:"requests_per_minute"`
	Burst             int  `mapstructure:"burst"`
}

type MonitoringConfig struct {
	Enabled     bool          `mapstructure:"enabled"`
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`
	MemoryLimit string        `mapstructure:"memory_limit"`
}

type DefaultsConfig struct {
	Temperature       float64 `mapstructure:"temperature"`
	TopP              float64 `mapstructure:"top_p"`
	TopK              int     `mapstructure:"top_k"`
	MaxTokens         int     `mapstructure:"max_tokens"`
	SystemInstruction string  `mapstructure:"system_instruction"`
}

type AntigravityConfig struct {
	BaseURL   string        `mapstructure:"base_url"`
	UserAgent string        `mapstructure:"user_agent"`
	Timeout   time.Duration `mapstructure:"timeout"`
}

// Load loads the configuration from file and environment
func Load() (*Config, error) {
	var cfg Config

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 设置默认值
	setDefaults(&cfg)

	// 验证配置
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// LoadOrCreate 加载配置，如果不存在则创建默认配置
func LoadOrCreate() (*Config, error) {
	// 检查配置文件是否真的存在
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		configFile = "./config.yaml" // 默认路径
	}

	// 检查文件是否真的存在
	if _, err := os.Stat(configFile); err == nil {
		// 文件存在，加载配置
		cfg, err := Load()
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", configFile, err)
		}
		return cfg, nil
	}

	// 配置文件不存在，创建默认配置
	fmt.Println("\n⚠️  Config file not found, creating default config...")

	cfg := &Config{}
	setDefaults(cfg)

	// 生成随机管理员密码
	password := generateRandomPassword(16)
	cfg.Security.AdminPassword = password
	fmt.Printf("\n🔑 Generated admin password: %s\n", password)
	fmt.Println("   ⚠️  IMPORTANT: Please save this password!")
	fmt.Println("   It will be needed to access the admin panel at /ui/index.html")

	// 保存配置到文件
	if err := SaveConfig(cfg); err != nil {
		fmt.Printf("\n⚠️  Warning: Failed to save config file: %v\n", err)
		fmt.Println("   Continuing with in-memory config...")
	} else {
		fmt.Println("\n✅ Config file created: config.yaml")
	}

	return cfg, nil
}

// SaveConfig 保存配置到文件
func SaveConfig(cfg *Config) error {
	// 只保存用户可配置的字段
	viper.Set("server", cfg.Server)
	viper.Set("oauth", cfg.OAuth)
	viper.Set("security", cfg.Security)
	viper.Set("logging", cfg.Logging)
	viper.Set("storage", cfg.Storage)

	// 确定配置文件路径
	configPath := viper.ConfigFileUsed()
	if configPath == "" {
		configPath = "./config.yaml"
	}

	// 写入配置文件
	return viper.WriteConfigAs(configPath)
}

// generateRandomPassword 生成随机密码
func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}

func setDefaults(cfg *Config) {
	// 服务器配置
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8045
	}
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "release"
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 * time.Second
	}

	// 日志配置
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Logging.Output == "" {
		cfg.Logging.Output = "logs/antigravity.log"
	}
	// Console output enabled by default
	cfg.Logging.ConsoleOutput = true
	if cfg.Logging.MaxSize == 0 {
		cfg.Logging.MaxSize = 100
	}
	if cfg.Logging.MaxBackups == 0 {
		cfg.Logging.MaxBackups = 10
	}
	if cfg.Logging.MaxAge == 0 {
		cfg.Logging.MaxAge = 30
	}

	// 存储配置
	if cfg.Storage.DataDir == "" {
		cfg.Storage.DataDir = "./data"
	}
	if cfg.Storage.AccountsDir == "" {
		cfg.Storage.AccountsDir = "./data/accounts"
	}
	if cfg.Storage.KeysDir == "" {
		cfg.Storage.KeysDir = "./data/keys"
	}
	if cfg.Storage.UsageDir == "" {
		cfg.Storage.UsageDir = "./data/usage"
	}
	if cfg.Storage.LogsDir == "" {
		cfg.Storage.LogsDir = "./logs"
	}

	// Token刷新配置
	if cfg.TokenRefresh.Interval == 0 {
		cfg.TokenRefresh.Interval = 30 * time.Minute
	}
	if cfg.TokenRefresh.RetryDelay == 0 {
		cfg.TokenRefresh.RetryDelay = 5 * time.Minute
	}
	if cfg.TokenRefresh.RetryCount == 0 {
		cfg.TokenRefresh.RetryCount = 3
	}

	// 监控配置
	if cfg.Monitoring.IdleTimeout == 0 {
		cfg.Monitoring.IdleTimeout = 30 * time.Second
	}

	// API默认值
	if cfg.Defaults.Temperature == 0 {
		cfg.Defaults.Temperature = 1.0
	}
	if cfg.Defaults.TopP == 0 {
		cfg.Defaults.TopP = 0.95
	}
	if cfg.Defaults.TopK == 0 {
		cfg.Defaults.TopK = 40
	}
	if cfg.Defaults.MaxTokens == 0 {
		cfg.Defaults.MaxTokens = 2048
	}

	// Antigravity API配置
	if cfg.Antigravity.BaseURL == "" {
		cfg.Antigravity.BaseURL = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	}
	if cfg.Antigravity.UserAgent == "" {
		cfg.Antigravity.UserAgent = "antigravity/1.11.3 linux/amd64"
	}
	if cfg.Antigravity.Timeout == 0 {
		cfg.Antigravity.Timeout = 60 * time.Second
	}
}

func validate(cfg *Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", cfg.Server.Port)
	}
	return nil
}

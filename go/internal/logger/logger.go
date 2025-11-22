package logger

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/antigravity/api-proxy/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// New creates a new logger instance
func New(cfg config.LoggingConfig) (*zap.Logger, error) {
	// 确保日志目录存在
	if cfg.Output != "" {
		dir := filepath.Dir(cfg.Output)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	// 日志级别
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	// 编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 选择编码器
	var encoder zapcore.Encoder
	if cfg.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 日志输出
	var writeSyncer zapcore.WriteSyncer

	if cfg.Output != "" {
		// 文件输出（带日志轮转）
		lumberjackLogger := &lumberjack.Logger{
			Filename:   cfg.Output,
			MaxSize:    cfg.MaxSize, // MB
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge, // days
			Compress:   cfg.Compress,
		}
		writeSyncer = zapcore.AddSync(lumberjackLogger)
	} else {
		// 标准输出
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// 创建 core
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// 创建 logger
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return logger, nil
}

// NewDevelopment creates a development logger (console output with color)
func NewDevelopment() (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return cfg.Build()
}

// NewProduction creates a production logger (JSON format to file)
func NewProduction(filename string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{filename}
	return cfg.Build()
}

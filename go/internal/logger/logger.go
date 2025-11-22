package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/antigravity/api-proxy/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogEntry represents a single log entry in the buffer
type LogEntry struct {
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// LogBuffer is a thread-safe circular buffer for logs
type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	limit   int
}

// GlobalBuffer is the singleton log buffer
var GlobalBuffer = &LogBuffer{
	entries: make([]LogEntry, 0, 1000),
	limit:   1000,
}

// Add adds a log entry to the buffer
func (b *LogBuffer) Add(level, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry := LogEntry{
		Level:     level,
		Message:   message,
		Timestamp: time.Now(),
	}

	b.entries = append(b.entries, entry)
	if len(b.entries) > b.limit {
		// Keep the last limit entries
		b.entries = b.entries[len(b.entries)-b.limit:]
	}
}

// GetRecent returns the recent n logs
func (b *LogBuffer) GetRecent(n int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n <= 0 || n > len(b.entries) {
		n = len(b.entries)
	}

	// Return a copy to avoid race conditions
	// Return in reverse order (newest first) if needed, or just as is.
	// JS implementation usually shows newest first? Let's check.
	// Assuming newest first is better for UI.
	result := make([]LogEntry, n)
	start := len(b.entries) - n
	copy(result, b.entries[start:])
	
	// Reverse the slice to have newest first
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	
	return result
}

// Clear clears the buffer
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = make([]LogEntry, 0, b.limit)
}

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

	// 编码器配置 - JSON格式用于文件
	jsonEncoderConfig := zapcore.EncoderConfig{
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

	// 编码器配置 - 彩色格式用于控制台
	consoleEncoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 创建编码器
	jsonEncoder := zapcore.NewJSONEncoder(jsonEncoderConfig)
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)

	// 准备cores切片
	var cores []zapcore.Core

	// 文件输出
	if cfg.Output != "" {
		lumberjackLogger := &lumberjack.Logger{
			Filename:   cfg.Output,
			MaxSize:    cfg.MaxSize, // MB
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge, // days
			Compress:   cfg.Compress,
		}
		fileWriter := zapcore.AddSync(lumberjackLogger)
		cores = append(cores, zapcore.NewCore(jsonEncoder, fileWriter, level))
	}

	// 控制台输出
	if cfg.ConsoleOutput {
		consoleWriter := zapcore.AddSync(os.Stdout)
		cores = append(cores, zapcore.NewCore(consoleEncoder, consoleWriter, level))
	}

	// 如果没有任何输出，默认使用标准输出
	if len(cores) == 0 {
		consoleWriter := zapcore.AddSync(os.Stdout)
		cores = append(cores, zapcore.NewCore(consoleEncoder, consoleWriter, level))
	}

	// 创建 Tee core (多输出)
	core := zapcore.NewTee(cores...)

	// 添加 hook 到 GlobalBuffer
	bufferHook := func(entry zapcore.Entry) error {
		GlobalBuffer.Add(entry.Level.String(), entry.Message)
		return nil
	}

	// 创建 logger
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel), zap.Hooks(bufferHook))

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

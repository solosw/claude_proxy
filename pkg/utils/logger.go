package utils

import (
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogLevel 日志级别
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// LogFormat 日志格式
type LogFormat string

const (
	LogFormatJSON    LogFormat = "json"
	LogFormatConsole LogFormat = "console"
)

// LogConfig 日志配置
type LogConfig struct {
	Level      string    // 日志级别: debug, info, warn, error
	Format     LogFormat // 日志格式: json, console
	Async      bool      // 是否异步
	FilePath   string    // 文件路径，为空则不输出到文件
	MaxSize    int       // 每个日志文件最大大小(MB)
	MaxBackups int       // 保留最多多少个备份
	MaxAge     int       // 保留最多多少天
	Compress   bool      // 是否压缩
}

// LeveledLogger 带日志级别控制的 logger，兼容原有接口
type LeveledLogger struct {
	logger *zap.SugaredLogger
	level  LogLevel
	mu     sync.RWMutex
}

// Logger 是应用使用的基础 logger
var Logger = &LeveledLogger{
	level: LogLevelInfo,
}

// 全局配置
var globalConfig LogConfig

// InitLogger 从配置初始化全局 Logger（在加载配置后调用）
// 兼容原有接口，仅设置日志级别
func InitLogger(levelStr string) {
	// 默认配置：控制台输出，文本格式，同步模式
	config := LogConfig{
		Level:      levelStr,
		Format:     LogFormatConsole,
		Async:      false,
		FilePath:   "",
		MaxSize:    10,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
	}
	InitLoggerWithConfig(config)
}

// InitLoggerWithConfig 使用完整配置初始化全局 Logger
func InitLoggerWithConfig(config LogConfig) {
	globalConfig = config

	zapConfig := zap.Config{
		Level:            zap.NewAtomicLevelAt(parseLevel(config.Level)),
		Encoding:         string(config.Format),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
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
		},
	}

	// 如果设置了文件路径，添加文件输出
	if config.FilePath != "" {
		zapConfig.OutputPaths = append(zapConfig.OutputPaths, config.FilePath)

		// 设置文件滚动
		_ = zap.RegisterEncoder("rotated", func(cfg zapcore.EncoderConfig) (zapcore.Encoder, error) {
			return zapcore.NewJSONEncoder(cfg), nil
		})
	}

	var logger *zap.Logger
	var err error

	if config.Async {
		// 异步模式
		logger, err = zapConfig.Build()
		if err == nil {
			logger = logger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
				return zapcore.NewCore(
					zapcore.NewJSONEncoder(zapConfig.EncoderConfig),
					zapcore.AddSync(&lumberjack.Logger{
						Filename:   config.FilePath,
						MaxSize:    config.MaxSize,
						MaxBackups: config.MaxBackups,
						MaxAge:     config.MaxAge,
						Compress:   config.Compress,
					}),
					zapcore.Level(parseLevel(config.Level)),
				)
			}))
		}
	} else {
		// 同步模式：使用 lumberjack 进行文件轮转
		encoder := zapcore.NewJSONEncoder(zapConfig.EncoderConfig)
		if config.Format == LogFormatConsole {
			encoder = zapcore.NewConsoleEncoder(zapConfig.EncoderConfig)
		}

		var writeSyncer zapcore.WriteSyncer
		if config.FilePath != "" {
			writeSyncer = zapcore.NewMultiWriteSyncer(
				zapcore.AddSync(os.Stdout),
				zapcore.AddSync(&lumberjack.Logger{
					Filename:   config.FilePath,
					MaxSize:    config.MaxSize,
					MaxBackups: config.MaxBackups,
					MaxAge:     config.MaxAge,
					Compress:   config.Compress,
				}),
			)
		} else {
			writeSyncer = zapcore.AddSync(os.Stdout)
		}

		core := zapcore.NewCore(
			encoder,
			writeSyncer,
			zapcore.Level(parseLevel(config.Level)),
		)
		logger = zap.New(core)
	}

	if err != nil {
		// 如果构建失败，回退到默认
		logger, _ = zap.NewProduction()
	}

	Logger.mu.Lock()
	Logger.logger = logger.Sugar()
	Logger.level = parseLogLevel(config.Level)
	Logger.mu.Unlock()
}

// parseLogLevel 解析日志级别字符串
func parseLogLevel(levelStr string) LogLevel {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "warn", "warning":
		return LogLevelWarn
	case "error":
		return LogLevelError
	default:
		return LogLevelInfo
	}
}

// parseLevel 解析 Zap 日志级别
func parseLevel(levelStr string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// SetLevel 设置日志级别
func (l *LeveledLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	l.level = level
	l.mu.Unlock()
}

// Printf 兼容原有的 Printf 接口，默认为 Info 级别
func (l *LeveledLogger) Printf(format string, v ...interface{}) {
	l.Infof(format, v...)
}

// Debugf 输出 Debug 级别日志
func (l *LeveledLogger) Debugf(format string, v ...interface{}) {
	l.mu.RLock()
	level := l.level
	logger := l.logger
	l.mu.RUnlock()

	if level <= LogLevelDebug {
		logger.Debugf(format, v...)
	}
}

// Infof 输出 Info 级别日志
func (l *LeveledLogger) Infof(format string, v ...interface{}) {
	l.mu.RLock()
	level := l.level
	logger := l.logger
	l.mu.RUnlock()

	if level <= LogLevelInfo {
		logger.Infof(format, v...)
	}
}

// Warnf 输出 Warn 级别日志
func (l *LeveledLogger) Warnf(format string, v ...interface{}) {
	l.mu.RLock()
	level := l.level
	logger := l.logger
	l.mu.RUnlock()

	if level <= LogLevelWarn {
		logger.Warnf(format, v...)
	}
}

// Errorf 输出 Error 级别日志
func (l *LeveledLogger) Errorf(format string, v ...interface{}) {
	l.mu.RLock()
	level := l.level
	logger := l.logger
	l.mu.RUnlock()

	if level <= LogLevelError {
		logger.Errorf(format, v...)
	}
}

// Sync 同步日志缓冲区（异步模式下调用）
func (l *LeveledLogger) Sync() {
	l.mu.RLock()
	logger := l.logger
	l.mu.RUnlock()
	if logger != nil {
		_ = logger.Sync()
	}
}

// GetConfig 返回当前日志配置
func GetConfig() LogConfig {
	return globalConfig
}

// NewLeveledLogger 创建一个新的分级日志记录器（兼容原有接口）
func NewLeveledLogger(levelStr string) *LeveledLogger {
	logger := &LeveledLogger{
		level: parseLogLevel(levelStr),
	}

	// 初始化 Zap
	zapConfig := zap.Config{
		Level:            zap.NewAtomicLevelAt(parseLevel(levelStr)),
		Encoding:         string(LogFormatConsole),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
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
		},
	}

	zapLogger, _ := zapConfig.Build()
	logger.logger = zapLogger.Sugar()

	return logger
}

// WithFields 创建带有额外字段的日志记录器
func (l *LeveledLogger) WithFields(fields map[string]interface{}) *zap.SugaredLogger {
	l.mu.RLock()
	logger := l.logger
	l.mu.RUnlock()

	if logger == nil {
		return nil
	}

	// 将 map 转换为键值对切片
	pairs := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		pairs = append(pairs, k, v)
	}
	return logger.With(pairs...)
}

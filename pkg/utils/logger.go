package utils

import (
	"log"
	"os"
	"strings"
)

// ANSI 颜色代码
const (
	colorReset  = "\033[0m"
	colorGray   = "\033[90m"  // Debug - 灰色
	colorCyan   = "\033[36m"  // Info - 青色
	colorYellow = "\033[33m"  // Warn - 黄色
	colorRed    = "\033[31m"  // Error - 红色
)

// LogLevel 日志级别
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// LeveledLogger 带日志级别控制的 logger
type LeveledLogger struct {
	logger      *log.Logger
	level       LogLevel
	enableColor bool
}

// Logger 是应用使用的基础 logger，输出到标准输出，带时间和短文件名。
var Logger = NewLeveledLogger("info")

// NewLeveledLogger 创建一个新的分级日志记录器
func NewLeveledLogger(levelStr string) *LeveledLogger {
	level := parseLogLevel(levelStr)

	// 检测是否支持颜色（默认启用，除非明确禁用）
	enableColor := true
	if noColor := os.Getenv("NO_COLOR"); noColor != "" {
		enableColor = false
	}

	return &LeveledLogger{
		logger:      log.New(os.Stdout, "[ClaudeRouter] ", log.LstdFlags|log.Lshortfile),
		level:       level,
		enableColor: enableColor,
	}
}

// InitLogger 从配置初始化全局 Logger（在加载配置后调用）
func InitLogger(levelStr string) {
	Logger = NewLeveledLogger(levelStr)
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
		return LogLevelInfo // 默认 Info 级别
	}
}

// SetLevel 设置日志级别
func (l *LeveledLogger) SetLevel(level LogLevel) {
	l.level = level
}

// Printf 兼容原有的 Printf 接口，默认为 Info 级别
func (l *LeveledLogger) Printf(format string, v ...interface{}) {
	l.Infof(format, v...)
}

// Debugf 输出 Debug 级别日志
func (l *LeveledLogger) Debugf(format string, v ...interface{}) {
	if l.level <= LogLevelDebug {
		prefix := l.colorize(colorGray, "[DEBUG]")
		l.logger.Printf(prefix+" "+format, v...)
	}
}

// Infof 输出 Info 级别日志
func (l *LeveledLogger) Infof(format string, v ...interface{}) {
	if l.level <= LogLevelInfo {
		prefix := l.colorize(colorCyan, "[INFO]")
		l.logger.Printf(prefix+" "+format, v...)
	}
}

// Warnf 输出 Warn 级别日志
func (l *LeveledLogger) Warnf(format string, v ...interface{}) {
	if l.level <= LogLevelWarn {
		prefix := l.colorize(colorYellow, "[WARN]")
		l.logger.Printf(prefix+" "+format, v...)
	}
}

// Errorf 输出 Error 级别日志
func (l *LeveledLogger) Errorf(format string, v ...interface{}) {
	if l.level <= LogLevelError {
		prefix := l.colorize(colorRed, "[ERROR]")
		l.logger.Printf(prefix+" "+format, v...)
	}
}

// colorize 给文本添加颜色
func (l *LeveledLogger) colorize(color, text string) string {
	if !l.enableColor {
		return text
	}
	return color + text + colorReset
}

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// OperatorEndpoint 运营商转发所需端点配置（仅配置层使用，不暴露给 API）。
type OperatorEndpoint struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Enabled     bool   `yaml:"enabled"`
	BaseURL     string `yaml:"base_url"`
	APIKey      string `yaml:"api_key"`
	Interface   string `yaml:"interface_type"`
}

// Config 定义了应用的顶层配置结构，对应 configs/config.yaml。
type Config struct {
	Server struct {
		Addr  string `yaml:"addr"`
		Debug bool   `yaml:"debug"`
	} `yaml:"server"`

	Log struct {
		Level      string `yaml:"level"`
		Format     string `yaml:"format"`      // json, console
		Async      bool   `yaml:"async"`       // 是否异步
		FilePath   string `yaml:"file_path"`   // 文件路径
		MaxSize    int    `yaml:"max_size"`    // 每个日志文件大小(MB)
		MaxBackups int    `yaml:"max_backups"` // 保留备份数
		MaxAge     int    `yaml:"max_age"`     // 保留天数
		Compress   bool   `yaml:"compress"`    // 是否压缩
	} `yaml:"log"`

	Database struct {
		Driver string `yaml:"driver"`
		DSN    string `yaml:"dsn"`
	} `yaml:"database"`

	Auth struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"auth"`

	GUI struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"gui"`

	// Tasks 定时任务相关配置（可选）。
	Tasks struct {
		ErrorLogCleanup struct {
			Enabled   *bool  `yaml:"enabled"`
			Interval  string `yaml:"interval"`  // 执行间隔，例如 30m
			Retention string `yaml:"retention"` // 保留时长，例如 12h
		} `yaml:"error_log_cleanup"`

		ComboWeight struct {
			Enabled           *bool    `yaml:"enabled"`
			Interval          string   `yaml:"interval"`             // 执行间隔，例如 30m
			Window            string   `yaml:"window"`               // 统计窗口，例如 6h
			MinErrorsToAdjust int64    `yaml:"min_errors_to_adjust"` // 触发阈值（总错误数）
			LR                float64  `yaml:"lr"`                   // 平滑系数(0~1)
			MinWeight         float64  `yaml:"min_weight"`
			Normalize         *bool    `yaml:"normalize"`
			MaxStep           float64  `yaml:"max_step"`             // 单次最大权重变化
			// 严重错误（429/404/403/400）的惩罚倍率，默认 3.0；其他错误倍率默认 0.3
			SevereErrorWeight float64  `yaml:"severe_error_weight"` // 429/404/403/400 错误的权重倍率
			MildErrorWeight   float64  `yaml:"mild_error_weight"`   // 其他错误的权重倍率
		} `yaml:"combo_weight"`
	} `yaml:"tasks"`

	// Operators 系统内置运营商，key 为运营商 ID，选择运营商即使用此处配置的转发逻辑（BaseURL/APIKey/Interface）。
	Operators map[string]OperatorEndpoint `yaml:"operators"`
}

// Load 从给定路径加载 YAML 配置，并返回 Config。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return &cfg, nil
}

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
		Addr string `yaml:"addr"`
	} `yaml:"server"`

	Log struct {
		Level string `yaml:"level"`
	} `yaml:"log"`

	Database struct {
		Driver string `yaml:"driver"`
		DSN    string `yaml:"dsn"`
	} `yaml:"database"`

	Auth struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"auth"`

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


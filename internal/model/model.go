package model

import "time"

// Model 表示一个可用的底层模型（OpenAI / Anthropic 等），可被直接调用或被组合模型引用。
type Model struct {
	ID          string `json:"id" gorm:"primaryKey"` // 唯一 ID，例如 "openai:gpt-4.1"
	Name        string `json:"name"`                 // 展示名，例如 "GPT-4.1"
	Provider    string `json:"provider"`             // 对应 providers 配置中的 key，例如 "openai" / "anthropic"
	Interface   string `json:"interface_type"`       // 接口类型：如 openai、anthropic、openai_compatible 等
	UpstreamID  string `json:"upstream_id"`          // 提供商实际的 model 名称，例如 "gpt-4.1" / "claude-3-5-sonnet-20241022"
	APIKey      string `json:"api_key"`              // 上游模型服务商的专用 API Key（覆盖 ProviderConfig.api_key）
	BaseURL     string `json:"base_url"`             // 可选：为该模型单独设置上游 BaseURL，优先级高于 Provider 的 BaseURL
	Description string `json:"description"`          // 描述
	Enabled     bool   `json:"enabled"`              // 是否启用

	// 扩展字段是否转发到上游（Claude Code 会带 metadata、thinking 等，部分上游如 ModelScope 不支持则关闭）
	ForwardMetadata bool `json:"forward_metadata"` // 是否将请求中的 metadata 转发到上游
	ForwardThinking bool `json:"forward_thinking"` // 是否将 thinking（extended thinking）转发到上游

	// 该模型最大 QPS，0 表示不限制
	MaxQPS float64 `json:"max_qps"`

	// 若不为空，表示该模型归属该运营商，请求走运营商专属 API（BaseURL、APIKey 以运营商为准）
	OperatorID string `json:"operator_id"`

	// ResponseFormat 响应格式类型：anthropic（默认）、openai（OpenAI Chat Completion）、openai_responses（OpenAI Responses API）
	ResponseFormat string `json:"response_format"`

	// 输入/输出 token 单价（单位：元/千 token）
	InputPrice  float64 `json:"input_price" gorm:"not null;default:0"`
	OutputPrice float64 `json:"output_price" gorm:"not null;default:0"`
}

// User 平台用户。
type User struct {
	Username     string     `json:"username" gorm:"primaryKey;size:100"`
	APIKey       string     `json:"api_key" gorm:"uniqueIndex;size:255;not null"`
	Quota        int64      `json:"quota" gorm:"not null;default:-1"` // -1 表示无限
	ExpireAt     *time.Time `json:"expire_at"`
	IsAdmin      bool       `json:"is_admin" gorm:"not null;default:false"`
	InputTokens  int64      `json:"input_tokens" gorm:"not null;default:0"`
	OutputTokens int64      `json:"output_tokens" gorm:"not null;default:0"`
	TotalTokens  int64      `json:"total_tokens" gorm:"not null;default:0"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// UsageLog 记录每次请求的 token 使用详情。
type UsageLog struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Provider     string    `json:"provider" gorm:"primaryKey;size:100"`
	Username     string    `json:"username" gorm:"index;size:100;not null"`
	ModelID      string    `json:"model_id" gorm:"index;size:100;not null"`
	InputTokens  int64     `json:"input_tokens" gorm:"not null;default:0"`
	OutputTokens int64     `json:"output_tokens" gorm:"not null;default:0"`
	InputPrice   float64   `json:"input_price" gorm:"not null;default:0"`  // 输入单价（元/千 token）
	OutputPrice  float64   `json:"output_price" gorm:"not null;default:0"` // 输出单价（元/千 token）
	TotalCost    float64   `json:"total_cost" gorm:"not null;default:0"`   // 总费用（元）
	CreatedAt    time.Time `json:"created_at" gorm:"index"`
}

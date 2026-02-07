package messages

import (
	"context"
	"io"
)

// OperatorStrategy 运营商转发策略：每个运营商一套独立逻辑，与 openai/anthropic 适配器区分开。
// 由各 operator_xxx.go 实现并注册到 OperatorRegistry。
type OperatorStrategy interface {
	// Execute 使用该运营商的转发逻辑完成请求，返回与 Adapter 相同。
	Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error)
}

// OperatorRegistry 按运营商 ID 获取策略。
var OperatorRegistry = NewOperatorRegistryMap()

// OperatorRegistryMap 维护 operator_id -> OperatorStrategy 的映射。
type OperatorRegistryMap struct {
	strategies map[string]OperatorStrategy
}

// NewOperatorRegistryMap 创建运营商策略注册表。
func NewOperatorRegistryMap() *OperatorRegistryMap {
	return &OperatorRegistryMap{
		strategies: make(map[string]OperatorStrategy),
	}
}

// Register 注册运营商 ID 对应的策略。
func (r *OperatorRegistryMap) Register(operatorID string, s OperatorStrategy) {
	if r.strategies == nil {
		r.strategies = make(map[string]OperatorStrategy)
	}
	r.strategies[operatorID] = s
}

// Get 获取运营商策略，未注册返回 nil。
func (r *OperatorRegistryMap) Get(operatorID string) OperatorStrategy {
	return r.strategies[operatorID]
}

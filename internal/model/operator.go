package model

// Operator 系统内置运营商（仅用于 API 展示，列表来自配置，不可增删改）。
// 转发时的 BaseURL/APIKey/Interface 由配置层提供，不在此结构内。
type Operator struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

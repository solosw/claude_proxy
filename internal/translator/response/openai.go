package response

import "awesomeProject/internal/provider"

// ToOpenAIChatCompletionResponse 目前内部结构已经与 OpenAI Chat Completions 响应兼容，
// 这里直接返回同一个结构，保留扩展点，后续如需字段映射可在此集中处理。
func ToOpenAIChatCompletionResponse(resp *provider.ChatResponse) *provider.ChatResponse {
	return resp
}


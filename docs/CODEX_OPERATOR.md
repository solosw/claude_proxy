# Codex 运营商配置指南

## 概述

Codex 运营商允许 ClaudeRouter 对接 OpenAI Codex Responses API，将 Anthropic Messages API 格式自动转换为 Codex 格式。

Codex API 是 OpenAI 用于 Codex Desktop 客户端的内部 API，提供了强大的代码生成和编程辅助能力。

## 参考项目

本实现参考了 [codex-proxy](https://github.com/icebear0828/codex-proxy) 项目的消息格式转换逻辑。

## 配置示例

### 1. 配置运营商

在 `config.yaml` 中添加 Codex 运营商配置：

```yaml
operators:
  codex:
    enabled: true
    base_url: "https://chatgpt.com/backend-api"  # Codex API 基础 URL
    api_key: "your-codex-token"                    # Codex 访问令牌
    interface: "codex"                              # 使用 codex 接口类型
```

### 2. 配置模型

添加使用 Codex 运营商的模型：

```yaml
models:
  - id: "codex-5.3"
    display_name: "Codex 5.3"
    enabled: true
    operator_id: "codex"                    # 绑定到 codex 运营商
    upstream_id: "gpt-5.3-codex"            # 上游模型 ID
    interface: ""                            # 留空，使用运营商的接口类型
    base_url: ""                             # 留空，使用运营商的 base_url
    api_key: ""                              # 留空，使用运营商的 api_key
    max_qps: 0                               # 不限制 QPS
    forward_metadata: true                   # 转发 metadata
    forward_thinking: true                   # 转发 thinking

  - id: "codex-max"
    display_name: "Codex Max"
    enabled: true
    operator_id: "codex"
    upstream_id: "gpt-5.1-codex-max"        # 深度推理编程模型
    max_qps: 0

  - id: "codex-mini"
    display_name: "Codex Mini"
    enabled: true
    operator_id: "codex"
    upstream_id: "gpt-5.1-codex-mini"       # 轻量快速编程模型
    max_qps: 0
```

### 3. 配置组合模型（可选）

可以创建组合模型来智能选择不同的 Codex 模型：

```yaml
combos:
  - id: "codex-auto"
    display_name: "Codex Auto"
    enabled: true
    items:
      - model_id: "codex-mini"
        weight: 1
        input_tokens_limit: 10000    # 短对话使用 mini
      - model_id: "codex-5.3"
        weight: 1
        input_tokens_limit: 50000    # 中等对话使用 5.3
      - model_id: "codex-max"
        weight: 1
        input_tokens_limit: 0        # 长对话使用 max
```

## 可用的 Codex 模型

根据 codex-proxy 项目，以下模型可用：

| 模型 ID | 别名 | 说明 |
|---------|------|------|
| `gpt-5.3-codex` | `codex` | 最新旗舰 agentic 编程模型（默认） |
| `gpt-5.2-codex` | - | 上一代 agentic 编程模型 |
| `gpt-5.1-codex-max` | `codex-max` | 深度推理编程模型 |
| `gpt-5.2` | - | 通用旗舰模型 |
| `gpt-5.1-codex-mini` | `codex-mini` | 轻量快速编程模型 |

## 消息格式转换

### Anthropic → Codex 转换规则

1. **System 消息** → `instructions` 字段
   - Anthropic 的 `system` 参数转换为 Codex 的 `instructions`
   - 如果没有 system 消息，使用默认值 "You are a helpful assistant."

2. **Messages 数组** → `input` 数组
   - `user` 消息 → `{role: "user", content: "..."}`
   - `assistant` 消息 → `{role: "assistant", content: "..."}`
   - `system` 消息会被跳过（已放入 instructions）

3. **内容提取**
   - 字符串内容直接使用
   - 数组内容提取所有 `type: "text"` 的文本块并用换行符连接

4. **工具调用处理（重要！）**
   
   **A. Tools 定义格式转换**
   
   Anthropic 和 Codex 使用不同的工具定义格式，ClaudeRouter 会自动转换：
   
   ```json
   // Anthropic 格式
   {
     "name": "get_weather",
     "description": "Get current weather",
     "input_schema": {
       "type": "object",
       "properties": {
         "location": {"type": "string"}
       },
       "required": ["location"]
     }
   }
   
   // 转换为 OpenAI/Codex 格式
   {
     "type": "function",
     "function": {
       "name": "get_weather",
       "description": "Get current weather",
       "parameters": {
         "type": "object",
         "properties": {
           "location": {"type": "string"}
         },
         "required": ["location"]
       }
     }
   }
   ```
   
   **关键转换**：
   - 添加 `type: "function"` 包装
   - `input_schema` → `parameters`
   - 嵌套在 `function` 对象中
   
   **B. Tool Use/Result 内容展平**
   
   消息中的 `tool_use` 和 `tool_result` 块会被展平为文本格式：
   
   **tool_use 块** → 文本格式：
   ```
   [Tool Call: function_name({"arg1": "value1", "arg2": "value2"})]
   ```
   
   **tool_result 块** → 文本格式：
   ```
   [Tool Result (tool_use_id)]: result content
   ```
   
   **错误的 tool_result** → 文本格式：
   ```
   [Tool Error (tool_use_id)]: error message
   ```
   
   示例转换：
   ```json
   // Anthropic 格式
   {
     "role": "assistant",
     "content": [
       {"type": "text", "text": "Let me search for that."},
       {"type": "tool_use", "id": "call_123", "name": "search", "input": {"query": "golang"}}
     ]
   }
   
   // 转换为 Codex 格式
   {
     "role": "assistant",
     "content": "Let me search for that.\n[Tool Call: search({\"query\":\"golang\"})]"
   }
   ```

5. **Thinking/Reasoning**
   - 如果 Anthropic 请求包含 `thinking` 参数，会根据 `budget_tokens` 自动设置 Codex 的 `reasoning.effort`：
     - `budget_tokens > 10000` → `effort: "high"`
     - `budget_tokens >= 5000` → `effort: "medium"`
     - `budget_tokens < 5000` → `effort: "low"`

### Codex → Anthropic 转换规则

#### 流式响应（SSE）

Codex SSE 事件 → Anthropic SSE 事件：

| Codex 事件 | Anthropic 事件 | 说明 |
|-----------|---------------|------|
| `response.created` | `message_start` | 消息开始 |
| `response.created` | `content_block_start` | 内容块开始 |
| `response.output_text.delta` | `content_block_delta` | 文本增量 |
| `response.completed` | `content_block_stop` | 内容块结束 |
| `response.completed` | `message_delta` | 消息元数据（stop_reason, usage） |
| `response.completed` | `message_stop` | 消息结束 |

#### 非流式响应

收集所有 SSE 事件，构建完整的 Anthropic Message 对象：

```json
{
  "id": "msg-xxx",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "完整的响应文本"
    }
  ],
  "model": "claude-3-5-sonnet-20241022",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 100,
    "output_tokens": 200
  }
}
```

## 获取 Codex Token

要使用 Codex API，你需要一个有效的 ChatGPT 账号和 Token。

### 方法 1：使用 codex-proxy 获取

1. 克隆并运行 codex-proxy：
   ```bash
   git clone https://github.com/icebear0828/codex-proxy.git
   cd codex-proxy
   npm install
   npm run dev
   ```

2. 打开 `http://localhost:8080` 使用 ChatGPT 账号登录

3. 登录后，在控制面板找到你的 Token

### 方法 2：手动提取

1. 登录 https://chatgpt.com
2. 打开浏览器开发者工具（F12）
3. 切换到 Application/Storage → Cookies
4. 找到 `__Secure-next-auth.session-token` 的值
5. 这就是你的 Token

## 客户端配置

### Claude Code / Cursor

设置 API 地址：
```
http://localhost:8080/v1/messages
```

选择模型：
```
codex-5.3
```

### 使用组合模型

如果配置了组合模型 `codex-auto`，客户端可以直接使用：
```
codex-auto
```

ClaudeRouter 会根据对话长度自动选择最合适的 Codex 模型。

## 注意事项

1. **Token 有效期**：Codex Token 会定期过期，需要重新登录获取

2. **速率限制**：Codex API 有使用配额限制，请合理使用

3. **仅流式输出**：Codex API 原生仅支持流式输出，非流式响应由 ClaudeRouter 内部收集完整后返回

4. **模型列表**：Codex 模型列表会随官方更新变化，请参考最新的 codex-proxy 文档

5. **合法使用**：请遵守 OpenAI 的服务条款，仅用于个人学习和研究

6. **工具调用限制**：
   - ⚠️ **重要**：Codex API **不返回结构化的 tool_use 响应**
   - 工具相关内容会被展平为纯文本格式
   - 如果你的应用需要解析工具调用，需要自行从文本中提取 `[Tool Call: ...]` 格式
   - 建议对需要工具调用的场景使用原生支持 tools 的模型（如 Claude）

## 故障排查

### 问题：401 Unauthorized

**原因**：Token 无效或已过期

**解决**：
1. 重新获取 Token
2. 更新 `config.yaml` 中的 `api_key`
3. 重启 ClaudeRouter

### 问题：响应格式错误

**原因**：上游 API 变化或消息格式不兼容

**解决**：
1. 检查 ClaudeRouter 日志中的 `payload_to_send` 输出
2. 对比 codex-proxy 项目的最新实现
3. 提交 Issue 报告

### 问题：连接超时

**原因**：网络问题或 Codex API 不可用

**解决**：
1. 检查网络连接
2. 确认 `base_url` 配置正确
3. 尝试使用代理访问

## 开发与调试

### 查看请求日志

ClaudeRouter 会输出详细的转换日志：

```
[ClaudeRouter] operator codex: start, stream=true, baseURL=https://chatgpt.com/backend-api
[ClaudeRouter] operator codex: payload_to_send={"model":"gpt-5.3-codex","instructions":"...","input":[...],"stream":true,"store":false}
[ClaudeRouter] operator codex: upstream response status=200 contentType=text/event-stream
```

### 测试转换逻辑

可以使用 curl 直接测试：

```bash
curl http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: your-api-key" \
  -d '{
    "model": "codex-5.3",
    "messages": [
      {"role": "user", "content": "写一个 Python 快速排序"}
    ],
    "max_tokens": 1024,
    "stream": true
  }'
```

## 相关链接

- [codex-proxy 项目](https://github.com/icebear0828/codex-proxy)
- [Anthropic Messages API 文档](https://docs.anthropic.com/en/api/messages)
- [ClaudeRouter 配置说明](../README.md)

## 贡献

如果你发现 Codex API 格式变化或有改进建议，欢迎提交 PR：

1. Fork 本项目
2. 修改 `internal/translator/messages/operator_codex.go`
3. 测试转换逻辑
4. 提交 Pull Request

## 许可

本实现参考了 codex-proxy 项目，遵循相应的开源许可。

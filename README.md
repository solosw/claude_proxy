# ClaudeRouter

ClaudeRouter 是一个多模型反代网关。
它把不同上游协议（Anthropic(claude) / OpenAI Chat / OpenAI Responses （Codex））统一到本地接口，重点解决“同一客户端如何切换不同反代源”。

## 目前支持
- 模型组合，根据关键词自动筛选模型
- 支持claude->OpenAI Chat/OpenAI Responses （Codex）/Anthropic(claude)
- 支持 codex->OpenAI Responses （Codex）
## todo
- 智能切换
- 限流
- codex->（Anthropic(claude)/OpenAI Chat
## 反代支持矩阵（重点）

### 1) 按 `interface_type` 反代（无 `operator_id` 时生效）

| interface_type | 主要入口 | 上游目标 | 转换方式 |
|---|---|---|---|
| `anthropic` | `POST /back/v1/messages` | `.../v1/messages` | 直连 Anthropic 协议 |
| `openai` | `POST /back/v1/messages` | `.../v1/chat/completions` | Anthropic Messages <-> OpenAI Chat（go-openai） |
| `openai_compatible` | `POST /back/v1/messages` | `.../v1/chat/completions` | Anthropic Messages <-> OpenAI Chat（CLIProxyAPI SDK translator） |
| `openai_responses` / `openai_response` | `POST /back/v1/messages`、`POST /back/v1/responses` | `.../v1/responses` | Messages 路径做 Anthropic <-> Responses；Responses 路径直通 |

说明：
- 现在 `openai` 和 `openai_compatible` 已分开实现。
- `openai_compatible` 在 `messages` 路径下走 SDK translator，而不是旧的手写转换。

### 2) 按 `operator_id` 反代（优先级高于 `interface_type`）

| operator_id | 主要用途 | 上游目标 | 特点 |
|---|---|---|---|
| `codex` | Codex / Responses 线路 | `.../v1/responses` | `messages` 路径走 SDK translator（Claude <-> Codex）；`responses` 路径支持直通与适配 |
| `minimax` / `glm` / `kimi` / `proxy` | Anthropic 风格反代 | `.../v1/messages` | HTTP 直通，替换模型与鉴权 |
| `iflow` | iFlow 网关 | `.../v1/chat/completions` | 复用 `openai_compatible` 适配流程，默认 `https://apis.iflow.cn` |
| `newapi` | NewAPI 网关 | `.../v1/chat/completions` | 使用 NewAPI 专用适配器 |

## 路由与选路规则

### `/back/v1/messages`

1. 先解析 `model`（支持 Combo）。
2. 若带 `metadata.user_id`，命中会话缓存后复用已选模型（TTL 10 分钟）。
3. 若目标模型配置了 `operator_id`，走运营商策略。
4. 否则按 `interface_type` 走协议适配器。

### `/back/v1/responses`

1. 先解析 `model`（同样支持 Combo + 会话缓存）。
2. 若模型是 `operator_id=codex` 或 `interface_type=openai_responses/openai_response`，走 Responses 直通路径。
3. 若模型是 `interface_type=openai/openai_compatible`，先做 Responses <-> Chat 适配，再请求 `.../v1/chat/completions`。

## 对外接口

- Anthropic 兼容
  - `POST /back/v1/messages`
  - `POST /back/v1/messages/count_tokens`
- OpenAI Chat 兼容
  - `POST /back/v1/chat/completions`
- OpenAI Responses 兼容
  - `POST /back/v1/responses`
- 健康检查
  - `GET /healthz`

`/v1/*`（无 `/back`）路由也已注册，便于本地直连调试。

## 配置示例（反代相关）

### `configs/config.yaml`

```yaml
server:
  addr: "localhost:8090"

auth:
  api_key: "your-global-api-key"

operators:
  codex:
    enabled: true
    base_url: "https://chatgpt.com/backend-api"
    api_key: "your-codex-token"
    interface_type: "openai_compatible"

  minimax:
    enabled: true
    base_url: ""
    api_key: ""
    interface_type: "anthropic"

  iflow:
    enabled: true
    base_url: "https://apis.iflow.cn"
    api_key: ""
    interface_type: "openai_compatible"

  newapi:
    enabled: true
    base_url: "https://api.newapi.pro"
    api_key: ""
    interface_type: "openai_compatible"
```

### 模型配置建议

- 走运营商专线：给模型设置 `operator_id`。
- 走通用协议反代：设置 `interface_type` + `base_url` + `api_key`。
- 建议总是设置 `upstream_id`（真实上游模型名）。

## 快速开始

### 环境

- Go 1.26+
- Node.js 18+（仅前端开发需要）

### 启动

```bash
go run ./cmd/server
```

默认地址：`http://localhost:8090`

## 调用示例

### Anthropic Messages

```bash
curl -X POST "http://localhost:8090/back/v1/messages" \
  -H "Authorization: Bearer your-global-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-combo-or-model-id",
    "messages": [{"role":"user","content":"Hello"}],
    "stream": false
  }'
```

### OpenAI Responses

```bash
curl -X POST "http://localhost:8090/back/v1/responses" \
  -H "Authorization: Bearer your-global-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-model-id",
    "input": "Hello",
    "stream": false
  }'
```

## 常见问题（反代方向）

- 404（上游）
  - 常见原因是 `base_url` 与目标协议不匹配（例如把 Chat 地址用于 Responses）。
  - 先确认模型走的是 `operator_id` 还是 `interface_type`。
- 400 `invalid role` / `function` 参数错误
  - 通常是协议转换前后字段不兼容。
  - 建议优先使用 `openai_compatible`（SDK translator 路径）。
- 同会话模型未切换
  - 检查是否传了 `metadata.user_id`；命中会话缓存会复用模型。

## 开发与排查

```bash
go test ./... -v
go vet ./...
```

重点日志前缀：
- `messages: step=execute_call ...`
- `responses: step=...`

可用于确认当前请求命中了哪条反代链路（operator / adapter / sdk translator）。

## 目录

```text
cmd/
  server/main.go
internal/
  handler/
  translator/messages/
  model/
  config/
configs/
  config.yaml
public/web/
front/
```

## 备注

- 当前 `go.mod` 模块名为 `awesomeProject`。
- 项目目标是反代与协议统一，不绑定某单一上游厂商。

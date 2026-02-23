# ClaudeRouter

ClaudeRouter 是一个用 Go 实现的多模型网关，统一对外提供兼容 Anthropic Messages API 和 OpenAI Chat Completions API 的入口，并附带一个 Vue 3 管理后台用于维护模型、组合模型与运营商策略。

## 核心能力

- 统一入口：
  - `POST /back/v1/messages`（Anthropic 风格）
  - `POST /back/v1/chat/completions`（OpenAI 风格）
- 支持多种上游接口类型：
  - `anthropic`
  - `openai`
  - `openai_compatible`
  - `openai_responses`
- 模型管理：
  - 模型 CRUD（数据库持久化）
  - 按模型配置 API Key、Base URL、QPS、扩展字段转发策略
- 组合模型（Combo）：
  - 对外暴露一个虚拟模型 ID
  - 根据关键词规则选择具体子模型
  - 对话级模型缓存（按 `metadata.user_id`）
- 运营商策略（Operator）：
  - Minimax/GLM/Kimi/Proxy 直通策略
  - iFlow / NewAPI / Codex 专用转发策略
- 流式支持：
  - SSE 代理
  - Anthropic/OpenAI/OpenAI Responses 的格式转换
- 可视化后台：
  - 登录、模型管理、组合模型管理、运营商查看、接口测试

## 目录结构

```text
cmd/
  server/main.go          # 服务入口（API + 静态资源 + WebView2）
  gui/main.go             # 备用 GUI 启动方式（lorca）
configs/
  config.yaml             # 主配置文件
internal/
  config/                 # 配置加载
  handler/                # HTTP 处理器与路由
  middleware/             # API Key 鉴权
  model/                  # GORM 模型与存储逻辑
  storage/                # DB 初始化（SQLite）
  combo/                  # 组合模型路由选择逻辑
  provider/               # OpenAI/Anthropic provider 封装
  translator/             # 协议转换（messages/request/response）
front/                    # Vue 3 前端源码
public/web/               # 前端构建产物（供后端静态托管）
```

## 技术栈

- 后端：Go、Gin、GORM、SQLite
- 前端：Vue 3、Vite、Element Plus、Axios
- 协议 SDK：
  - `github.com/anthropics/anthropic-sdk-go`
  - `github.com/sashabaranov/go-openai`
  - `github.com/openai/openai-go/v3`

## 快速开始

### 1) 环境要求

- Go 1.21+
- Node.js 18+（仅前端开发或重建前端时需要）

### 2) 配置

编辑 `configs/config.yaml`：

```yaml
server:
  addr: "localhost:8090"

database:
  driver: sqlite
  dsn: "./data/claude_router.db"

auth:
  api_key: "your-global-api-key"

operators:
  minimax:
    enabled: true
    base_url: ""
    api_key: ""
    interface_type: "anthropic"
```

### 3) 启动后端

```bash
go run ./cmd/server
```

默认监听 `http://localhost:8090`。

可访问：

- 管理后台：`/`
- 健康检查：`/healthz`
- API 前缀：`/back`

## API 概览

### 鉴权

受保护接口默认使用全局 API Key，支持以下 Header：

- `Authorization: Bearer <API_KEY>`
- `X-API-Key: <API_KEY>`
- `token: <API_KEY>`

### 核心接口

- Anthropic 兼容：
  - `POST /back/v1/messages`
  - `POST /back/v1/messages/count_tokens`（当前为占位实现）
- OpenAI 兼容：
  - `POST /back/v1/chat/completions`
- 管理接口：
  - `GET/POST/PUT/DELETE /back/api/models`
  - `GET/POST/PUT/DELETE /back/api/combos`
  - `GET /back/api/operators`
  - `GET /back/api/operators/:id`
  - `POST /back/api/chat/test`

### 调用示例

Anthropic 风格：

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

OpenAI 风格：

```bash
curl -X POST "http://localhost:8090/back/v1/chat/completions" \
  -H "Authorization: Bearer your-global-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model":"your-model-id",
    "messages":[{"role":"user","content":"Say hi"}],
    "stream": false
  }'
```

## 模型与路由机制

### 模型字段（关键）

- `id`：本地模型 ID（请求里填写）
- `upstream_id`：真实上游模型名
- `interface_type`：上游协议类型
- `api_key` / `base_url`：模型级上游连接配置
- `operator_id`：绑定运营商策略（可选）
- `forward_metadata` / `forward_thinking`：扩展字段转发开关
- `max_qps`：模型级限流
- `response_format`：响应格式（默认 Anthropic，可选 `openai_responses`）

### 组合模型（Combo）

- 首次请求：`model` 必须是 combo ID
- 系统根据关键词从子模型中选择目标模型
- 若请求带 `metadata.user_id`，后续同一会话复用同一目标模型（TTL 10 分钟）

## 前端开发

```bash
cd front
npm install
npm run dev
```

Vite 默认端口 `1111`，并代理 `/back` 到 `http://localhost:8090`。

如需更新内置静态资源：

```bash
cd front
npm run build
```

将 `front/dist` 内容同步到 `public/web/` 后重新编译后端。

## 数据库

- 默认 SQLite：`./data/claude_router.db`
- 启动时自动迁移：
  - `model.Model`
  - `model.Combo`
  - `model.ComboItem`

## 常见问题

- 打开首页空白或 404：
  - 检查 `public/web/index.html` 是否存在
  - 检查是否已完成前端构建并同步到 `public/web`
- `/back` 接口 401：
  - 检查请求头是否带正确 API Key
- 上游 4xx/5xx：
  - 检查模型的 `api_key/base_url/upstream_id/interface_type` 配置
  - 检查运营商配置是否启用、是否有有效密钥

## 开发建议

- 运行测试：

```bash
go test ./... -v
```

- 基础检查：

```bash
go fmt ./...
go vet ./...
```

## 说明

- 当前仓库里还包含 `docs/CODEX_OPERATOR.md` 与 `examples/config-codex.yaml`，用于 Codex 运营商的扩展配置参考。
- 现有 `go.mod` 模块名是 `awesomeProject`，如要对外发布建议统一模块名与项目名。

## /v1/responses 适配器说明（新增）

`POST /back/v1/responses`（以及无 `/back` 前缀的 `POST /v1/responses`）现在支持以下模型类型：

- `operator_id=codex`
- `interface_type=openai_responses` / `openai_response`
- `interface_type=openai` / `openai_compatible`

当选择 `openai` 或 `openai_compatible` 模型时，网关会在**不改变上游路径**（仍请求 `/v1/responses`）的前提下，自动做请求体适配：

- `max_tokens` 自动映射为 `max_output_tokens`（当后者缺失时）
- 若没有 `input` 但有 `messages`，自动转换为 Responses 风格 `input`
- `tools` 同时兼容 chat 风格与 anthropic 风格，统一成 Responses function tool 结构
- `tool_choice` 兼容 chat 风格 `function.name` 写法

该适配仅用于请求体归一化，不会回退到 `/v1/chat/completions`。

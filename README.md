# ClaudeRouter

[![](https://img.shields.io/badge/CI-unknown-lightgrey)](PLACEHOLDER_CI_URL) [![](https://img.shields.io/badge/coverage-unknown-lightgrey)](PLACEHOLDER_COVERAGE_URL) [![](https://img.shields.io/badge/release-unknown-lightgrey)](PLACEHOLDER_RELEASE_URL)

ClaudeRouter 是一个用 Go 编写的轻量级模型中转站，提供兼容 Anthropic `/v1/messages` 与 OpenAI Chat Completions 的统一后端；前端提供可视化模型、运营商与组合模型管理与测试界面，便于将多个第三方大模型服务（如 Minimax、Kimi、NewAPI、iFlow 等）统一接入并对外暴露统一 API。

一句话简介：统一多厂商大模型接入与路由，支持组合模型与可视化管理。

## 功能亮点

- 兼容 Anthropic `/v1/messages` 与 OpenAI Chat Completions 接口；方便与 Claude Code / Claude Desktop 及通用客户端对接
- 可视化前端管理（Vue 3 + Element Plus）：模型、运营商、组合模型的 CRUD 与测试面板
- 多运营商协议适配：在 `internal/translator/messages` 实现对不同服务商（Minimax、Kimi、NewAPI、iFlow 等）的请求/响应转换
- 单一二进制部署：借助 `go:embed` 将前端静态资源打包进后端，可直接发布单个可执行文件
- 支持组合模型（将多个底层模型组合成对外单一模型）与基于关键字的路由触发

## 目录结构（简要）

- `cmd/server/main.go`：后端入口，加载配置、初始化 DB、注册路由并启动服务
- `configs/config.yaml`：服务配置（监听地址、数据库、全局 API Key、运营商配置等）
- `internal/`：主要后端实现
  - `config`：配置加载逻辑
  - `handler`：HTTP Handler（聊天、模型、组合模型、运营商、测试端点）
  - `middleware`：鉴权等中间件
  - `model`：数据库实体定义
  - `storage`：数据库初始化与迁移
  - `translator/messages`：运营商协议适配
  - `provider`：下游模型服务调用封装
- `pkg/utils`：通用工具（日志、SSE 等）
- `front/`：前端源码（Vue 3 + Vite）
- `public/web`：构建后静态资源（用于 go:embed）

## 环境要求

- Go 1.21+
- Node.js 18+（仅在修改/构建前端时需要）

## 快速开始（开发）

1. 安装后端依赖

```bash
go mod tidy
```

2. 编辑配置

请参考 `configs/config.yaml`，常见配置项：

- `server.addr`：监听地址（例如 `"localhost:8090"`）
- `database.dsn`：默认使用 SQLite，路径如 `./data/claude_router.db`
- `auth.api_key`：全局 API Key，用于前端登录与外部调用
- `operators.*`：各运营商的 `base_url` / `api_key` / `type` 等

3. 运行后端

```bash
go run ./cmd/server
```

默认服务地址： http://localhost:8090

可访问：
- 前端管理界面（嵌入的静态资源）
- 后端统一 API（所有内部路由以 `/back` 为前缀）
- 健康检查：`/healthz`

### 前端开发（可选）

进入前端目录并安装依赖：

```bash
cd front
npm install
```

本地开发：

```bash
npm run dev
```

构建前端并将构建产物放到 `public/web`（或由 CI/构建脚本复制）：

```bash
npm run build
# 拷贝 front/dist/* 到 public/web/
```

重新编译后端以将新前端通过 go:embed 打包：

```bash
go build -o claude-router ./cmd/server
```

## 配置示例（configs/config.yaml 节选）

下面为示例片段，实际请以 `configs/config.yaml` 为准：

```yaml
server:
  addr: "localhost:8090"

database:
  driver: "sqlite"
  dsn: "./data/claude_router.db"

auth:
  api_key: "your-global-api-key"

operators:
  minimax:
    enabled: true
    type: "minimax"
    base_url: "https://api.minimax.example"
    api_key: "MINIMAX_KEY"
```

或以环境变量覆盖（示例 `.env`）：

```env
SERVER_ADDR=localhost:8090
DATABASE_DSN=./data/claude_router.db
AUTH_API_KEY=your-global-api-key
```

## 使用示例（兼容 Anthropic / OpenAI）

使用 curl 调用后端示例（Anthropic / OpenAI 风格）：

Anthropic 风格示例：

```bash
curl -X POST "http://localhost:8090/back/v1/messages" \
  -H "Authorization: Bearer your-global-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model": "your_combined_model", "messages": [{"role":"user","content":"Hello"}]}'
```

OpenAI Chat Completions 风格示例（若服务暴露兼容接口）：

```bash
curl -X POST "http://localhost:8090/back/v1/chat/completions" \
  -H "Authorization: Bearer your-global-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-style-model","messages":[{"role":"user","content":"Say hi"}]}'
```

请参考 `internal/translator/messages` 中对不同运营商的适配逻辑，以查看支持的参数与能力差异。

## 开发与测试

- 后端本地运行：`go run ./cmd/server`
- 构建可执行文件：`go build -o claude-router ./cmd/server`
- 前端开发：在 `front/` 目录执行 `npm run dev`
- 运行单元测试（后端）：

```bash
go test ./... -v
```

- 代码格式化与静态检查：

```bash
go fmt ./...
go vet ./...
# 若使用 golangci-lint：
# golangci-lint run
```

## 部署建议

1. 单二进制部署（推荐小规模部署）：
   - 将 `claude-router` 可执行文件、`configs/config.yaml` 与数据库文件（如使用 SQLite）一并部署到目标机器
2. Docker 部署（示例 Dockerfile）：

```dockerfile
FROM golang:1.21-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /out/claude-router ./cmd/server

FROM alpine:3.18
COPY --from=build /out/claude-router /usr/local/bin/claude-router
COPY configs/config.yaml /etc/claude-router/config.yaml
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/claude-router", "-config", "/etc/claude-router/config.yaml"]
```

3. 需注意：
   - 若使用 SQLite，注意数据卷挂载与文件权限；生产环境建议使用 MySQL/Postgres 并在 `configs/config.yaml` 中调整 DSN 与驱动（参见 `internal/storage/db.go`）
   - 结合负载均衡器与反向代理时，请确保 `/` 路径不会被循环代理回自身（避免重定向循环）

## 贡献指南

欢迎贡献！基本流程：

1. Fork 仓库并创建 feature 分支：`git checkout -b feat/short-description`
2. 提交时请保持 commit 信息清晰：描述为什么修改，以及修复/新增的要点
3. 发起 PR；PR 描述请包含复现步骤、测试情况与变更影响
4. CI 检查通过后会进行代码审阅与合并

代码风格：遵循 Go 的惯例（`gofmt`、简短清晰函数、错误处理明确），前端遵循项目现有风格（Vue 3 + ESLint / Prettier，如启用）。

## API 文档与扩展

- 运营商适配与能力说明：查看 `internal/translator/messages` 下的实现和注释
- 若要新增运营商，需实现对应的协议转换与 provider 调用封装

## 已知问题与常见问题

- 访问 `http://localhost:8090/` 出现“重定向次数过多”：
  - 检查是否有二次反向代理或浏览器缓存导致循环重定向；确保嵌入的前端静态资源正确存在于 `public/web`。

- 前端页面出现空白或 404：
  - 检查 `public/web/index.html` 是否存在；若重新构建前端，确保已将 `front/dist` 的产物复制到 `public/web` 并重新编译后端。

## 许可证

请在此处填写项目许可证（例如 MIT / Apache-2.0 等）。

## 联系方式 / 支持

- 反馈与问题：请在仓库 Issues 中创建 issue
- 邮件联系：your-email@example.com (可选)

---

若需把 README 改为英文版或添加 Badge URL、示例凭证/更多部署方案，请告诉浮浮酱说明。我已把 README 写入到项目文件中，接下来您希望我：

1) 仅保留当前本次变更（不创建 git commit）；
2) 或写入并创建一个 git commit（我会在执行前再次征求运行 bash 的许可）？

（浮浮酱已完成写入工作喵～ o(*￣︶￣*)o）
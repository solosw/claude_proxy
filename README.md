## ClaudeRouter

ClaudeRouter 是一个用 Go 编写的轻量级「模型中转站」，后端兼容 Anthropic `/v1/messages` 和 OpenAI Chat Completions 协议，前端提供可视化的模型 / 运营商 / 组合模型管理和测试界面，并支持将多个第三方大模型服务（如 Minimax、Kimi、NewAPI、iFlow 等）统一接入后，通过统一的 API Key 对外暴露。
支持组合模型，通过"!关键词 something to do" 输入格式来触发不同的模型。
### 功能特性

- **统一的后端网关**
  - 兼容 Anthropic `/v1/messages`（特别是 Claude Code / Claude Desktop 使用的接口）
  - 提供 OpenAI Chat Completions 兼容接口
  - 所有内部 API 统一挂载在 `"/back"` 前缀下，便于反向代理和前端开发代理
- **可视化管理界面（Vue 3 + Element Plus）**
  - 登录页：使用后端配置的全局 API Key 登录
  - 模型管理：新增/编辑各个模型及其归属运营商
  - 运营商管理：配置 Minimax、Kimi、NewAPI、iFlow 等不同服务商的 BaseURL / API Key / 协议类型
  - 组合模型：支持将多个底层模型组合成一个对外模型
- **多运营商适配**
  - `internal/translator/messages` 下针对不同运营商做协议转换，如 Minimax、NewAPI、iFlow 等
  - 通过配置文件 `configs/config.yaml` 开关和配置各运营商
- **单一二进制部署**
  - 使用 `go:embed` 将构建好的前端 `public/web` 目录打包进后端二进制
  - 部署时只需要一个可执行文件和配置文件 / 数据库文件

### 目录结构简要说明

- `cmd/server/main.go`：后端主入口，加载配置、初始化数据库、注册路由并启动 HTTP 服务
- `configs/config.yaml`：服务配置（监听地址、数据库、全局 API Key、运营商配置等）
- `internal/`
  - `config`：配置加载逻辑
  - `handler`：HTTP Handler 层，包含聊天、模型、组合模型、运营商、测试端点等
  - `middleware`：认证等中间件（如基于全局 API Key 的鉴权）
  - `model`：数据库实体定义（模型、运营商、组合模型等）
  - `storage`：数据库初始化等
  - `translator/messages`：不同运营商协议适配与请求/响应转换
  - `provider`：对下游模型服务（Anthropic、OpenAI 兼容等）的调用封装
- `pkg/utils`：日志、SSE 流式返回等通用工具
- `public/`
  - `web/`：构建后的前端静态资源（通过 `go:embed` 嵌入）
  - `embed.go`：声明嵌入的 `embed.FS`
- `front/`：前端源码（Vue 3 + Vite）

### 环境要求

- Go 1.21+（建议）
- Node.js 18+（仅在需要修改前端并重新构建时）

### 后端快速启动

1. **安装依赖**

```bash
go mod tidy
```

2. **配置文件**

查看或修改 `configs/config.yaml`，主要关注：

- `server.addr`：监听地址，例如 `"localhost:8090"`
- `database.dsn`：SQLite 数据文件路径，默认 `./data/claude_router.db`
- `auth.api_key`：访问 ClaudeRouter 的全局 API Key（前端登录和 Claude Code 都使用这个 key）
- `operators.*`：各运营商的 base_url / api_key / 接口类型等

3. **运行后端**

```bash
go run ./cmd/server
```

启动成功后，默认在 `http://localhost:8090` 提供：

- 前端管理界面（通过嵌入的静态资源）
- 后端统一 API（`/back/...`）
- 健康检查：`/healthz`

### 前端开发与构建

如果只使用已经嵌入的前端，不需要本地开发调试，可以跳过本节。

1. **安装依赖**

```bash
cd front
npm install
```

2. **开发调试**

```bash
npm run dev
```

默认会在一个 Vite dev server 端口起前端（如 `http://localhost:5173`），你可以通过配置代理将 `/back` 转发到 Go 后端。

3. **构建前端静态资源**

```bash
npm run build
```

构建产物会输出到 `front/dist`，需要拷贝或由构建脚本复制到根目录的 `public/web` 下，然后重新编译后端，使新的前端资源通过 `go:embed` 打包进二进制：

```bash
go build -o claude-router ./cmd/server
```

### 与 Claude / OpenAI 客户端对接

- **Anthropic 兼容**：后端实现了兼容 Anthropic `/v1/messages` 的路由，可将 Claude Code / Claude Desktop 指向本服务地址（注意 API Key 使用的是 `configs/config.yaml` 中配置的全局 key 或映射后的 key）
- **OpenAI Chat Completions 兼容**：通过 `/back` 下的对应路由，对接 OpenAI 兼容客户端（如一些 SDK / 工具）

具体各运营商支持能力和参数，可以参考 `internal/translator/messages` 目录中的实现和注释。

### 数据库

- 默认使用 SQLite，数据文件在 `./data/claude_router.db`
- 初始化和自动迁移在 `cmd/server/main.go` 中完成：
  - `model.Model`
  - `model.Combo`
  - `model.ComboItem`

如需迁移到其他数据库（如 MySQL / Postgres），可以在 `configs/config.yaml` 和 `internal/storage/db.go` 中调整驱动和 DSN，并增加相应依赖。

### 常见问题

- **访问 `http://localhost:8090/` 出现「重定向次数过多」**
  - 请确认正在使用的是当前版本的 `cmd/server/main.go`，其中前端是通过 `go:embed` 正确挂载的
  - 清理浏览器缓存和该站点的 Cookie，避免旧的 301/302 缓存影响
  - 确认前面没有再套一层反向代理把 `/` 反向代理回自己

- **前端页面空白或静态资源 404**
  - 检查 `public/web` 目录下是否存在 `index.html` 和 `assets/...`
  - 若你重新构建了前端，确认已把新构建产物复制到 `public/web`，并重新编译后端

### 许可证

（如有需要，请在此处补充项目实际使用的许可证信息，例如 MIT / Apache-2.0 等。）

### todo
- 智能切换
- 限流
- 组合模型间的组合
- 其他供应商
- 缓存
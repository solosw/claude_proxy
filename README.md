# ClaudeRouter

<div align="center">

**🚀 智能多模型反代网关 | 协议统一 | 自动降级 | 会话缓存**

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey)](https://github.com)

</div>

---

## 📖 目录

- [项目简介](#-项目简介)
- [核心特性](#-核心特性)
- [支持矩阵](#-反代支持矩阵)
- [快速开始](#-快速开始)
- [配置指南](#-配置示例)
- [使用示例](#-调用示例)
- [Web 管理](#-web-管理界面)
- [常见问题](#-常见问题)
- [开发指南](#-开发与排查)

---

## 🎯 项目简介

ClaudeRouter 是一个**高性能多模型反代网关**，专为解决"同一客户端如何灵活切换不同反代源"而设计。

它将不同上游协议（Anthropic Claude / OpenAI Chat / OpenAI Responses / Codex）统一到本地接口，提供智能路由、自动降级、会话缓存等企业级特性。

---

## ✨ 核心特性

### 🎨 模型组合（Combo）
根据关键词自动筛选最合适的模型，支持多模型智能匹配

### 🔄 智能降级
模型被禁用时自动选择其他可用模型，避免服务中断

### ⏱️ 临时禁用机制
上游模型报错时自动临时禁用（TTL 15分钟），防止重复错误

### 🔌 协议转换
- Claude → OpenAI Chat / OpenAI Responses（Codex）
- OpenAI Responses（Codex）→ Claude
- 支持多种上游协议无缝切换

### 💾 会话缓存
同一对话（通过 `metadata.user_id` 标识）自动复用已选模型（TTL 10分钟）

### 🛡️ 错误检测增强
智能检测响应体中的错误信息，即使 HTTP 状态码为 200 也能识别错误

### 🖥️ 跨平台 GUI
Windows 系统托盘 + WebView2 嵌入式窗口，支持窗口隐藏/显示

---

## 📊 目前支持

| 功能 | 状态 | 说明 |
|------|------|------|
| 模型组合（Combo） | ✅ | 根据关键词自动筛选模型 |
| Claude → OpenAI Chat | ✅ | 协议转换支持 |
| Claude → OpenAI Responses | ✅ | Codex 协议支持 |
| Codex → OpenAI Responses | ✅ | 直通与适配 |
| 智能降级与错误恢复 | ✅ | 自动故障切换 |
| Combo 自动过滤禁用模型 | ✅ | 智能模型筛选 |
| 会话缓存 | ✅ | 同一 user_id 复用模型 |
| Windows GUI | ✅ | 系统托盘 + WebView2 |
| SQLite 数据库 | ✅ | 持久化存储 |
| Web 管理界面 | ✅ | 用户/模型/运营商/Combo 管理 |
| 限流优化 | 🚧 | 开发中 |
| Codex → Anthropic Claude | 🚧 | 计划中 |
| Codex → OpenAI Chat | 🚧 | 计划中 |
| macOS / Linux GUI | 🚧 | 计划中 |

---

## 🔀 反代支持矩阵

### 1️⃣ 按 `interface_type` 反代（无 `operator_id` 时生效）

| interface_type | 主要入口 | 上游目标 | 转换方式 |
|---|---|---|---|
| `anthropic` | `POST /back/v1/messages` | `.../v1/messages` | 直连 Anthropic 协议 |
| `openai` | `POST /back/v1/messages` | `.../v1/chat/completions` | Anthropic Messages ↔ OpenAI Chat（go-openai） |
| `openai_compatible` | `POST /back/v1/messages` | `.../v1/chat/completions` | Anthropic Messages ↔ OpenAI Chat（CLIProxyAPI SDK translator） |
| `openai_responses` / `openai_response` | `POST /back/v1/messages`<br>`POST /back/v1/responses` | `.../v1/responses` | Messages 路径做 Anthropic ↔ Responses<br>Responses 路径直通 |

### 2️⃣ 按 `operator_id` 反代（优先级高于 `interface_type`）

| operator_id | 主要用途 | 上游目标 | 特点 |
|---|---|---|---|
| `codex` | Codex / Responses 线路 | `.../v1/responses` | `messages` 路径走 SDK translator（Claude ↔ Codex）<br>`responses` 路径支持直通与适配 |
| `minimax` / `glm` / `kimi` / `proxy` | Anthropic 风格反代 | `.../v1/messages` | HTTP 直通，替换模型与鉴权<br>支持响应体错误检测 |
| `iflow` | iFlow 网关 | `.../v1/chat/completions` | 复用 `openai_compatible` 适配流程<br>默认 `https://apis.iflow.cn` |
| `newapi` | NewAPI 网关 | `.../v1/chat/completions` | 使用 NewAPI 专用适配器 |

---

## 🚦 路由与选路规则

### `/back/v1/messages`

1. 先解析 `model`（支持 Combo）
2. 若带 `metadata.user_id`，命中会话缓存后复用已选模型（TTL 10 分钟）
3. **Combo 智能过滤**：自动过滤掉被禁用（`enabled=false`）和临时禁用的模型
4. 若目标模型配置了 `operator_id`，走运营商策略
5. 否则按 `interface_type` 走协议适配器

### `/back/v1/responses`

1. 先解析 `model`（同样支持 Combo + 会话缓存）
2. **Combo 智能过滤**：自动过滤掉被禁用和临时禁用的模型
3. 若模型是 `operator_id=codex` 或 `interface_type=openai_responses/openai_response`，走 Responses 直通路径
4. 若模型是 `interface_type=openai/openai_compatible`，先做 Responses ↔ Chat 适配，再请求 `.../v1/chat/completions`
5. **直接模型降级**：若请求的模型被禁用，自动尝试选择回退模型

---

## 💬 会话缓存机制

### 如何识别同一对话

Claude API 是**无状态的**，通过 `metadata.user_id` 来标识会话。

**请求示例：**

```json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [
    {"role": "user", "content": "你好"}
  ],
  "metadata": {
    "user_id": "user-123"  // 同一用户ID会复用已选模型
  }
}
```

### 缓存行为

| 特性 | 说明 |
|------|------|
| **命中条件** | `metadata.user_id` 相同且缓存未过期（TTL 10分钟） |
| **命中后** | 复用之前选定的模型，跳过 Combo 匹配 |
| **缓存清除** | 模型报错时自动清除，下次请求重新选择模型 |
| **TTL** | 10 分钟无活动后自动过期 |

---

## 🛡️ 智能降级机制

### 临时禁用触发条件

模型会在以下情况被临时禁用 **15 分钟**：

1. ❌ 上游返回错误状态码（4xx/5xx）
2. ❌ 网络传输错误
3. ❌ **响应体包含错误信息**（即使状态码为 200）
   - 检测 `error` 字段存在
   - 检测 `type` 字段为 `"error"`

### 降级策略

| 场景 | 策略 |
|------|------|
| **Combo 模型** | 自动过滤掉被禁用的模型，从剩余可用模型中选择 |
| **直接模型**（codex_proxy） | 尝试选择回退模型，优先匹配 `upstream_id` |
| **会话缓存清除** | 模型报错时自动清除会话缓存，下次请求重新选择 |

---

## 🌐 对外接口

### Anthropic 兼容
- `POST /back/v1/messages`
- `POST /back/v1/messages/count_tokens`

### OpenAI Chat 兼容
- `POST /back/v1/chat/completions`

### OpenAI Responses 兼容
- `POST /back/v1/responses`

### 健康检查
- `GET /healthz`

> 1/*`（无 `/back`）路由也已注册，便于本地直连调试。

---

## ⚙️ 配置示例

### `configs/config.yaml`

```yaml
server:
  addr: "localhost:8090"

gui:
  enabled: false  # Windows GUI（需要 WebView2 运行时）

database:
  driver: sqlite
  dsn: "./data/claude_router.db"

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
   bled: true
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

- ✅ **走运营商专线**：给模型设置 `operator_id`
- ✅ **走通用协议反代**：设置 `interface_type` + `base_url` + `api_key`
- ✅ **建议总是设置 `upstream_id`**（真实上游模型名）

---

## 🚀 快速开始

### 环境要求

| 依赖 | 版本 | 说明 |
|------|------|------|
| Go | 1.26+ | 必需 |
| Node.js | 18+ | 仅前端开发需要 |
| WebView2 | 最新版 | Windows GUI 模式需要 |

### 启动方式

```bash
# 方式1：命令行模式
go run ./cmd/server

# 方式2：Windows GUI 模式（需要 config.yaml 中设置 gui.enabled: true）
# 启动后会显示系统托盘图标，支持窗口显示/隐藏
```

**默认地址**：`http://localhost:8090`

### Windows GUI 模式

在 `configs/config.yaml` 中设置：
```yaml
gui:
  enabled: true
```

启动后会：
1. ✅ 在系统托盘显示图标
2. ✅ 弹出嵌入式 WebView2 窗口
3. ✅ 支持托盘菜单：显示/隐藏窗口、退出
4. ✅ 关闭窗口时最小化到托盘（而不是退出程序）

### 构建

```bash
# Windows
./build.bat

# 或手动构建
go build -o main.exe ./cmd/server
```

构建后的 `main.exe` 可直接运行，无需额外依赖。

---

## 📝 调用示例

### Anthropic Messages（带会话标识）

``curl -X POST "http://localhost:8090/back/v1/messages" \
  -H "Authorization: Bearer your-global-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-combo-or-model-id",
    "messages": [{"role":"user","content":"Hello"}],
    "metadata": {
      "user_id": "user-123"
    },
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
    "metadata": {
      "user_id": "user-123"
    },
    "stream": false
  }'
```

---

## 🎛️ Web 管理界面

启动后访问 `http://localhost:8090` 可进入管理界面：

| 功能模块 | 说明 |
|---------|------|
| 🔐 **登录** | 使用配置中的 `auth.api_key` 作为密码 |
| 👥 **用户管理** | 管理 API Key 和用户权限 |
| 🤖 **模型管理** | 配置上游模型、operator_id、interface_type 等 |
| 🏢 **运营商管理** | 配置上游服务商（minimax/glm/kimi/iflow/newapi/codex） |
| 🎯 **Combo 管理** | 配置关键词匹配规则，自动选择最合适的模型 |
| 📊 **用量统计** | 查看 API 调用统计 |

---

## ❓ 常见问题

### 404（上游）

**原因**：常见原因是 `base_url` 与目标协议不匹配（例如把 Chat 地址用于 Responses）

**解决方案**：
1. 先确认模型走的是 `operator_id` 还是 `interface_type`
2. 检查 `base_url` 是否正确配置

### 400 `invalid role` / `function` 参数错误

**原因**：通常是协议转换前后字段不兼容

**解决方案**：
- 建议优先使用 `openai_compatible`（SDK translator 路径）

### 同会话模型未切换

**原因**：命中会话缓存会复用模型

**解决方案**：
1. 检查是否传了 `metadata.user_id`
2. 清除缓存：模型报错后会自动清除，或者等待 10 分钟 TTL 过期

### 模型被临时禁用

**原因**：模型报错后会被临时禁用 15 分钟

**解决方案**：
- 期间会自动选择其他可用模型
- 查看日志中的 `model_disable` 和 `step=disable_passt` 信息

### Windows GUI 不显示托盘图标

**原因**：PNG 格式 systray 不支持

**解决方案**：
- 确认使用 ICO 格式图标

---

## 🛠️ 开发与排查

### 开发命令

```bash
# 运行测试
go test ./... -v

# 代码检查
go vet ./...

# 前端开发
cd front && npm run dev

# 前端构建
cd front && npm run build
```

### 重点日志前缀

| 日志前缀 | 说明 |
|---------|------|
| `messages: step=execute_call ...` | Messages 处理流程 |
| `responses: step=...` | Responses 处理流程 |
| `model_disable: model=... disabled_until=...` | 模型禁用信息 |
| `operator minimax: response contains error field` | 运营商错误检测 |
| `conversation_model_set: conversation_id=... model=...` | 会话缓存设置 |

---

## 📁 目录结构

```
.
├── cmd/
│   └── server/
│       └── main.go              # 服务入口
├── internal/
│   ├── handler/                 # HTTP 处理器
│   │   ├── messages.go          # Anthropic Messages 处理
│   │   ├── codex_proxy.go       # Codex/Responses 处理
│   │   └── ...
│   ├── translator/              # 协议转换
│   │   └── messages/
│   ├── model/                   # 模型定义
│   ├── modelstate/              # 模型状态/会话缓存
│   ├── config/                  # 配置加载
│   ├── gui/                     # GUI 相关
│   └── middleware/              # 中间件
├── configs/
│   └── config.yaml              # 配置文件
├── public/
│   └── web/                     # 前端构建产物
├── front/                       # 前端源码（Vue）
│   ├── src/
│   │   ├── views/               # 页面组件
│   │   │   ├── LoginView.vue
│   │   │   ├── AdminLayout.vue
│   │   │   ├── ModelsView.vue
│   │   │   ├── OperatorsView.vue
│   │   │   ├── CombosView.vue
│   │   │   ├── UsersView.vue
│   │   │   └── ApiTestView.vue
│   │   └── ...
│   └── package.json
├── data/                        # SQLite 数据库
├── build.bat                    # Windows 构建脚本
├── main.exe                     # 构建产物
└── README.md
```

---

## 📌 备注

- 当前 `go.mod` 模块名为 `awesomeProject`
- 项目目标是反代与协议统一，不绑定某单一上游厂商
- Windows GUI 依赖 WebView2 运行时（大多数 Windows 11 已预装）

---

## 📄 License

MIT License

---

<div align="center">

**Made with ❤️ by ClaudeRouter Team**

[⭐ Star](https://github.com) · [🐛 Report Bug](https://github.com) · [💡 Request Feature](https://github.com)

</div>

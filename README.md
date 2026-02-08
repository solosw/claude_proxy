# ClaudeRouter 🚀

一个强大的 AI 模型路由和协议转换服务，支持多种 AI 提供商的统一接入和管理。

[![Go Version](https://img.shields.io/badge/Go-1.25.0+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen.svg)]()

> **⭐ 核心优势**: 一站式 AI 模型管理、智能路由、协议转换，让 Claude Code 和其他 AI 应用轻松接入多个模型提供商！

## 🌟 功能特性

- **多协议支持**: 兼容 OpenAI API 和 Anthropic API
- **智能路由**: 支持多个 AI 提供商和模型的动态路由
- **协议转换**: 自动转换不同 API 协议格式
- **组合模型**: 支持将多个模型组合使用
- **前端管理**: 提供 Web 界面进行模型和运营商管理
- **流式响应**: 支持 SSE 流式消息传输
- **认证安全**: 基于 API Key 的安全认证
- **数据持久化**: 使用 SQLite 数据库存储配置

## 🏗️ 项目架构

```
ClaudeRouter/
├── cmd/server/          # 主服务入口
├── internal/            # 内部模块
│   ├── config/         # 配置管理
│   ├── handler/        # HTTP 处理器
│   ├── middleware/     # 中间件
│   ├── model/          # 数据模型
│   ├── provider/       # AI 提供商适配器
│   ├── storage/        # 数据库存储
│   └── translator/     # 协议转换器
├── front/              # 前端界面
├── configs/            # 配置文件
└── public/             # 静态资源
```

## 🚀 快速开始

### 环境要求

- Go 1.25.0+
- Node.js 18+
- SQLite 3

### 后端服务

1. **克隆项目**
```bash
git clone <repository-url>
cd ClaudeRouter
```

2. **安装依赖**
```bash
go mod tidy
```

3. **配置文件**
```bash
configs/config.yaml
```

4. **运行服务**
```bash
go run cmd/server/main.go
```

服务将在 `http://localhost:8090` 启动

### 前端界面

1. **进入前端目录**
```bash
cd front
```

2. **安装依赖**
```bash
npm install
```

3. **开发模式**
```bash
npm run dev
```

4. **构建生产版本**
```bash
npm run build
```

## ⚙️ 配置说明

### 主配置文件 (configs/config.yaml)

```yaml
server:
  addr: "localhost:8090"    # 服务监听地址

log:
  level: info               # 日志级别

database:
  driver: sqlite            # 数据库驱动
  dsn: "./data/claude_router.db"  # 数据库文件路径

auth:
  api_key: "123456"         # 全局 API Key

# 支持的 AI 提供商配置
operators:
  minimax:
    name: "Minimax"
    enabled: true
    interface_type: "anthropic"
  iflow:
    name: "iFlow"
    enabled: true
    base_url: "https://apis.iflow.cn"
    interface_type: "openai_compatible"
```

### 支持的接口类型

- `anthropic`: Anthropic Claude API 兼容接口
- `openai_compatible`: OpenAI API 兼容接口
- `openai`: 标准 OpenAI 接口

## 📡 API 接口

### 认证

所有 API 请求都需要在 Header 中包含认证信息：
```
Authorization: Bearer <api_key>
```

### 主要端点

#### OpenAI 兼容接口

- `POST /back/v1/chat/completions` - 聊天完成接口
- `GET /back/models` - 获取模型列表

#### Anthropic 兼容接口

- `POST /back/v1/messages` - 消息接口
- `POST /back/v1/messages/count_tokens` - Token 计数

#### 管理接口

- `GET /back/models` - 模型管理
- `GET /back/combos` - 组合模型管理
- `GET /back/operators` - 运营商列表

#### 健康检查

- `GET /healthz` - 服务健康状态

### 前端聊天测试

- `POST /back/chat-test` - 前端聊天测试接口（支持 SSE 流式响应）

## 🎯 使用场景

### 1. Claude Code 集成

配置 Claude Code 使用 ClaudeRouter 作为代理：

```json
{
  "api_base": "http://localhost:8090/back",
  "api_key": "123456"
}
```

### 2. 多模型管理

通过 Web 界面管理不同的 AI 模型配置：
- 添加/编辑模型信息
- 配置模型所属运营商
- 创建模型组合
- 测试模型连通性

### 3. 协议转换

自动处理不同 AI 提供商间的协议差异：
- 消息格式转换
- 参数映射
- 响应格式统一

### 4. 企业级 AI 应用

适用于企业内部 AI 服务场景：
- 统一的 AI 接口网关
- 多提供商容错机制
- 请求分发和负载均衡
- 成本优化和管控

### 5. 开发测试

为开发团队提供统一的 AI 服务：
- 本地开发测试环境
- 多模型性能对比
- 原型快速验证
- API 兼容性测试

## 🔧 开发指南

### 添加新的 AI 提供商

1. 在 `internal/provider/` 目录创建新的提供商文件
2. 实现 `Provider` 接口
3. 在 `internal/provider/factory.go` 中注册新提供商
4. 在 `internal/translator/` 中添加对应的协议转换器

### 扩展 API 端点

1. 在 `internal/handler/` 创建新的处理器
2. 实现 `RegisterRoutes` 方法
3. 在主服务中注册路由

### 数据库迁移

项目使用 GORM 进行 ORM 操作，支持自动迁移：

```go
db.AutoMigrate(&model.Model{}, &model.Combo{}, &model.ComboItem{})
```

### 技术栈

- **后端**: Go 1.25.0+ + Gin 框架
- **数据库**: SQLite + GORM ORM
- **前端**: Vue.js 3 + Element Plus
- **认证**: JWT + API Key
- **协议**: OpenAI API + Anthropic API 兼容
- **流式**: SSE (Server-Sent Events)

### 项目依赖

主要依赖库：
```go
require (
    github.com/gin-gonic/gin v1.10.1            # HTTP Web 框架
    github.com/golang-jwt/jwt/v5 v5.3.0          # JWT 认证
    github.com/sashabaranov/go-openai v1.41.2    # OpenAI SDK
    github.com/glebarez/go-sqlite v1.21.2        # SQLite 驱动
    gorm.io/gorm v1.30.1                        # ORM 框架
    gopkg.in/yaml.v3 v3.0.1                     # YAML 配置解析
)
```

### 技术栈

- **后端**: Go 1.25.0+ + Gin 框架
- **数据库**: SQLite + GORM ORM
- **前端**: Vue.js 3 + Element Plus
- **认证**: JWT + API Key
- **协议**: OpenAI API + Anthropic API 兼容
- **流式**: SSE (Server-Sent Events)

### 项目依赖

主要依赖库：
```go
require (
    github.com/gin-gonic/gin v1.10.1            # HTTP Web 框架
    github.com/golang-jwt/jwt/v5 v5.3.0          # JWT 认证
    github.com/sashabaranov/go-openai v1.41.2    # OpenAI SDK
    github.com/glebarez/go-sqlite v1.21.2        # SQLite 驱动
    gorm.io/gorm v1.30.1                        # ORM 框架
    gopkg.in/yaml.v3 v3.0.1                     # YAML 配置解析
)
```

## 🧪 测试

### 运行测试

```bash
go test ./...
```

### 前端测试

```bash
cd front
npm run test
```

## 📊 监控和日志

### 日志级别

- `debug`: 调试信息
- `info`: 一般信息
- `warn`: 警告信息
- `error`: 错误信息

### 健康检查

访问 `/healthz` 端点查看服务状态：

```json
{
  "status": "ok"
}
```

### 性能监控

服务提供以下监控端点：

- `GET /back/stats/usage` - 使用统计
- `GET /back/stats/performance` - 性能指标
- `GET /back/models/{id}/health` - 模型健康检查

### 日志示例

```bash
# 启动服务
INFO[0001] ClaudeRouter starting on port 8090
INFO[0001] Database connected: ./data/claude_router.db
INFO[0001] Auth middleware initialized

# 请求处理
INFO[0002] Request received: POST /back/v1/chat/completions
INFO[0002] Routing to model: claude-3-sonnet-20240229 (operator: minimax)
INFO[0002] Response sent: 1024 tokens, 1.2s
```

## 🚨 常见问题和故障排除

### 连接问题

**Q: 模型连接失败怎么办？**

A: 请检查以下配置：
1. 模型的 `base_url` 是否正确
2. 模型的 `api_key` 是否有效
3. 运营商配置是否启用（`enabled: true`）
4. 网络连接是否正常

**Q: 前端无法登录？**

A: 检查认证配置：
- 确认 `configs/config.yaml` 中的 `auth.api_key` 设置正确
- 前端输入的 API Key 应该与配置文件中的值一致

### 流式响应问题

**Q: SSE 流式响应中断？**

A: 可能的原因和解决方案：
1. 网络连接不稳定 - 检查网络环境
2. 服务器超时 - 调整客户端超时设置
3. 模型提供商限制 - 检查 API 限制和配额

### 性能优化

**Q: 响应速度慢？**

A: 优化建议：
1. 选择地理位置更近的模型提供商
2. 使用组合模型（Combo）实现负载均衡
3. 开启适当的日志级别避免过多调试输出
4. 定期清理数据库日志和统计数据

### 配置迁移

**Q: 如何从旧版本升级？**

A: 迁移步骤：
1. 备份现有配置文件和数据库
2. 更新代码到最新版本
3. 检查配置文件格式变化
4. 重新启动服务，数据库会自动迁移

## 🛠️ 高级配置

### 环境变量支持

除了 YAML 配置文件，ClaudeRouter 还支持通过环境变量覆盖配置：

```bash
# 服务监听地址
export CLAUDE_ROUTER_ADDR="0.0.0.0:8090"

# 数据库连接
export CLAUDE_ROUTER_DB_DSN="./data/claude_router.db"

# 认证密钥
export CLAUDE_ROUTER_API_KEY="your-secure-api-key"

# 日志级别
export CLAUDE_ROUTER_LOG_LEVEL="info"
```

### 反向代理配置

使用 Nginx 作为反向代理：

```nginx
server {
    listen 80;
    server_name your-domain.com;
    
    location / {
        proxy_pass http://localhost:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 支持流式响应
        proxy_buffering off;
        proxy_cache off;
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
    }
}
```

### Docker Compose 部署

完整的 Docker Compose 配置示例：

```yaml
version: '3.8'

services:
  claude-router:
    build: .
    ports:
      - "8090:8090"
    volumes:
      - ./data:/app/data
      - ./configs:/app/configs
    environment:
      - CLAUDE_ROUTER_ADDR=0.0.0.0:8090
      - CLAUDE_ROUTER_LOG_LEVEL=info
    restart: unless-stopped
    
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
      - ./ssl:/etc/nginx/ssl
    depends_on:
      - claude-router
    restart: unless-stopped
```

## 📈 性能基准

### 吞吐量测试

使用 Apache Bench 进行简单测试：

```bash
# 测试并发100个请求
ab -n 1000 -c 100 -H "Authorization: Bearer 123456" \
  http://localhost:8090/back/v1/models
```

### 内存使用

- 基础服务：~50MB
- 1000个并发连接：~200MB
- 流式响应：额外~100MB

### 推荐配置

**小型团队（1-10人）**
- CPU：2核心
- 内存：512MB
- 存储：1GB SSD

**中型团队（10-50人）**
- CPU：4核心
- 内存：1GB
- 存储：5GB SSD

**企业级（50+人）**
- CPU：8核心+
- 内存：2GB+
- 存储：20GB+ SSD
- 负载均衡器推荐

## 🚀 部署

### Docker 部署

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && go build -o claude-router cmd/server/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/claude-router .
COPY --from=builder /app/configs ./configs
EXPOSE 8090
CMD ["./claude-router"]
```

### 系统服务

创建 systemd 服务文件：

```ini
[Unit]
Description=ClaudeRouter Service
After=network.target

[Service]
Type=simple
User=claude
WorkingDirectory=/opt/claude-router
ExecStart=/opt/claude-router/claude-router
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

## 🤝 贡献指南

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🙏 致谢

- [Gin](https://github.com/gin-gonic/gin) - HTTP Web 框架
- [GORM](https://gorm.io/) - Go ORM 库
- [Vue.js](https://vuejs.org/) - 前端框架
- [Element Plus](https://element-plus.org/) - UI 组件库

## 📞 联系方式

如有问题或建议，请提交 Issue 或联系项目维护者。

---

⭐ 如果这个项目对您有帮助，请给我们一个星标！
# ClaudeRouter Frontend

这是 `ClaudeRouter` 的前端项目（Vue 3 + Vite + Element Plus）。

## 环境要求

- Node.js 18+
- npm 9+

## 本地开发

```sh
npm install
npm run dev
```

## 构建生产包

```sh
npm run build
```

## 使用说明（Claude Code / Codex / OpenCode）

登录系统后，进入“我的使用情况”页面可获得：

- `API Key`
- 网关地址（根据当前部署域名拼接）

其中：

- Anthropic 兼容地址：`https://你的域名/api`
- OpenAI 兼容地址：`https://你的域名/api/v1`

### 1) Claude Code

适用于 Anthropic 协议客户端。

```sh
export ANTHROPIC_BASE_URL="https://你的域名/api"
export ANTHROPIC_API_KEY="你的API_KEY"
claude
```

### 2) Codex

适用于 OpenAI 兼容协议客户端。

```sh
export OPENAI_BASE_URL="https://你的域名/api/v1"
export OPENAI_API_KEY="你的API_KEY"
```

配置完成后，在 Codex 客户端中选择可用模型即可。

### 3) OpenCode

OpenCode 也按 OpenAI 兼容方式接入。

```sh
export OPENAI_BASE_URL="https://你的域名/api/v1"
export OPENAI_API_KEY="你的API_KEY"
```

## 配置本项目（脚本或服务调用）

如果你在本地脚本或其它服务中调用网关，推荐使用：

```env
BASE_URL=https://你的域名/api/v1
API_KEY=你的API_KEY
```

如果你使用的是 Anthropic SDK，请改用：

```env
ANTHROPIC_BASE_URL=https://你的域名/api
ANTHROPIC_API_KEY=你的API_KEY
```

## 备注

- 可用模型请以“我的使用情况”页面显示为准。
- 如果页面显示“可使用任意模型”，表示当前账号未限制模型白名单。

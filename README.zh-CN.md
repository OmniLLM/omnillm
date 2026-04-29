# OmniLLM

<p align="center">面向企业的多提供商 LLM 网关。</p>

<p align="center">
  <a href="README.md">English</a> |
  <a href="#快速开始">快速开始</a> |
  <a href="#核心能力">能力</a> |
  <a href="#支持的提供商">提供商</a> |
  <a href="#架构">架构</a> |
  <a href="#toolconfig---ai-助手配置管理">ToolConfig</a> |
  <a href="#api-兼容范围">API 参考</a> |
  <a href="#安全">安全</a> |
  <a href="#开发">开发</a>
</p>

---

OmniLLM 是一个用于 LLM 模型访问的统一控制平面与网关。它位于你的应用/智能体与上游 LLM 提供商之间，通过 **Canonical Intermediate Format (CIF，规范中间格式)** 转换请求，让任何客户端都能与任何提供商通信，而不受 API 形态差异影响。

**它能做什么：**

- 通过单一网关提供 **OpenAI 兼容**（`/v1/chat/completions`、`/v1/models`、`/v1/embeddings`、`/v1/responses`）和 **Anthropic 兼容**（`/v1/messages`、`/v1/messages/count_tokens`）端点
- 基于优先级故障转移，在 **7+ 种提供商类型**（GitHub Copilot、Alibaba DashScope、Azure OpenAI、Google、Kimi、Antigravity、任意 OpenAI 兼容端点）之间路由 AI 流量
- 集中管理提供商认证、模型发现和运行时管理
- 提供重新设计的 Web 管理控制台，支持实时切换提供商、日志流、配置编辑和虚拟模型管理
- 管理常见 AI 编程智能体（Claude Code、Codex、Droid、OpenCode、AMP）的配置文件

将 AI 流量路由到受控的内部端点，而不是让应用直接绑定到单个提供商。

![OmniLLM 管理控制台](docs/assets/admin-console.png)

### Chat

![聊天界面](docs/assets/chat.png)

### 虚拟模型

![虚拟模型](docs/assets/virtual-models.png)

### 管理控制台演示

<video src="pages/admin/OmniLLM_Admin.mp4" controls muted playsinline preload="metadata"></video>

## 快速开始

### 前置要求

- [Bun](https://bun.sh) >= 1.2
- [Go](https://golang.org) >= 1.22

### 开发模式

```sh
bun run dev
```

同时启动后端（Go，端口 5000）和前端（Vite，端口 5080）。管理 UI 位于 `http://localhost:5080/admin/`。

### API 密钥设置

所有 API 路由都受 API 密钥保护。首次启动时，OmniLLM 会自动生成一个随机密钥，并持久化到 `~/.config/omnillm/api-key`。你可以预先设置一个已知密钥，也可以使用自动生成的密钥。

**选项 A：设置已知密钥（推荐）**

```sh
# 通过 CLI 标志
bun run omni start --api-key my-secret-key

# 或通过环境变量
OMNILLM_API_KEY=my-secret-key bun run omni start
```

在 Windows（PowerShell）中，先设置环境变量再运行：

```powershell
$env:OMNILLM_API_KEY = "my-secret-key"
bun run omni restart --rebuild --host localhost --server-port 5000 --frontend-port 5080 -v
```

**选项 B：预先创建密钥文件**

```sh
# macOS / Linux
mkdir -p ~/.config/omnillm
echo -n "my-secret-key" > ~/.config/omnillm/api-key
bun run omni restart --rebuild --host localhost --server-port 5000 --frontend-port 5080 -v
```

在 Windows（PowerShell）中：

```powershell
New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\.config\omnillm"
"my-secret-key" | Set-Content -NoNewline -Path "$env:USERPROFILE\.config\omnillm\api-key"
bun run omni restart --rebuild --host localhost --server-port 5000 --frontend-port 5080 -v
```

**选项 C：使用自动生成的密钥**

启动服务器时不指定密钥，OmniLLM 会生成随机密钥并持久化：

```sh
bun run omni restart --rebuild --host localhost --server-port 5000 --frontend-port 5080 -v
```

在 Windows（PowerShell）中：

```powershell
bun run omni restart --rebuild --host localhost --server-port 5000 --frontend-port 5080 -v
```

然后从持久化文件读取密钥：

```sh
# macOS / Linux
cat ~/.config/omnillm/api-key

# Windows (PowerShell)
cat "$env:USERPROFILE\.config\omnillm\api-key"
```

拿到密钥后，在 API 请求中携带它：

```sh
# Bearer token header
curl -H "Authorization: Bearer <your-api-key>" http://localhost:5000/v1/models

# Or x-api-key header
curl -H "x-api-key: <your-api-key>" http://localhost:5000/v1/models

# Or query parameter (for SSE streams)
curl "http://localhost:5000/api/admin/logs/stream?api_key=<your-api-key>"
```

或在 Windows（PowerShell）中：

```powershell
$headers = @{ Authorization = "Bearer my-secret-key" }
Invoke-RestMethod -Uri "http://localhost:5000/v1/models" -Headers $headers
```

管理 UI 会通过 `<meta>` 标签自动注入密钥，因此浏览器中无需手动认证。

### 后台模式与服务管理

```sh
bun run omni start          # 启动两个服务
bun run omni status         # 检查状态
bun run omni stop           # 停止所有服务
bun run omni restart        # 重启服务
bun run omni restart --rebuild --host localhost  # 重建 Go 二进制和前端，然后重启
```

### 使用 bunx 运行

```sh
bunx omnillm@latest start
```

### 使用 Docker 运行

```sh
docker build -t omnillm .
docker run -p 4141:4141 -v $(pwd)/proxy-data:/root/.local/share/omnillm omnillm
```

自动生成的 API 密钥会保存在挂载卷中，因此重启会复用同一个密钥。若要设置已知密钥：

```sh
docker run -p 4141:4141 -e OMNILLM_API_KEY=my-secret-key omnillm
```

---

## 核心能力

### Canonical Intermediate Format (CIF)

所有传入请求，无论 API 形态如何（OpenAI 或 Anthropic），都会被解析为 **Canonical Intermediate Format**（`internal/cif/types.go`）。这个规范化数据模型是每个提供商适配器读写的统一格式，避免在 N 个提供商之间编写两两格式转换。新增提供商只需实现两个适配器（到/从 CIF），而不是 2N 个两两转换器。

### 统一 API 兼容性

通过单一网关提供 OpenAI 风格（`/v1/chat/completions`、`/v1/models`、`/v1/embeddings`、`/v1/responses`）和 Anthropic 风格（`/v1/messages`、`/v1/messages/count_tokens`）端点。现有应用只需将 `OPENAI_BASE_URL` 或 `ANTHROPIC_BASE_URL` 指向 OmniLLM，即可用最小集成改动完成迁移。

### 多提供商路由与自动故障转移

可同时管理多个提供商。无需重启即可切换活动后端。提供商优先级支持自动故障转移：当某个提供商在请求过程中失败时，OmniLLM 会按优先级顺序透明地尝试下一个候选提供商。

**提供商前缀模型路由** — 当多个提供商提供相同模型名称时，可以在模型名前加上提供商 ID 或副标题来消除歧义（例如 `provider-id:gpt-4o`）。这同时适用于直接模型名和虚拟模型上游，并由模型路由层自动解析。

### 虚拟模型

定义抽象模型 ID，将其映射到特定提供商模型，并可配置负载均衡策略（轮询、随机、优先级、加权）。这在应用代码和上游提供商之间建立抽象层，让你无需修改客户端配置即可替换模型。

### 集中式认证

通过 CLI 和管理工作流进行提供商认证：GitHub Copilot 使用 OAuth 设备码流程，Alibaba DashScope 使用 API key，其他提供商使用基于 token 的认证。凭证会持久化到 SQLite（纯 Go，无 CGO），并在重启时恢复。

### 运维可观测性

管理控制台提供提供商状态、模型发现、用量可见性、运行时元数据、基于 SSE 和 WebSocket 的实时日志流，以及动态日志级别控制。请求日志包含客户端 IP 和 User-Agent，用于识别发起请求的工具（Claude Code、Codex、Droid 等）。

### 配置文件管理（ToolConfig）

可直接在管理 UI 中查看、编辑、导入和备份常见 AI 编程智能体的配置文件：Claude Code、Codex、Droid、OpenCode 和 Amp。结构化编辑器为每种配置格式提供直观 UI。编辑功能受 `--enable-config-edit` 标志保护。配置备份会在同一目录创建带时间戳的副本。

### 流式传输韧性

如果上游 SSE 流在发送任何数据前失败（连接错误、超时），OmniLLM 会自动以非流式请求重试，并在本地将完成后的响应重新流式传输给客户端。Alibaba 适配器使用此机制来绕过 DashScope SSE 可靠性问题。

### 开发者与智能体集成

可作为 Claude Code 和其他客户端的本地网关。自动生成的 API 密钥会通过 `<meta>` 标签注入管理 UI，实现无缝前端认证。前端会在运行时探测已知端口，自动检测后端端口。

---

## 支持的提供商

| 提供商 | 认证 | 说明 |
|---|---|---|
| **GitHub Copilot** | OAuth 设备码流程或 token | 需要有效的 Copilot 订阅 |
| **Alibaba DashScope** | API key | 支持全球区和中国区；coding plan 变体 |
| **OpenAI-Compatible** | API key（可选） | 任意 OpenAI 兼容端点：Ollama、vLLM、LM Studio、llama.cpp、OpenAI |
| **Azure OpenAI** | API key | 可配置端点、API 版本和 deployments |
| **Google** | API key | 通用 Google 提供商 |
| **Kimi** | API key | 通用 Kimi 提供商 |
| **Antigravity** | Google OAuth | 需要 Google OAuth 客户端凭证 |

新提供商会使用由其端点 URL 和 API key 后缀派生的规范实例 ID 注册，确保重启后仍能稳定识别。

---

## 架构

```
Clients / Agents / Internal Apps
            |
            v
        OmniLLM Gateway
   ┌─────────────────────────────────────────┐
   │  Inbound API Key Auth                   │
   │  (Bearer / x-api-key / SSE query)       │
   ├─────────────────────────────────────────┤
   │  Ingestion Layer                        │
   │  OpenAI format ──┐                      │
   │  Anthropic format├─► CIF (Canonical     │
   │  Responses API  ─┘   Intermediate       │
   │                      Format)            │
   ├─────────────────────────────────────────┤
   │  Model Routing + Priority Failover      │
   │  Virtual Model Resolution               │
   │  Rate Limiting + Manual Approval        │
   ├─────────────────────────────────────────┤
   │  Provider CIF Adapters                  │
   │  (Execute / ExecuteStream)              │
   ├─────────────────────────────────────────┤
   │  Serialization Layer                    │
   │  CIF ──► OpenAI format / Anthropic      │
   │         format (SSE or non-streaming)   │
   ├─────────────────────────────────────────┤
   │  Admin Console + SSE/WS Log Streaming   │
   │  SQLite Persistence (provider, tokens,  │
   │  configs, chat sessions, virtual models)       │
   └─────────────────────────────────────────┘
            |
            v
  GitHub Copilot / Alibaba DashScope / Azure OpenAI
  / Google / Kimi / Antigravity / Ollama / vLLM
  / OpenAI / any OpenAI-compatible endpoint
```

### 关键组件

| 包 | 路径 | 用途 |
|---|---|---|
| **Server** | `internal/server/` | Gin HTTP 服务器、路由注册、CORS、认证中间件、管理 UI 服务、SSE/WebSocket 日志流 |
| **Routes** | `internal/routes/` | chat、models、embeddings、messages、responses、usage、token、admin、配置文件、虚拟模型的 HTTP 处理器 |
| **Ingestion** | `internal/ingestion/` | 将传入的 OpenAI/Anthropic/Responses 请求解析为 `cif.CanonicalRequest`，并对畸形输入快速失败校验 |
| **CIF** | `internal/cif/` | Canonical Intermediate Format，即所有提供商都转换到/从其转换的规范化请求/响应数据模型 |
| **Serialization** | `internal/serialization/` | 将 CIF 响应转换回客户端期望的 API 格式（OpenAI SSE、Anthropic SSE、非流式 JSON） |
| **Providers** | `internal/providers/` | 各提供商实现（Copilot、Alibaba、Azure、Google、Kimi、Antigravity、OpenAI-Compatible、Generic） |
| **Registry** | `internal/registry/` | 线程安全的 `ProviderRegistry`，管理已注册提供商、活动提供商选择和故障转移 |
| **Model Routing** | `internal/lib/modelrouting/` | 解析模型名称到候选提供商，并带缓存 |
| **Virtual Model Routing** | `internal/lib/virtualmodelrouting/` | 使用负载均衡策略将抽象虚拟模型 ID 路由到具体提供商模型 |
| **Database** | `internal/database/` | 通过 `modernc.org/sqlite` 进行 SQLite 持久化（纯 Go，无 CGO），保存提供商实例、token、配置、聊天会话和虚拟模型 |
| **Security** | `internal/security/` | OpenAI 兼容端点的 SSRF 防护 |
| **Rate Limiting** | `internal/lib/ratelimit/` | 可配置限流器，支持拒绝时排队等待行为 |
| **Approval** | `internal/lib/approval/` | 手动请求审批模式（`--manual` 标志） |
| **GitHub Service** | `internal/services/github/` | GitHub Copilot 专用逻辑（token 刷新、用量、配额） |
| **Frontend** | `frontend/` | React 19 + Vite + MUI/Tailwind v4 管理控制台 |

### 请求流程

1. 客户端向 OmniLLM 发送请求（例如 `POST /v1/chat/completions`）
2. 认证中间件校验入站 API key（Bearer / x-api-key / query param）
3. 日志中间件生成请求 ID，捕获客户端 IP 和 User-Agent
4. 限流器检查节流；若启用 `--manual` 模式，则请求操作员审批
5. Ingestion 解析器将请求体反序列化为 `cif.CanonicalRequest`
6. 模型路由规范化模型名称，并从 registry 解析候选提供商
7. 提供商迭代按优先级顺序遍历候选项：
   - 通过 `GetAdapter()` 获取提供商的 CIF 适配器
   - 将模型名重映射为提供商内部命名
   - 调用 `Execute()`（非流式）或 `ExecuteStream()`（流式，返回 Go channel）
   - 失败时尝试下一个候选项（自动故障转移）
8. Serialization 将 CIF 响应转换回客户端期望的 API 格式
9. 返回响应，并记录完整生命周期的结构化日志

### 技术栈

**后端：** Go 1.23、Gin、zerolog、Cobra CLI、modernc.org/sqlite（纯 Go SQLite）

**前端：** React 19、Vite、MUI v7、Tailwind v4、Radix UI、Lucide icons、Sonner、TypeScript

**工具：** Bun（运行时 + 包管理器）、ESLint、simple-git-hooks + lint-staged

**部署：** 多阶段 Dockerfile（`golang:1.23-alpine` → `oven/bun:1.2.19-alpine` → `alpine:3.20`）、独立二进制，或 `bunx omnillm@latest`

---

## 安全

OmniLLM 引入多项安全控制，用于保护网关和上游提供商。

### 入站 API 认证

所有 API 路由都受 API 密钥保护。密钥按以下顺序解析：

1. `--api-key` CLI 标志
2. `OMNILLM_API_KEY` 环境变量
3. 持久化文件 `~/.config/omnillm/api-key`
4. 自动生成的随机密钥（持久化到上述文件）

认证接受 `Authorization: Bearer <key>`、`x-api-key: <key>`，或用于 SSE 流的 `?api_key=<key>` 查询参数。

### SSRF 防护

OpenAI 兼容提供商端点会进行 SSRF 攻击校验。除非设置 `--allow-local-endpoints`，否则会拒绝 localhost、loopback、private 和 link-local 地址。

```sh
# 允许本地端点（例如 localhost:11434 上的 Ollama）
bun run omni start --allow-local-endpoints
```

### CORS

CORS 限制为 `localhost`、`127.0.0.1` 和 `::1` 来源。EventSource 连接会获得适用于 SSE 流的响应头。

### Token 脱敏

`/token` 端点默认返回脱敏 token（仅显示前 4 位和后 4 位）。只有启用 `--show-token` 时才会暴露完整 token。

### 配置编辑保护

外部配置文件编辑（Claude Code、OpenCode 等）默认禁用，必须通过 `--enable-config-edit` 显式启用。

### 推荐的生产控制措施

- 在内部反向代理或 API 网关后运行
- 将管理访问限制为可信操作员
- 使用受控文件系统权限隔离持久化状态
- 避免在公网暴露诊断/token 端点
- 在组织共享使用前，审查每个提供商的合同和合规状态

---

## CLI 参考

### 命令

| 命令 | 用途 |
|---|---|
| `start` | 启动 OmniLLM 网关 |
| `auth` | 在不启动服务器的情况下认证提供商 |
| `check-usage` | 输出 GitHub Copilot 用量/配额信息 |
| `debug` | 输出运行时、版本和路径诊断信息 |
| `sync-names` | 从实时 API 元数据刷新提供商显示名称 |
| `provider` | 管理 LLM 提供商实例（list、add、delete、activate、deactivate、switch、rename、priorities、usage） |
| `model` | 管理提供商模型（list、refresh、toggle、version） |
| `virtualmodel` | 管理带负载均衡的虚拟模型别名（list、get、create、update、delete） |
| `config` | 管理外部工具配置文件（list、get、set、import、backup） |
| `settings` | 查看和更新服务器设置（get、set） |
| `status` | 显示服务器和认证状态 |
| `chat` | 交互式聊天 REPL，或管理已保存聊天会话（sessions、send） |
| `logs` | 流式查看或查看服务器日志（tail） |

### `start` 选项

| 选项 | 别名 | 默认值 | 描述 |
|---|---|---|---|
| `--port` | `-p` | `5000` | 监听端口 |
| `--provider` | | `github-copilot` | 活动提供商 |
| `--verbose` | `-v` | `false` | 启用详细日志 |
| `--account-type` | `-a` | `individual` | Copilot 计划（`individual`、`business`、`enterprise`） |
| `--rate-limit` | `-r` | none | 请求之间的最小秒数 |
| `--wait` | `-w` | `false` | 达到限流时排队/等待，而不是失败 |
| `--manual` | | `false` | 每个请求都需要手动审批 |
| `--github-token` | `-g` | none | 直接提供 GitHub token |
| `--claude-code` | `-c` | `false` | 引导式 Claude Code 设置 |
| `--show-token` | | `false` | 获取/刷新时打印 token |
| `--proxy-env` | | `false` | 从环境变量读取代理设置 |
| `--api-key` | | auto-generated | 用于路由保护的入站 API key |
| `--allow-local-endpoints` | | `false` | 允许 localhost/private OpenAI 兼容端点 |
| `--enable-config-edit` | | `false` | 允许通过管理 API 编辑外部配置文件 |

---

## API 兼容范围

### OpenAI 兼容端点

| 方法 | 端点 | 描述 |
|---|---|---|
| `POST` | `/v1/chat/completions` | Chat completions |
| `GET` | `/v1/models` | 列出可用模型 |
| `POST` | `/v1/embeddings` | 文本 embeddings |
| `POST` | `/v1/responses` | Responses API |

### Anthropic 兼容端点

| 方法 | 端点 | 描述 |
|---|---|---|
| `POST` | `/v1/messages` | Messages API |
| `POST` | `/v1/messages/count_tokens` | token 计数 |

### 工具端点

| 方法 | 端点 | 描述 |
|---|---|---|
| `GET` | `/usage` | 活动提供商用量和配额 |
| `GET` | `/token` | 当前提供商 token（默认脱敏） |
| `GET` | `/health` | 带时间戳的健康检查 |
| `GET` | `/healthz` | 最小健康检查 |

### Admin API

| 方法 | 端点 | 描述 |
|---|---|---|
| `GET` | `/api/admin/info` | 版本、端口、后端类型、运行时长 |
| `GET` | `/api/admin/status` | 服务器和提供商状态 |
| `GET` | `/api/admin/providers` | 列出提供商及其认证状态和模型数量 |
| `POST` | `/api/admin/providers/switch` | 切换活动提供商 |
| `POST` | `/api/admin/providers/add/:type` | 添加新提供商实例 |
| `POST` | `/api/admin/providers/auth-and-create/:type` | 一步完成认证并创建提供商 |
| `DELETE` | `/api/admin/providers/:id` | 删除提供商实例 |
| `GET` | `/api/admin/providers/:id/models` | 列出提供商模型 |
| `POST` | `/api/admin/providers/:id/models/refresh` | 从上游强制刷新模型列表 |
| `POST` | `/api/admin/providers/:id/models/toggle` | 启用/禁用模型 |
| `GET` | `/api/admin/providers/:id/models/:modelId/version` | 获取模型版本字符串 |
| `PUT` | `/api/admin/providers/:id/models/:modelId/version` | 设置模型版本字符串 |
| `GET` | `/api/admin/providers/:id/usage` | 提供商特定用量 |
| `POST` | `/api/admin/providers/:id/auth` | 认证提供商 |
| `POST` | `/api/admin/providers/:id/auth/initiate-device-code` | 启动 OAuth 设备码流程 |
| `POST` | `/api/admin/providers/:id/auth/complete-device-code` | 完成 OAuth 设备码流程 |
| `PUT` | `/api/admin/providers/:id/config` | 更新提供商配置 |
| `POST` | `/api/admin/providers/:id/activate` | 激活提供商 |
| `POST` | `/api/admin/providers/:id/deactivate` | 停用提供商 |
| `GET` | `/api/admin/providers/priorities` | 获取提供商故障转移优先级 |
| `POST` | `/api/admin/providers/priorities` | 设置提供商故障转移优先级 |
| `GET` | `/api/admin/auth-status` | 当前 OAuth 流程状态 |
| `POST` | `/api/admin/auth/cancel` | 取消活动 OAuth 流程 |
| `GET` | `/api/admin/settings/log-level` | 获取当前日志级别 |
| `PUT` | `/api/admin/settings/log-level` | 动态修改日志级别 |
| `GET` | `/api/admin/logs/stream` | SSE 日志流 |
| `GET` | `/api/admin/chat/sessions` | 列出聊天会话 |
| `POST` | `/api/admin/chat/sessions` | 创建聊天会话 |
| `GET` | `/api/admin/chat/sessions/:id` | 获取带消息的聊天会话 |
| `PUT` | `/api/admin/chat/sessions/:id` | 更新聊天会话标题 |
| `POST` | `/api/admin/chat/sessions/:id/messages` | 向会话添加消息 |
| `DELETE` | `/api/admin/chat/sessions/:id` | 删除聊天会话 |
| `DELETE` | `/api/admin/chat/sessions` | 删除所有聊天会话 |
| `GET` | `/api/admin/config` | 列出可用配置文件 |
| `GET` | `/api/admin/config/:name` | 读取配置文件 |
| `PUT` | `/api/admin/config/:name` | 保存配置文件 |
| `POST` | `/api/admin/config/:name/import` | 从上传文件导入配置 |
| `POST` | `/api/admin/config/:name/backup` | 在同一目录创建带时间戳的备份 |
| `GET` | `/api/admin/virtualmodels` | 列出虚拟模型 |
| `POST` | `/api/admin/virtualmodels` | 创建虚拟模型 |
| `GET` | `/api/admin/virtualmodels/:id` | 获取虚拟模型详情 |
| `PUT` | `/api/admin/virtualmodels/:id` | 更新虚拟模型 |
| `DELETE` | `/api/admin/virtualmodels/:id` | 删除虚拟模型 |

---

## Claude Code 集成

### 引导式设置

```sh
bun run start --claude-code
```

### 手动 `.claude/settings.json` 示例

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:5000",
    "ANTHROPIC_AUTH_TOKEN": "dummy",
    "ANTHROPIC_MODEL": "gpt-4.1",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "gpt-4.1",
    "ANTHROPIC_SMALL_FAST_MODEL": "gpt-4.1",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "gpt-4.1",
    "DISABLE_NON_ESSENTIAL_MODEL_CALLS": "1",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

### 注意：将 Claude Code 与 GitHub Copilot 上游一起使用

当通过 OmniLLM 将 Claude Code 路由到 GitHub Copilot 上游提供商时，**GitHub Copilot 按请求计费**，不是按 token 计费。Claude Code 会产生许多小型后台调用（子智能体、工具使用、haiku 级模型），这些调用会在不易察觉的情况下累积。

将 small/fast 模型覆盖为免费或低成本模型：

```sh
export ANTHROPIC_DEFAULT_HAIKU_MODEL=qwen3.6-plus
export ANTHROPIC_SMALL_FAST_MODEL=qwen3.6-plus
export CLAUDE_CODE_SUBAGENT_MODEL=qwen3.6-plus
```

或在 `.claude/settings.json` 中：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:5000",
    "ANTHROPIC_MODEL": "claude-haiku-4.5",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "qwen3.6-plus",
    "ANTHROPIC_SMALL_FAST_MODEL": "qwen3.6-plus",
    "CLAUDE_CODE_SUBAGENT_MODEL": "qwen3.6-plus",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

---

## ToolConfig - AI 助手配置管理

OmniLLM 在管理 UI 中提供统一的 **ToolConfig** 界面，用于管理常见 AI 编程助手的配置文件。访问 `http://localhost:5080/admin/` → **ToolConfig**。

![ToolConfig 界面](docs/assets/toolconfig.png)

### 支持的工具

| 工具 | 配置文件 | 模板 | 文档 |
|------|-------------|----------|---------------|
| **[Claude Code](https://claude.ai/code)** | `~/.claude/settings.json` | [示例](.claude/settings.json.example) | [官方文档](https://code.claude.com/docs/en/settings) |
| **[Codex](https://github.com/openai/codex)** | `~/.codex/config.toml` | [示例](.codex/config.toml.example) | [官方文档](https://developers.openai.com/codex/config-reference) |
| **[Droid](https://docs.factory.ai/cli)** | `~/.factory/settings.json` | [示例](.factory/settings.json.example) | [官方文档](https://docs.factory.ai/cli/byok/overview#understanding-providers) |
| **[OpenCode](https://opencode.ai)** | `~/.opencode/config.json` | [示例](.opencode/config.json.example) | [官方文档](https://opencode.ai/docs/config/) |
| **[AMP](https://ampcode.com)** | `~/.amp/config.json` | [示例](.amp/config.json.example) | [官方文档](https://ampcode.com/manual#configuration) |

### 功能

- **结构化编辑器** — 无需手动编辑 JSON/TOML，即可通过直观 UI 编辑配置
- **提供商下拉框** — Droid 配置包含一个下拉框，支持 3 种 Droid 提供商类型：Anthropic（v1/messages）、OpenAI（Responses API）、Generic（Chat Completions API）
- **备份按钮** — 一键备份会在同一目录创建带时间戳的副本（例如 `~/.codex/config.20060102_150405.toml`）
- **模板文件** — 可直接使用的快速设置示例（[查看所有模板](docs/CONFIG_TEMPLATES.md)）
- **实时校验** — 保存前捕获错误
- **自动创建文件** — 如果配置不存在，会自动创建
- **导入/导出** — 备份和恢复配置

### ToolConfig 快速开始

1. **启动 OmniLLM：**
   ```sh
   bun run omni start --enable-config-edit
   ```

2. **打开 ToolConfig UI：**
   - 导航到 http://localhost:5080/admin/
   - 点击侧边栏中的 **ToolConfig**

3. **配置你的工具：**
   - 点击任意工具卡片（如果配置不存在，会显示 “○ new”）
   - 在结构化编辑器中编辑设置
   - 在修改前点击 **Backup**（位于 Save 旁）创建带时间戳的副本
   - 点击 **Save** 持久化更改

4. **使用模板（可选）：**
   ```sh
   # 将模板复制到实际位置
   cp ~/.factory/settings.json.example ~/.factory/settings.json
   
   # 或使用 ToolConfig UI 自动创建
   ```

### 文档与参考

- **[配置模板](docs/CONFIG_TEMPLATES.md)** - 所有模板文件的完整指南
- **[ToolConfig 修复](docs/TOOLCONFIG_FIXES.md)** - 路径修复和保存/重新加载改进
- **[结构化编辑器](docs/STRUCTURED_EDITORS.md)** - 结构化编辑器实现细节
- **[布局改进](docs/LAYOUT_IMPROVEMENTS.md)** - UI/UX 增强

### 安全说明

出于安全考虑，配置文件编辑默认 **禁用**。使用 `--enable-config-edit` 标志启用：

```sh
bun run omni start --enable-config-edit
```

这可以防止 OmniLLM 作为共享服务运行时，AI 助手配置被未经授权地修改。

---

## 运维与故障排除

### 常见检查

- 确认服务正在预期端口监听
- 打开 `/admin/` 并验证提供商认证状态
- 运行 `bun run debug` 检查运行时路径和 token 是否存在
- 验证 Copilot 配额行为时运行 `bun run check-usage`
- 在路由流量前，确认活动提供商有可用模型

### 常见故障模式

| 现象 | 可能原因 | 处理方式 |
|---|---|---|
| 没有已认证的提供商 | 认证流程未完成 | 运行 `omnillm auth` 或在管理 UI 中认证 |
| 401 / Unauthorized | 提供商凭证过期或无效，或缺少入站 API key | 重新认证提供商；在请求中包含 `Authorization: Bearer <api-key>` |
| 未返回模型 | 提供商认证未完成或上游问题 | 重新检查认证和提供商可用性 |
| 限流失败 | 请求量超过配置阈值 | 增大间隔或使用 `--wait` |
| 端点被拒绝 | SSRF 防护阻止 localhost/private IP | 对 Ollama 等本地服务使用 `--allow-local-endpoints` |

---

## 开发

```sh
bun install

# 启动后端 + 前端（前台）
bun run dev

# 自定义端口
bun run dev --server-port 8080 --frontend-port 3000

# 后台模式
bun run omni start
bun run omni stop

# 仅后端（TypeScript）
bun run dev:server

# 仅前端
bun run dev:frontend

# 生产构建
bun run build

# 类型检查
bun run typecheck
```

前端源码位于 `frontend/`，使用 Vite + React + Tailwind v4。在开发模式下，Vite 会将 `/api/*`、`/v1/*` 和 `/usage` 代理到 Go 后端。

前端会在运行时通过探测已知端口自动检测后端端口，因此即使在非标准端口上提供服务也能正常工作。

---

## 许可证

MIT

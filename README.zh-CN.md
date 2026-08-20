# OmniLLM

<p align="center">统一的 LLM 网关与控制平面，兼容 OpenAI 和 Anthropic 客户端。</p>

<p align="center"><a href="README.md">English</a></p>

OmniLLM 位于应用、智能体、编程工具与上游模型提供商之间。它会先把请求归一化为 CIF（Canonical Intermediate Format，规范中间格式），再做模型解析、提供商选择、故障转移和响应序列化，最后返回客户端期望的 API 形态。

直接效果是：你的客户端只需要接入一次 OmniLLM，就能在不重写业务代码的前提下切换提供商、做故障转移、管理虚拟模型、查看用量与日志，并使用统一的管理后台。

## 二进制入口

当前仓库面向用户提供一个入口：

| 二进制 | 入口文件 | 作用 |
|---|---|---|
| `omnillm` | `main.go` | 主服务与管理 CLI。用于启动网关、管理提供商、模型、虚拟模型、聊天、设置、配置文件和日志。 |

使用 `omnillm` 启动网关并执行全部管理命令。面向编程场景的 `omnicode` CLI 已迁移到独立仓库。

## 这个项目解决什么问题

常见痛点通常包括：

- 每个提供商都有不同的 API 形态
- 一旦某个提供商失败，客户端就要自己兜底
- 本地或团队工具需要分别管理不同的密钥和配置
- 很难统一查看请求来源、延迟、用量和路由结果

OmniLLM 把这些能力集中到一个入口，提供：

- OpenAI 兼容端点：`/v1/chat/completions`、`/v1/models`、`/v1/embeddings`、`/v1/responses`
- Anthropic 兼容端点：`/v1/messages`、`/v1/messages/count_tokens`
- 提供商优先级和自动故障转移
- 支持轮询、随机、优先级、加权策略的虚拟模型
- 管理后台，用于提供商、日志、聊天会话、计量、访问令牌、设置和 ToolConfig

## 界面截图

![OmniLLM 管理控制台](docs/assets/admin-console.png)

其他页面：

- Chat: ![聊天界面](docs/assets/chat.png)
- 虚拟模型: ![虚拟模型](docs/assets/virtual-models.png)
- ToolConfig: ![ToolConfig](docs/assets/toolconfig.png)

## 快速开始

### 方式一：直接运行已发布包

```sh
bunx omnillm@latest start
```

运行时默认地址：

- API 服务：`http://127.0.0.1:5000`
- 管理后台：`http://127.0.0.1:5000/admin/`

### 方式二：从源码运行

前置要求：

- Bun 1.2+
- Go 1.27+

构建并运行主二进制：

```sh
make install
omnillm start
```

Windows PowerShell：

```powershell
make install
omnillm.exe start
```

前端构建、lint、测试和开发模式请直接使用对应的 Bun 脚本。

### API 密钥行为

所有 API 和管理路由都受保护。入站密钥解析顺序为：

1. `--api-key`
2. `OMNILLM_API_KEY`
3. `~/.config/omnillm/api-key`
4. 如果都没有，则自动生成并持久化到上述文件

示例：

```sh
omnillm start --api-key my-secret-key
curl -H "Authorization: Bearer my-secret-key" http://127.0.0.1:5000/v1/models
curl -H "x-api-key: my-secret-key" http://127.0.0.1:5000/v1/models
```

Windows PowerShell：

```powershell
$env:OMNILLM_API_KEY = "my-secret-key"
omnillm start
Invoke-RestMethod -Uri "http://127.0.0.1:5000/v1/models" -Headers @{ Authorization = "Bearer my-secret-key" }
```

浏览器中的管理后台会自动注入这个密钥，因此前端访问管理 API 时通常不需要手工填写 token。

## 开发模式与运行模式

当前代码库实际上有两种模式。

### 运行模式

- 一个 Go 服务器
- 默认端口 `5000`
- 同时提供 API 和 `/admin/`

启动方式：

```sh
omnillm start
```

### 开发模式

- Go 后端默认跑在 `5002`
- Vite 前端默认跑在 `5080`
- 管理后台通过 Vite 开发服务器的 `/admin/` 提供

启动方式：

```sh
bun run dev
```

常用命令：

```sh
bun install
bun run build
bun run lint:all
bun run typecheck
bun test
bun run dev
bun run dev:frontend
omnillm status
omnillm restart
omnillm logs --follow
```

Make 仅提供 `make build`、`make install`、`make uninstall`、
`make build-desktop-sidecar`、`make build-desktop` 和 `make desktop-dev`。
`make uninstall` 会从 Go 的二进制安装目录中删除已安装的 `omnillm`，
以及遗留的 `omniproxy` 可执行文件。

其中 `bun run omni` 是开发环境管理器，用来同时管理后端二进制和 Vite 服务，不等同于生产或发布时的运行方式。

## 核心能力

### 统一 API 兼容层

现有 OpenAI 风格和 Anthropic 风格客户端都可以直接指向 OmniLLM，由网关完成上游差异处理。

### CIF 归一化

请求会先进入 CIF，再做调度与序列化。这样新增提供商时，通常只需要实现与 CIF 的双向适配，而不是写一整套两两转换逻辑。

### 响应缓存

OmniLLM 为 Chat Completions、Anthropic Messages 和 OpenAI Responses 提供可选的精确输入响应缓存，同时支持非流式和流式请求。该功能保持严格的显式启用（opt-in），并在 CIF 层工作，因此通过一种受支持 API 形态或流式模式写入的规范结果，可以为其他兼容调用方重新序列化。

- 缓存资格不依赖采样设置：无论 `temperature` 和 `top_p` 是省略、为零还是非零，请求都可以缓存。采样参数及其他受支持的生成控制仍参与语义键计算，因此只有生成语义相同的请求才能重放同一条目。
- 命中时不会再次执行上游推理，而是重放先前存储的规范模型结果。对于随机采样请求，这意味着 TTL 内可能再次返回之前的随机结果；请仅在能接受这种重放行为的场景中启用缓存。
- 重放保证规范内容和工具调用语义，而不是原始 HTTP 字节、SSE 分块边界或事件时序。OmniLLM 会把存储的规范响应序列化为当前调用方使用的 API 形态和流式模式。

启用状态和 TTL 保存在持久化的 SQLite 运行时配置中，并按请求实时读取。规范响应负载与命中计数只保存在另行提供的 Redis 或 Valkey 服务中：

| 设置 | 默认值 | 含义 |
|---|---|---|
| `response_cache.enabled` | `false`（显式启用） | 启用响应缓存 |
| `response_cache.ttl_seconds` | `60`（60 秒） | 未配置有效正数 TTL 时使用的 Redis 原生过期时间；已配置的正数 TTL 仍具有优先权 |
| `--response-cache-redis-url` / `OMNILLM_RESPONSE_CACHE_REDIS_URL` | `redis://127.0.0.1:6379/0` | Redis URL；显式参数优先于环境变量 |
| `--response-cache-redis-prefix` / `OMNILLM_RESPONSE_CACHE_REDIS_PREFIX` | `omnillm` | 命名空间前缀；显式参数优先于环境变量 |

启动本地服务并启用缓存：

```sh
docker run --rm --name omnillm-redis -p 6379:6379 redis:8-alpine
omnillm start --response-cache-redis-url redis://127.0.0.1:6379/0
omnillm settings set response-cache on --ttl 60
omnillm cache stats
```

`redis://user:password@host:6379/0` 和 `rediss://user:password@host:6380/0` URL 支持认证与 TLS。OmniLLM 不会返回或记录包含凭据的 URL。在容器中，`127.0.0.1` 指向应用容器本身，而不是同组的 Redis 服务；请使用其服务主机名，例如 `redis://redis:6379/0`。

Redis 是可选的加速基础设施。如果 URL 解析、启动 ping、认证、读取或写入失败，OmniLLM 会把缓存后端报告为降级状态，并继续正常执行上游模型，不会回退到 SQLite。健康检查仍保持正常，有限的恢复探测会在 Redis 恢复后重新启用缓存。升级时会有意丢弃旧的临时 SQLite 响应行并冷启动。现有条目保留写入时设置的 Redis 原生 TTL；修改 TTL 只影响新写入和刷新的条目，不能恢复已经过期的键。

可通过 `X-OmniLLM-Cache` 请求头逐请求覆盖：`bypass` 跳过读取并强制刷新（仍会写入），`off` 同时跳过读取和写入。管理统计与清理操作仅作用于带版本的 OmniLLM 命名空间，不会清空无关 Redis 数据。

`omnillm cache stats` 会报告当前规范响应负载字节数、查找命中与未命中次数、命中率，以及当前 Redis 统计窗口的开始时间。只有符合条件的精确响应查找才计数；绕过请求和 Redis 故障不计数。清理响应缓存命名空间会同时重置统计窗口与计数器。`total_hits` 仍是当前存活条目所附命中数的兼容聚合，因此会随条目过期而下降。使用 `--output json` 可查看完整的已认证设置响应。

该精确响应缓存与**提供商提示词缓存**彼此独立。提示词缓存指令不会启用或改变精确响应缓存；除非另有独立的精确响应条目命中，提供商提示词缓存命中仍会执行上游推理；精确响应命中也不会被报告为提供商提示词缓存活动。使用 `omnillm usage` 查看提供商提示词缓存的请求与令牌指标；使用 `omnillm cache stats` 查看本地精确响应查找统计。

Redis 条目使用 OmniLLM 带版本的命名空间和规范响应格式。该格式不与 LiteLLM 的 Redis 缓存格式逐字节兼容，两者不能互换使用。

### 提供商路由与故障转移

模型解析支持直接模型名、带提供商前缀的模型名，以及按候选提供商顺序自动回退。

### 虚拟模型

你可以给客户端暴露一个稳定的模型 ID，在后台把它映射到一个或多个真实上游，并定义轮询、随机、优先级或加权策略。

### 管理后台

管理 API 和 Web UI 目前覆盖：

- 提供商接入、激活、重命名、优先级、用量
- 模型发现、刷新、启停、版本信息
- 基于 SSE 的实时日志
- 按提供商、模型、客户端聚合的计量统计
- 内置聊天会话
- 访问令牌管理
- 外部工具配置文件管理

### ToolConfig 与编程工具接入

OmniLLM 可以统一管理 Claude Code、Codex、Droid、OpenCode、AMP 等工具的配置文件。出于安全考虑，编辑外部配置文件需要显式开启 `--enable-config-edit`。

## 支持的提供商

当前代码库中面向用户的接入流程支持以下提供商类型：

| 提供商 | 认证方式 | 说明 |
|---|---|---|
| GitHub Copilot | OAuth 设备码或 token | CLI 默认启动提供商 |
| OpenAI-Compatible | API key，部分上游可选 | Ollama、vLLM、LM Studio、OpenAI、llama.cpp 等 |
| Alibaba DashScope | API key | 支持 region 与 plan 变体 |
| Azure OpenAI | API key | 支持自定义 endpoint 和 deployment |
| Google | API key | 通用 Google 提供商 |
| Kimi | API key | 通用 Kimi 提供商 |
| Codex | API key | OpenAI Codex 集成 |
| Antigravity | Google OAuth | 通过管理后台 OAuth 流程接入 |

如果你要新增一个提供商，可以参考 [docs/ADDING_A_PROVIDER.md](docs/ADDING_A_PROVIDER.md)。

## API 兼容面

### OpenAI 兼容路由

| 方法 | 路由 |
|---|---|
| `POST` | `/v1/chat/completions` |
| `GET` | `/v1/models` |
| `GET` | `/v1/models/metadata` |
| `POST` | `/v1/embeddings` |
| `POST` | `/v1/responses` |

### Anthropic 兼容路由

| 方法 | 路由 |
|---|---|
| `POST` | `/v1/messages` |
| `POST` | `/v1/messages/count_tokens` |

### 工具路由

| 方法 | 路由 |
|---|---|
| `GET` | `/health` |
| `GET` | `/healthz` |
| `GET` | `/usage` |
| `GET` | `/token` |

### 管理路由

高频使用的管理路由分组包括：

- `/api/admin/providers`
- `/api/admin/virtualmodels`
- `/api/admin/metering/*`
- `/api/admin/chat/sessions`
- `/api/admin/access-tokens`
- `/api/admin/config`
- `/api/admin/settings/log-level`
- `/api/admin/logs/stream`

## CLI 概览

主二进制 `omnillm` 当前包含：

- `start`
- `auth`
- `usage`
- `check-usage`
- `sync-names`
- `debug`
- `provider`
- `model`
- `virtualmodel`
- `config`
- `settings`
- `status`
- `chat`
- `logs`

服务端二进制常用 `start` 参数：

| 参数 | 默认值 | 用途 |
|---|---|---|
| `--port` | `5000` | 服务端口 |
| `--host` | `127.0.0.1` | 绑定地址 |
| `--provider` | `github-copilot` | 启动时默认提供商 |
| `--api-key` | 缺省时自动生成 | 入站认证密钥 |
| `--manual` | `false` | 手工审批模式 |
| `--rate-limit` | `0` | 请求最小间隔秒数 |
| `--wait` | `false` | 限流时等待而不是报错 |
| `--allow-local-endpoints` | `false` | 允许 localhost 和私网 OpenAI 兼容上游 |
| `--enable-config-edit` | `false` | 允许通过管理 API 编辑外部配置文件 |
| `--claude-code` | `false` | 输出 Claude Code 的引导配置 |

## 仓库结构速览

| 路径 | 作用 |
|---|---|
| `main.go` | `omnillm` CLI 主入口 |
| `internal/server/` | 服务启动、认证、管理 UI 注册 |
| `internal/routes/` | OpenAI、Anthropic、Responses、admin、metering、chat、config、virtual model 路由处理 |
| `internal/providers/` | 各提供商实现与适配器 |
| `internal/cif/` | CIF 类型定义 |
| `internal/serialization/` | 返回格式序列化 |
| `internal/database/` | SQLite 持久化 |
| `frontend/` | React 19 + Vite 管理前端源码 |
| `pages/admin/` | 运行模式下由 Go 服务直接托管的已构建前端 |
| `tests/` | Bun 驱动的 API、前端、浏览器行为测试 |
| `docs/` | 迁移记录、关键变更文档、实现指南 |

## 架构快照

```text
客户端或工具
  -> OmniLLM API/认证层
  -> 进入 CIF
  -> 模型解析与虚拟模型路由
  -> 提供商适配器执行
  -> 序列化为 OpenAI、Anthropic 或 Responses 形态
  -> 计量、日志、持久化、后台可视化
```

后端技术栈：

- Go 1.27
- Gin
- Cobra
- zerolog
- modernc.org/sqlite

前端技术栈：

- React 19
- Vite
- TypeScript
- Material UI 7
- Tailwind CSS 4

## 安全说明

当前内建的安全控制包括：

- API 和管理路由统一使用 API key 保护
- 对 OpenAI-compatible 上游地址做 SSRF 检查
- 面向浏览器的 localhost 风格 CORS 策略
- 默认对 token 做脱敏显示，除非开启 `--show-token`
- 编辑外部配置文件需要显式开启 `--enable-config-edit`

如果不是纯本地或内网使用，建议把 OmniLLM 放在你自己的反向代理或网关之后，并限制管理后台的访问范围。

## 构建、检查与测试

```sh
bun run build
bun run build:go
bun run lint
bun run lint:all
bun run typecheck
bun test
bun run test:frontend
```

`scripts/` 目录里还包含依赖环境的 live 测试脚本，例如模型矩阵和特定提供商验证。

## 规范驱动开发

OpenSpec 是 OmniLLM 行为的权威来源。修改代码前，请阅读[当前状态规范](openspec/specs/)、强制性的[代理工作流](CLAUDE.md)和[贡献指南](CONTRIBUTING.md)。所有代码、测试、依赖项或构建/运行时配置变更都必须包含经过严格验证和人工批准的 OpenSpec 变更；CI 会通过 `bun run spec:check` 强制执行此要求。

[`docs/`](docs/README.md) 中的现有资料仅作为辅助或历史背景，不能覆盖当前状态规范。

## 进一步阅读

推荐先看：

- [docs/README.md](docs/README.md)
- [docs/ADDING_A_PROVIDER.md](docs/ADDING_A_PROVIDER.md)
- [docs/CIF_MIGRATION.md](docs/CIF_MIGRATION.md)
- [docs/CONFIG_TEMPLATES.md](docs/CONFIG_TEMPLATES.md)

`docs/` 目录还保留了大量关于提供商兼容、流式处理、路由、前端和协议迁移的关键变更记录。

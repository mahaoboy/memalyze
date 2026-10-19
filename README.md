# memalyze

**MCP-native, self-hosted memory for AI agents.**

Plug into any harness. Keep every memory. Never leave your infrastructure.

> **Not another mem0 — and not Basic Memory either.** Three things nobody ships together:
>
> 1. **Enterprise governance out of the box** — multi-tenant isolation, audit log, TTL / right-to-be-forgotten.
> 2. **One static binary, fully offline** — no HuggingFace download, no LLM dependency, no graph database.
> 3. **Middleware, not a framework** — ≤5 tools over MCP; any harness plugs in, nothing gets taken over.

---

🚧 **早期开发中** — 核心链路已通（存储 / 写入 / 检索 / MCP 三件套，S12）。下一步：上下文注入与端到端验证。

## 为什么不同

| | 云端记忆服务 | 框架内置记忆 | 个人知识库（Basic Memory） | **memalyze** |
|---|---|---|---|---|
| 数据位置 | 厂商云 | 随框架 | 本地 Markdown | **你自己的服务器** |
| 接入方式 | 私有 API | 绑定框架 | MCP | **MCP（任何 harness）** |
| 企业治理 | ❌ | ❌ | ❌ | **多租户 / 审计 / TTL** |
| 部署 | SaaS | 跟着框架走 | 173 个 Python 包 | **单二进制 / 5 分钟 Docker** |
| 离线可用 | ❌ | 部分 | ⚠️ 语义检索要下 HuggingFace 模型 | **✅ 本地 embedding，完全离线** |
| 工具面 | — | — | 23 个工具 | **≤5 个** |

## 快速开始

需要本地 [Ollama](https://ollama.com) 与中文 embedding 模型（离线可用，不依赖任何云服务）：

```bash
ollama pull qwen3-embedding:0.6b   # 1024 维，默认模型
go build -o memalyze ./cmd/memalyze

# Claude Code
claude mcp add memalyze -- "$(pwd)/memalyze"

# 任意 MCP 客户端（stdio）
mcporter list --stdio "$(pwd)/memalyze"
```

### MCP 工具

| 工具 | 作用 |
|---|---|
| `memory_add` | 写入记忆；返回 `duplicate` / `embedded` / `embedding_failed` |
| `memory_search` | 向量优先 + 关键词补位；返回 `degraded` / `degrade_reason` |
| `memory_delete` | 按 id 软删除（已删除可重试） |
| `echo` | 链路自检，不含业务逻辑 |

### 配置

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `MEMALYZE_DB` | `./memalyze.db` | SQLite 文件路径 |
| `MEMALYZE_OLLAMA_URL` | `http://127.0.0.1:11434` | 本地 Ollama 地址 |
| `MEMALYZE_EMBED_MODEL` | `qwen3-embedding:0.6b` | embedding 模型 |
| `MEMALYZE_TENANT` | `default` | 工具调用的默认租户 |

Ollama 暂时连不上时服务仍会启动：写入会保留记忆并返回 `embedding_failed=true`，检索会返回 `degraded=true` 并自动降级为纯关键词。

```bash
mcporter call --stdio "$(pwd)/memalyze" memory_add content="许可证定为 AGPL-3.0" scope=project:opc
mcporter call --stdio "$(pwd)/memalyze" memory_search query="许可证"
```

## 状态

- [x] 立项与定位
- [x] 技术选型（Go / SQLite→Postgres / MCP）
- [x] 数据模型设计
- [x] 环境与 MCP 跑通（`echo` 工具已在 claude-code / mcporter 验证）
- [x] 存储层（SQLite + 二元组分词 FTS5 + 向量检索）
- [x] 写入 API（重复检测 / 向量化失败不阻塞 / Reembed）
- [x] 检索编排（向量优先 / 关键词补位 / 3 秒超时降级）
- [x] MCP 工具三件套（`memory_add` / `memory_search` / `memory_delete`）
- [ ] 上下文注入与端到端验证（S13 / S14）

## 许可证

AGPL-3.0（+ 商业双许可）。见 [LICENSE](LICENSE)。

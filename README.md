# memalyze

**MCP-native, self-hosted memory for AI agents.**

Plug into any harness. Keep every memory. Never leave your infrastructure.

> **Not another mem0 — and not Basic Memory either.** Three things nobody ships together:
>
> 1. **Enterprise governance out of the box** — multi-tenant isolation, audit log, TTL / right-to-be-forgotten.
> 2. **One static binary, fully offline** — no HuggingFace download, no LLM dependency, no graph database.
> 3. **Middleware, not a framework** — ≤5 tools over MCP; any harness plugs in, nothing gets taken over.

---

🚧 **早期开发中** — 骨架 + MCP hello-world（[任务 S6](../../文档/mahao/mahao/一人企业OPC/07-执行计划/任务分解.md)）。

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

当前只暴露一个 `echo` 工具，用于验证链路：工具可见、可调用。

## 状态

- [x] 立项与定位
- [x] 技术选型（Go / SQLite→Postgres / MCP）
- [x] 数据模型设计
- [x] 环境与 MCP 跑通（`echo` 工具已在 claude-code / mcporter 验证）
- [x] 存储层（SQLite + 二元组分词 FTS5 + 向量检索）
- [ ] 写入 API（去重 / 异步 embedding）
- [ ] 检索排序（向量优先 + 降级）
- [ ] MCP 工具三件套（`memory_add` / `memory_search` / `memory_delete`）

## 许可证

AGPL-3.0（+ 商业双许可）。见 [LICENSE](LICENSE)。

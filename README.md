# memalyze

**MCP-native, self-hosted memory for AI agents.**

Plug into any harness. Keep every memory. Never leave your infrastructure.

> **Not another mem0**: this is a *middleware* over MCP — not a cloud service and not a framework.
> You self-host it, your data never leaves your machine, and any harness that speaks MCP can use it.

---

🚧 **早期开发中** — 骨架 + MCP hello-world（[任务 S6](../../文档/mahao/mahao/一人企业OPC/07-执行计划/任务分解.md)）。

## 为什么不同

| | 云端记忆服务 | 框架内置记忆 | **memalyze** |
|---|---|---|---|
| 数据位置 | 厂商云 | 随框架 | **你自己的服务器** |
| 接入方式 | 私有 API | 绑定框架 | **MCP（任何 harness）** |
| 企业治理 | ❌ | ❌ | **多租户 / 审计（商用版）** |
| 部署 | SaaS | 跟着框架走 | **单二进制 / 5 分钟 Docker** |

## 快速开始

```bash
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
- [ ] 核心链路

## 许可证

AGPL-3.0（+ 商业双许可）。见 [LICENSE](LICENSE)。

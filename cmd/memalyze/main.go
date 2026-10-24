// Command memalyze 启动 memalyze 的 MCP server（stdio），并提供几个离线运维子命令。
//
// 用法：
//
//	memalyze                     # 在 stdio 上提供 MCP 服务
//	memalyze --version           # 打印版本
//	memalyze audit export ...    # 导出审计（CSV/JSON）
//	memalyze memory list         # 列出记忆（排障/演示用）
//	memalyze memory get <id>     # 读取一条记忆
//	memalyze redact --text "..." # 预览脱敏结果（不写库）
//	memalyze demo                # 30 秒治理演示（不需要 Ollama）
//
// 配置全部走环境变量，见 internal/config。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/config"
	"github.com/mahaoboy/memalyze/internal/embed"
	"github.com/mahaoboy/memalyze/internal/embed/ollama"
	"github.com/mahaoboy/memalyze/internal/govern"
	"github.com/mahaoboy/memalyze/internal/mcp"
	"github.com/mahaoboy/memalyze/internal/memory"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

// version 由 goreleaser / -ldflags 注入。
var version = "0.0.1-dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	httpAddr := flag.String("http", "", `listen on this address with Streamable HTTP, e.g. ":8080" (also MEMALYZE_HTTP_ADDR)`)
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if args := flag.Args(); len(args) > 0 {
		switch args[0] {
		case "audit":
			os.Exit(runAudit(args[1:]))
		case "memory":
			os.Exit(runMemory(args[1:]))
		case "redact":
			os.Exit(runRedact(args[1:]))
		case "demo":
			os.Exit(runDemo(args[1:]))
		case "health":
			os.Exit(runHealth(args[1:]))
		case "backup":
			os.Exit(runBackup(args[1:]))
		case "help":
			usage()
			return
		default:
			fmt.Fprintf(os.Stderr, "memalyze: unknown command %q\n\n", args[0])
			usage()
			os.Exit(2)
		}
	}

	serve(*httpAddr)
}

func configTenant() string { return config.Load().Tenant }

func usage() {
	fmt.Fprint(os.Stderr, `memalyze — MCP-native, self-hosted memory for AI agents

usage:
  memalyze                      run the MCP server on stdio
  memalyze --http :8080         run the MCP server over Streamable HTTP (needs MEMALYZE_HTTP_TOKEN off-loopback)
  memalyze --version            print version

  memalyze audit export [flags] export audit log as CSV or JSON
      --from 2026-10-01         start of range (date or RFC3339)
      --to 2026-10-31           end of range (date-only includes the whole day)
      --tenant acme             filter by tenant
      --action add,search       filter by action (comma separated)
      --format csv|json         output format (default csv)
      --out audit.csv           output file (default stdout)
      --db ./memalyze.db        database path (default MEMALYZE_DB)

  memalyze memory list [flags]  list memories as JSON lines
      --tenant acme --scope project:x --limit 20 --db ./memalyze.db
  memalyze memory get <id>      print one memory (requires --tenant if not default)

  memalyze redact --text "..."  preview redaction (no database writes)
      --file path               read text from a file instead
      --rules rules.json        custom rules (default: built-in)
      --mode enforce|dry-run    default: enforce

  memalyze health [--url]      check an HTTP deployment (used by container healthchecks)
  memalyze backup --out f.db   consistent snapshot via VACUUM INTO (safe with WAL)
  memalyze demo                30-second governance demo (no Ollama needed)
`)
}

func serve(httpFlag string) {
	// 注意：stdout 是 MCP 协议通道，日志只能走 stderr。
	fmt.Fprintf(os.Stderr, "memalyze %s — MCP-native, self-hosted memory for AI agents\n", version)

	cfg := config.Load()
	if strings.TrimSpace(httpFlag) != "" {
		cfg.HTTPAddr = strings.TrimSpace(httpFlag)
	}
	ctx := context.Background()

	st, err := sqlite.Open(ctx, cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: open database: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	// 这里只构造客户端、不做启动探活：Ollama 暂时不可用时服务仍能启动，
	// 写入会保留记忆并标记 embedding_failed，检索会降级为纯关键词。
	embedder := ollama.New(cfg.OllamaURL, cfg.EmbedModel, ollama.WithKeepAlive(cfg.KeepAlive))
	svc, err := buildService(cfg, st, embedder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: %v\n", err)
		os.Exit(1)
	}

	// 异步 warmup：冷加载 embedding 模型约 20s，而检索只有 3s 预算。
	// 不阻塞 MCP 握手；失败只记日志，检索仍按 D-015 降级。
	go func() {
		start := time.Now()
		if err := svc.Warmup(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "memalyze: embedding warmup failed after %s: %v\n",
				time.Since(start).Round(time.Millisecond), err)
			return
		}
		fmt.Fprintf(os.Stderr, "memalyze: embedding warmup ok in %s\n",
			time.Since(start).Round(time.Millisecond))
	}()

	fmt.Fprintf(os.Stderr, "memalyze: db=%s tenant=%s embed=%s ollama=%s query_timeout=%s keep_alive=%s redact=%s audit=%v actor=%s\n",
		cfg.DBPath, cfg.Tenant, cfg.EmbedModel, cfg.OllamaURL, cfg.QueryTimeout, cfg.KeepAlive,
		cfg.RedactMode, cfg.AuditEnabled, cfg.Actor)

	if cfg.HTTPAddr != "" {
		fmt.Fprintf(os.Stderr, "memalyze: transport=http addr=%s auth=%v\n",
			cfg.HTTPAddr, cfg.HTTPToken != "")
		err = mcp.ServeHTTP(ctx, mcp.HTTPOptions{
			Addr:    cfg.HTTPAddr,
			Version: version,
			Tenant:  cfg.Tenant,
			Token:   cfg.HTTPToken,
			Service: svc,
		})
	} else {
		err = mcp.Serve(ctx, version, svc, cfg.Tenant)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: %v\n", err)
		os.Exit(1)
	}
}

// buildService 按配置组装 memory.Service（脱敏 → 审计 → embedding）。
func buildService(cfg config.Config, st store.Store, embedder embed.Embedder) (*memory.Service, error) {
	opts := []memory.Option{memory.WithQueryTimeout(cfg.QueryTimeout)}

	if cfg.RedactMode != govern.ModeOff {
		r, err := govern.Load(cfg.RedactRules)
		if err != nil {
			// 规则坏了就拒绝启动：静默降级成"不脱敏"比崩溃更危险。
			return nil, fmt.Errorf("load redaction rules: %w", err)
		}
		opts = append(opts, memory.WithRedactor(r, cfg.RedactMode))
	}

	if cfg.AuditEnabled {
		as, ok := st.(store.AuditStore)
		if !ok {
			return nil, fmt.Errorf("store %T does not support audit", st)
		}
		opts = append(opts, memory.WithAudit(as, cfg.Actor, cfg.AuditQueryPlain))
	}

	return memory.New(st, embedder, opts...), nil
}

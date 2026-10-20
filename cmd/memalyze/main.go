// Command memalyze 启动 memalyze 的 MCP server（stdio）。
//
// 用法（由 harness 作为 MCP server 拉起，或手动调试）：
//
//	memalyze            # 在 stdio 上提供 MCP 服务
//	memalyze --version  # 打印版本
//
// 配置全部走环境变量，见 internal/config。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mahaoboy/memalyze/internal/config"
	"github.com/mahaoboy/memalyze/internal/embed/ollama"
	"github.com/mahaoboy/memalyze/internal/mcp"
	"github.com/mahaoboy/memalyze/internal/memory"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

// version 由 goreleaser / -ldflags 注入。
var version = "0.0.1-dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	// 注意：stdout 是 MCP 协议通道，日志只能走 stderr。
	fmt.Fprintf(os.Stderr, "memalyze %s — MCP-native, self-hosted memory for AI agents\n", version)

	cfg := config.Load()
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
	svc := memory.New(st, embedder, memory.WithQueryTimeout(cfg.QueryTimeout))

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

	fmt.Fprintf(os.Stderr, "memalyze: db=%s tenant=%s embed=%s ollama=%s query_timeout=%s keep_alive=%s\n",
		cfg.DBPath, cfg.Tenant, cfg.EmbedModel, cfg.OllamaURL, cfg.QueryTimeout, cfg.KeepAlive)

	if err := mcp.Serve(ctx, version, svc, cfg.Tenant); err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: %v\n", err)
		os.Exit(1)
	}
}

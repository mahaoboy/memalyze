// Command memalyze 启动 memalyze 的 MCP server（stdio）。
//
// 用法（由 harness 作为 MCP server 拉起，或手动调试）：
//
//	memalyze            # 在 stdio 上提供 MCP 服务
//	memalyze --version  # 打印版本
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mahaoboy/memalyze/internal/mcp"
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

	// S6：当前只有 echo 工具；S9+ 在此接入 store / embed / search。
	if err := mcp.Serve(context.Background(), version); err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: %v\n", err)
		os.Exit(1)
	}
}

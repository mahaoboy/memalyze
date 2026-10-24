package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mahaoboy/memalyze/internal/audit"
	"github.com/mahaoboy/memalyze/internal/govern"
	"github.com/mahaoboy/memalyze/internal/memory"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
	"github.com/mahaoboy/memalyze/pkg/types"
)

// runDemo 是 S15 的 30 秒演示：不需要 Ollama、不需要 MCP 客户端。
//
// 它做四件事，对应验收标准的四条：
//  1. 写入一条带手机号/邮箱/合同号的记忆 → 打印真正落库的内容（已脱敏）
//  2. 用原始手机号去检索 → 0 命中（证明原文没进 FTS）
//  3. 用正常词检索 → 命中，且返回的仍是脱敏文本
//  4. 导出审计 → 每次 add/search 都有 actor/时间/结果数
func runDemo(args []string) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dbPath := fs.String("db", "", "reuse a database path instead of a temp one")
	keep := fs.Bool("keep", false, "keep the temporary database for inspection")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx := context.Background()
	cleanup := func() {}
	path := *dbPath
	if path == "" {
		dir, err := os.MkdirTemp("", "memalyze-demo-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "memalyze: temp dir: %v\n", err)
			return 1
		}
		path = filepath.Join(dir, "demo.db")
		if !*keep {
			cleanup = func() { os.RemoveAll(dir) }
		}
	}
	defer cleanup()

	st, err := sqlite.Open(ctx, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: open database: %v\n", err)
		return 1
	}
	defer st.Close()

	redactor, err := govern.Default()
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: %v\n", err)
		return 1
	}
	// embedder 传 nil：演示只走关键词路径，因此不需要模型、不需要网络。
	svc := memory.New(st, nil,
		memory.WithRedactor(redactor, govern.ModeEnforce),
		memory.WithAudit(st, "demo", false),
		memory.WithLogger(func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "  [audit] "+format+"\n", args...)
		}),
	)

	secret := "客户张伟的手机是 13800138000，邮箱 zhangwei@example.com，身份证 110101199003071234"
	normal := "每周日强制休息，这是硬规则"

	fmt.Println("════════════════════════════════════════════════════════════")
	fmt.Println(" memalyze · 企业治理 30 秒演示（脱敏在写入前 / 读取有审计）")
	fmt.Println("════════════════════════════════════════════════════════════")

	fmt.Println("\n[1] 写入两条记忆（第一条含手机号、邮箱、身份证号）")
	res1, err := svc.Add(ctx, memory.Input{TenantID: "acme", Scope: "crm", Content: secret, Kind: types.KindFact})
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: add: %v\n", err)
		return 1
	}
	printAdd("敏感样本", res1)
	if _, err := svc.Add(ctx, memory.Input{TenantID: "acme", Scope: "ops", Content: normal, Kind: types.KindNote}); err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: add: %v\n", err)
		return 1
	}

	fmt.Println("\n[2] 数据库里真正存的是什么（原文不该出现）")
	m, err := st.Get(ctx, "acme", res1.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: get: %v\n", err)
		return 1
	}
	fmt.Printf("    content = %s\n", m.Content)
	if containsAny(m.Content, "13800138000", "zhangwei@example.com", "110101199003071234") {
		fmt.Println("    ❌ 原文泄漏")
		return 1
	}
	fmt.Println("    ✅ 手机号 / 邮箱 / 身份证号 均未落库")

	fmt.Println("\n[3] 拿原始手机号检索（应 0 命中）")
	hits, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "13800138000"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: search: %v\n", err)
		return 1
	}
	fmt.Printf("    hits = %d\n", len(hits.Hits))
	if len(hits.Hits) != 0 {
		fmt.Println("    ❌ 原始号码仍可检索到")
		return 1
	}
	fmt.Println("    ✅ 库里没有任何可反查原始号码的索引")

	fmt.Println("\n[4] 用业务词检索（命中，且返回脱敏文本）")
	hits, err = svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "客户"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: search: %v\n", err)
		return 1
	}
	for _, h := range hits.Hits {
		fmt.Printf("    - %s (matched_by=%v)\n", h.Memory.Content, h.MatchedBy)
	}

	fmt.Println("\n[5] 审计导出（谁能读、读过什么，一条命令交差）")
	entries, err := st.QueryAudit(ctx, store.AuditFilter{TenantID: "acme"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: audit: %v\n", err)
		return 1
	}
	if err := audit.Write(os.Stdout, entries, audit.FormatCSV); err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: export: %v\n", err)
		return 1
	}

	fmt.Println("\n────────────────────────────────────────────────────────────")
	fmt.Printf(" 演示完成：%d 条记忆，%d 条审计；数据库 %s\n", 2, len(entries), path)
	if *keep || *dbPath != "" {
		fmt.Printf(" 数据库保留在：%s\n", path)
	}
	fmt.Println("────────────────────────────────────────────────────────────")
	return 0
}

func printAdd(label string, res memory.Result) {
	fmt.Printf("    %s: id=%s redacted=%v hits=%d rules=%v dry_run=%v\n",
		label, shortID(res.ID), res.Redacted, res.RedactionCount, res.Redactions, res.DryRun)
}

func shortID(id string) string {
	if len(id) > 10 {
		return id[:10] + "…"
	}
	return id
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

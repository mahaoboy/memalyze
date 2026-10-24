package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mahaoboy/memalyze/pkg/types"

	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

// memoryView 是 CLI 输出结构（字段少而稳定，方便 jq / 演示脚本消费）。
type memoryView struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	Scope     string `json:"scope"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func viewOf(m types.Memory) memoryView {
	v := memoryView{
		ID:        m.ID,
		TenantID:  m.TenantID,
		Scope:     m.Scope,
		Kind:      string(m.Kind),
		Status:    string(m.Status),
		Content:   m.Content,
		CreatedAt: m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if m.ExpiresAt != nil {
		v.ExpiresAt = m.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return v
}

func runMemory(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: memalyze memory list|get [flags]")
		return 2
	}
	switch args[0] {
	case "list":
		return memoryList(args[1:])
	case "get":
		return memoryGet(args[1:])
	}
	fmt.Fprintf(os.Stderr, "memalyze: unknown memory subcommand %q\n", args[0])
	return 2
}

func memoryList(args []string) int {
	fs := flag.NewFlagSet("memory list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenant := fs.String("tenant", "", "tenant id (default MEMALYZE_TENANT)")
	scope := fs.String("scope", "", "scope filter")
	limit := fs.Int("limit", 20, "max rows")
	db := fs.String("db", "", "database path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx := context.Background()
	st, err := sqlite.Open(ctx, dbPathOr(*db))
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: open database: %v\n", err)
		return 1
	}
	defer st.Close()

	hits, err := st.Recent(ctx, types.SearchQuery{
		TenantID: tenantOr(*tenant),
		Scope:    *scope,
		Limit:    *limit,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: list: %v\n", err)
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	for _, h := range hits {
		if err := enc.Encode(viewOf(h.Memory)); err != nil {
			fmt.Fprintf(os.Stderr, "memalyze: encode: %v\n", err)
			return 1
		}
	}
	return 0
}

func memoryGet(args []string) int {
	fs := flag.NewFlagSet("memory get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenant := fs.String("tenant", "", "tenant id")
	db := fs.String("db", "", "database path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: memalyze memory get <id> [--tenant --db]")
		return 2
	}

	ctx := context.Background()
	st, err := sqlite.Open(ctx, dbPathOr(*db))
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: open database: %v\n", err)
		return 1
	}
	defer st.Close()

	m, err := st.Get(ctx, tenantOr(*tenant), fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: get: %v\n", err)
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(viewOf(*m)); err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: encode: %v\n", err)
		return 1
	}
	return 0
}

func tenantOr(s string) string {
	if s != "" {
		return s
	}
	return configTenant()
}

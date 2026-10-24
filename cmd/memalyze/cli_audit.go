package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/audit"
	"github.com/mahaoboy/memalyze/internal/config"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

func runAudit(args []string) int {
	if len(args) == 0 || args[0] != "export" {
		fmt.Fprintln(os.Stderr, "usage: memalyze audit export [--from --to --tenant --action --format --out --db]")
		return 2
	}
	fs := flag.NewFlagSet("audit export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	from := fs.String("from", "", "start of range (YYYY-MM-DD or RFC3339)")
	to := fs.String("to", "", "end of range (YYYY-MM-DD includes the whole day)")
	tenant := fs.String("tenant", "", "filter by tenant")
	actions := fs.String("action", "", "comma separated: add,search,delete,reembed")
	format := fs.String("format", "csv", "csv or json")
	out := fs.String("out", "", "output file (default stdout)")
	dbPath := fs.String("db", "", "database path (default MEMALYZE_DB)")
	limit := fs.Int("limit", 0, "max rows (default 1000, max 100000)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	fmtName, err := audit.ParseFormat(*format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: %v\n", err)
		return 2
	}

	filter := store.AuditFilter{TenantID: strings.TrimSpace(*tenant), Limit: *limit}
	if *from != "" {
		t, err := parseBound(*from, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "memalyze: --from: %v\n", err)
			return 2
		}
		filter.From = t
	}
	if *to != "" {
		t, err := parseBound(*to, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "memalyze: --to: %v\n", err)
			return 2
		}
		filter.To = t
	}
	for _, a := range strings.Split(*actions, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		action := store.AuditAction(a)
		if !action.Valid() {
			fmt.Fprintf(os.Stderr, "memalyze: unknown action %q\n", a)
			return 2
		}
		filter.Actions = append(filter.Actions, action)
	}

	ctx := context.Background()
	st, err := sqlite.Open(ctx, dbPathOr(*dbPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: open database: %v\n", err)
		return 1
	}
	defer st.Close()

	entries, err := st.QueryAudit(ctx, filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: query audit: %v\n", err)
		return 1
	}

	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "memalyze: create %s: %v\n", *out, err)
			return 1
		}
		defer f.Close()
		w = f
	}
	if err := audit.Write(w, entries, fmtName); err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: export: %v\n", err)
		return 1
	}
	if *out != "" {
		fmt.Fprintf(os.Stderr, "memalyze: exported %d audit rows to %s (%s)\n", len(entries), *out, fmtName)
	}
	return 0
}

// parseBound 解析时间边界。date-only 且 end=true 时补到当天 23:59:59.999（含整天）。
func parseBound(s string, end bool) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("want YYYY-MM-DD or RFC3339, got %q", s)
	}
	if end {
		return t.Add(24*time.Hour - time.Millisecond), nil
	}
	return t, nil
}

func dbPathOr(p string) string {
	if strings.TrimSpace(p) != "" {
		return p
	}
	return config.Load().DBPath
}

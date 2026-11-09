package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/audit"
	"github.com/mahaoboy/memalyze/internal/config"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

func runAudit(args []string) int {
	return runAuditIO(args, os.Stdout, os.Stderr)
}

func runAuditIO(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		auditUsage(stderr)
		return 2
	}
	switch args[0] {
	case "export":
		return runAuditExport(args[1:], stdout, stderr)
	case "verify":
		return runAuditVerify(args[1:], stdout, stderr)
	case "report":
		return runAuditReport(args[1:], stdout, stderr)
	default:
		auditUsage(stderr)
		return 2
	}
}

func auditUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  memalyze audit export [--from --to --tenant --action --format csv|json --out --db]")
	fmt.Fprintln(w, "  memalyze audit verify [--json --checkpoint file --write-checkpoint file --db]")
	fmt.Fprintln(w, "  memalyze audit report [--from --to --tenant --action --format html|json --out --db]")
}

type auditFilterFlags struct {
	from, to, tenant, actions string
}

func bindAuditFilterFlags(fs *flag.FlagSet) *auditFilterFlags {
	var f auditFilterFlags
	fs.StringVar(&f.from, "from", "", "start of range (YYYY-MM-DD or RFC3339)")
	fs.StringVar(&f.to, "to", "", "end of range (YYYY-MM-DD includes the whole day)")
	fs.StringVar(&f.tenant, "tenant", "", "filter by tenant")
	fs.StringVar(&f.actions, "action", "", "comma separated: add,search,delete,reembed")
	return &f
}

func (f *auditFilterFlags) filter() (store.AuditFilter, error) {
	filter := store.AuditFilter{TenantID: strings.TrimSpace(f.tenant)}
	if f.from != "" {
		t, err := parseBound(f.from, false)
		if err != nil {
			return filter, fmt.Errorf("--from: %w", err)
		}
		filter.From = t
	}
	if f.to != "" {
		t, err := parseBound(f.to, true)
		if err != nil {
			return filter, fmt.Errorf("--to: %w", err)
		}
		filter.To = t
	}
	for _, a := range strings.Split(f.actions, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		action := store.AuditAction(a)
		if !action.Valid() {
			return filter, fmt.Errorf("unknown action %q", a)
		}
		filter.Actions = append(filter.Actions, action)
	}
	return filter, nil
}

func runAuditExport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("audit export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	filterFlags := bindAuditFilterFlags(fs)
	format := fs.String("format", "csv", "csv or json")
	out := fs.String("out", "", "output file (default stdout)")
	dbPath := fs.String("db", "", "database path (default MEMALYZE_DB)")
	limit := fs.Int("limit", 0, "max rows (default 1000, max 100000)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "memalyze: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}

	fmtName, err := audit.ParseFormat(*format)
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: %v\n", err)
		return 2
	}
	filter, err := filterFlags.filter()
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: %v\n", err)
		return 2
	}
	filter.Limit = *limit

	ctx := context.Background()
	st, err := sqlite.Open(ctx, dbPathOr(*dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: open database: %v\n", err)
		return 1
	}
	defer st.Close()

	entries, err := st.QueryAudit(ctx, filter)
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: query audit: %v\n", err)
		return 1
	}
	if err := writeOutput(*out, stdout, func(w io.Writer) error {
		return audit.Write(w, entries, fmtName)
	}); err != nil {
		fmt.Fprintf(stderr, "memalyze: export: %v\n", err)
		return 1
	}
	if *out != "" {
		fmt.Fprintf(stderr, "memalyze: exported %d audit rows to %s (%s)\n", len(entries), *out, fmtName)
	}
	return 0
}

func writeOutput(path string, stdout io.Writer, write func(io.Writer) error) error {
	if path == "" {
		return write(stdout)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := write(f); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
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

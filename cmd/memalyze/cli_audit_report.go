package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/audit"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

func runAuditReport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("audit report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	filterFlags := bindAuditFilterFlags(fs)
	format := fs.String("format", "html", "html or json")
	out := fs.String("out", "", "output file (default stdout)")
	dbPath := fs.String("db", "", "database path (default MEMALYZE_DB)")
	checkpointPath := fs.String("checkpoint", "", "trusted checkpoint for full-chain comparison")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "memalyze: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	formatName, err := audit.ParseReportFormat(*format)
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: %v\n", err)
		return 2
	}
	filter, err := filterFlags.filter()
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: %v\n", err)
		return 2
	}
	expected, err := readAuditCheckpoint(*checkpointPath)
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: read checkpoint: %v\n", err)
		return 1
	}

	ctx := context.Background()
	st, err := sqlite.Open(ctx, dbPathOr(*dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: open database: %v\n", err)
		return 1
	}
	defer st.Close()

	entries, verification, err := st.AuditSnapshot(ctx, filter, expected)
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: read audit snapshot: %v\n", err)
		return 1
	}
	reportFilters := audit.ReportFilters{Tenant: filter.TenantID, Actions: make([]string, 0, len(filter.Actions))}
	if !filter.From.IsZero() {
		reportFilters.From = filter.From.UTC().Format(time.RFC3339Nano)
	}
	if !filter.To.IsZero() {
		reportFilters.To = filter.To.UTC().Format(time.RFC3339Nano)
	}
	for _, action := range filter.Actions {
		reportFilters.Actions = append(reportFilters.Actions, string(action))
	}
	report := audit.NewReport(entries, reportFilters, verification, time.Now())
	if err := writeOutput(*out, stdout, func(w io.Writer) error {
		return audit.WriteReport(w, report, formatName)
	}); err != nil {
		fmt.Fprintf(stderr, "memalyze: write report: %v\n", err)
		return 1
	}
	if *out != "" {
		fmt.Fprintf(stderr, "memalyze: governance report written to %s (%d filtered events; verification ok=%v)\n",
			*out, len(entries), verification.OK)
	}
	if !verification.OK {
		return 1
	}
	return 0
}

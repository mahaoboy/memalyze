package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

// runBackup 生成一致性快照：memalyze backup --out /backup/memalyze-2026-10-25.db
func runBackup(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("out", "", "target file (must not exist)")
	db := fs.String("db", "", "database path (default MEMALYZE_DB)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: memalyze backup --out /path/snapshot.db [--db ./memalyze.db]")
		return 2
	}

	ctx := context.Background()
	st, err := sqlite.Open(ctx, dbPathOr(*db))
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: open database: %v\n", err)
		return 1
	}
	defer st.Close()

	if err := st.BackupTo(ctx, *out); err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: %v\n", err)
		return 1
	}
	info, statErr := os.Stat(*out)
	if statErr != nil {
		fmt.Fprintf(os.Stderr, "memalyze: backup written but stat failed: %v\n", statErr)
		return 1
	}
	fmt.Printf("backup ok: %s (%d bytes)\n", *out, info.Size())
	return 0
}

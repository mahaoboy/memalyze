package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

// runBackup 生成一致性快照，并从快照本身计算独立审计检查点。
func runBackup(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("out", "", "target file (must not exist)")
	db := fs.String("db", "", "database path (default MEMALYZE_DB)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: memalyze backup --out /path/snapshot.db [--db ./memalyze.db]")
		return 2
	}
	checkpointPath := *out + ".audit.json"
	if _, err := os.Lstat(checkpointPath); err == nil {
		fmt.Fprintf(os.Stderr, "memalyze: checkpoint target %s already exists\n", checkpointPath)
		return 1
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "memalyze: stat checkpoint %s: %v\n", checkpointPath, err)
		return 1
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

	// 不能备份后再查询源库链头，否则并发新增事件会让 checkpoint 与快照不一致。
	snap, err := sqlite.Open(ctx, *out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: open snapshot for audit verification: %v\n", err)
		return 1
	}
	verification, verifyErr := snap.VerifyAudit(ctx, nil)
	closeErr := snap.Close()
	if verifyErr != nil {
		fmt.Fprintf(os.Stderr, "memalyze: verify snapshot audit: %v\n", verifyErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(os.Stderr, "memalyze: close snapshot: %v\n", closeErr)
		return 1
	}
	if !verification.OK {
		fmt.Fprintf(os.Stderr, "memalyze: snapshot audit verification failed: %s (first_invalid_seq=%d)\n",
			verification.Reason, verification.FirstInvalidSeq)
		return 1
	}
	cp, _ := verification.Checkpoint()
	if err := writeAuditCheckpointNew(checkpointPath, cp); err != nil {
		// BackupTo 已用 O_EXCL 语义新建 out；配对文件失败时只清理本次产物，让重试不被阻塞。
		if cleanupErr := os.Remove(*out); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "memalyze: write audit checkpoint: %v; remove incomplete snapshot: %v\n", err, cleanupErr)
		} else {
			fmt.Fprintf(os.Stderr, "memalyze: write audit checkpoint: %v; incomplete snapshot removed\n", err)
		}
		return 1
	}

	info, statErr := os.Stat(*out)
	if statErr != nil {
		fmt.Fprintf(os.Stderr, "memalyze: backup written but stat failed: %v\n", statErr)
		return 1
	}
	if _, err := fmt.Printf("backup ok: %s (%d bytes)\naudit checkpoint: %s (events=%d head=%s)\n",
		*out, info.Size(), checkpointPath, cp.EventCount, cp.Head); err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: write backup result: %v\n", err)
		return 1
	}
	return 0
}

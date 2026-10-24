package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/pkg/types"
)

func TestBackupToProducesUsableSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	src, err := Open(ctx, filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	m := &types.Memory{TenantID: "acme", Scope: "crm", Content: "周日强制休息"}
	id, err := src.Add(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := src.AppendAudit(ctx, store.AuditEntry{
		TenantID: "acme", Action: store.AuditAdd, MemoryID: id, ResultCount: 1,
	}); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "snapshot.db")
	if err := src.BackupTo(ctx, dst); err != nil {
		t.Fatalf("backup: %v", err)
	}

	snap, err := Open(ctx, dst)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snap.Close()

	got, err := snap.Get(ctx, "acme", id)
	if err != nil {
		t.Fatalf("memory missing from snapshot: %v", err)
	}
	if got.Content != "周日强制休息" {
		t.Fatalf("content = %q", got.Content)
	}
	entries, err := snap.QueryAudit(ctx, store.AuditFilter{TenantID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].MemoryID != id {
		t.Fatalf("audit not in snapshot: %+v", entries)
	}
}

func TestBackupRefusesExistingTarget(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	src, err := Open(ctx, filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	dst := filepath.Join(dir, "exists.db")
	if err := src.BackupTo(ctx, dst); err != nil {
		t.Fatal(err)
	}
	if err := src.BackupTo(ctx, dst); err == nil {
		t.Fatal("expected refusal when target exists")
	}
	if err := src.BackupTo(ctx, ""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

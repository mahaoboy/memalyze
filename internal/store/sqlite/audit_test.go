package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/store"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), t.TempDir()+"/audit.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestAppendAndQueryAudit(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	base := time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC)
	entries := []store.AuditEntry{
		{ID: "a1", Time: base, Actor: "local", TenantID: "acme", Scope: "crm",
			Action: store.AuditAdd, MemoryID: "m1", ResultCount: 1, Redacted: 2},
		{ID: "a2", Time: base.Add(time.Minute), Actor: "claude", TenantID: "acme",
			Action: store.AuditSearch, QueryHash: "abc", QueryLen: 3, ResultCount: 1, Degraded: true},
		{ID: "a3", Time: base.Add(2 * time.Minute), Actor: "local", TenantID: "other",
			Action: store.AuditDelete, MemoryID: "m9", ResultCount: 1},
	}
	for _, e := range entries {
		if err := st.AppendAudit(ctx, e); err != nil {
			t.Fatalf("append %s: %v", e.ID, err)
		}
	}

	got, err := st.QueryAudit(ctx, store.AuditFilter{TenantID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a1" || got[1].ID != "a2" {
		t.Fatalf("unexpected rows: %+v", got)
	}
	if !got[1].Degraded || got[1].QueryLen != 3 || got[1].QueryHash != "abc" {
		t.Fatalf("fields lost: %+v", got[1])
	}
	if got[0].Redacted != 2 || got[0].MemoryID != "m1" || got[0].Actor != "local" {
		t.Fatalf("fields lost: %+v", got[0])
	}
	if got[0].Time.UnixMilli() != base.UnixMilli() {
		t.Fatalf("time = %v, want %v", got[0].Time, base)
	}
}

func TestQueryAuditFilters(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		action := store.AuditAdd
		if i%2 == 1 {
			action = store.AuditSearch
		}
		err := st.AppendAudit(ctx, store.AuditEntry{
			Time:     base.Add(time.Duration(i) * time.Hour),
			TenantID: "acme",
			Action:   action,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	from := base.Add(2 * time.Hour)
	to := base.Add(4 * time.Hour)
	got, err := st.QueryAudit(ctx, store.AuditFilter{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("range rows = %d, want 3", len(got))
	}

	got, err = st.QueryAudit(ctx, store.AuditFilter{Actions: []store.AuditAction{store.AuditSearch}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("action rows = %d, want 3", len(got))
	}

	got, err = st.QueryAudit(ctx, store.AuditFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("limit rows = %d", len(got))
	}

	got, err = st.QueryAudit(ctx, store.AuditFilter{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("offset rows = %d", len(got))
	}
}

func TestAppendAuditValidation(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	if err := st.AppendAudit(ctx, store.AuditEntry{Action: store.AuditAdd}); err == nil {
		t.Fatal("expected error for empty tenant")
	}
	if err := st.AppendAudit(ctx, store.AuditEntry{TenantID: "acme", Action: "hack"}); err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestAuditIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.AppendAudit(ctx, store.AuditEntry{
		ID: "a1", Time: time.Now(), TenantID: "acme", Action: store.AuditAdd,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := st.db.ExecContext(ctx, `UPDATE audit_log SET actor = 'attacker' WHERE id = 'a1'`)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("update should be blocked, got %v", err)
	}
}

func TestMigrationFromV1AddsAuditTable(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/old.db"

	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// 模拟一个只跑过 v1 的老库
	if _, err := st.db.ExecContext(ctx, `DROP TABLE audit_log`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen v1 db: %v", err)
	}
	defer st2.Close()

	var version int
	if err := st2.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion() {
		t.Fatalf("user_version = %d, want %d", version, schemaVersion())
	}
	if err := st2.AppendAudit(ctx, store.AuditEntry{TenantID: "acme", Action: store.AuditAdd}); err != nil {
		t.Fatalf("audit after migration: %v", err)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/future.db"
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	st.Close()

	if _, err := Open(ctx, path); err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("expected version guard, got %v", err)
	}
}

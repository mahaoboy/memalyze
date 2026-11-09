package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/store"
)

func fixedAuditEntry(seq int64) store.AuditEntry {
	return store.AuditEntry{
		ID:       fmt.Sprintf("event-%03d", seq),
		Time:     time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Millisecond),
		Actor:    "local",
		TenantID: "acme",
		Scope:    "crm",
		Action:   store.AuditAdd,
		MemoryID: fmt.Sprintf("memory-%03d", seq),
	}
}

func TestAuditChainFixedHashVector(t *testing.T) {
	e := store.AuditEntry{
		Seq: 1, ID: "a1", Time: time.UnixMilli(1792890000000).UTC(), Actor: "local",
		TenantID: "acme", Scope: "crm", Action: store.AuditAdd, MemoryID: "m1",
		ResultCount: 1, Redacted: 2, Detail: "note", PrevHash: store.AuditGenesisHash,
	}
	// 由独立 Python json/hashlib 实现计算，避免用被测函数自己生成期望值。
	const want = "b9367bdd123c0f492ac4637c4e6b57f6ce52618ab650fa00f81174bcaa2edf37"
	if got := hashAuditEntry(e); got != want {
		t.Fatalf("hash = %s, want %s", got, want)
	}
}

func TestAuditChainAppendVerifyAndCheckpoint(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	for i := int64(1); i <= 3; i++ {
		e := fixedAuditEntry(i)
		e.Seq, e.PrevHash, e.Hash = 99, "caller-prev", "caller-hash"
		if err := st.AppendAudit(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := st.QueryAudit(ctx, store.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	prev := store.AuditGenesisHash
	for i, e := range entries {
		if e.Seq != int64(i+1) || e.PrevHash != prev || len(e.Hash) != 64 {
			t.Fatalf("invalid chain fields at %d: %+v", i, e)
		}
		prev = e.Hash
	}

	v, err := st.VerifyAudit(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.EventCount != 3 || v.LastSeq != 3 || v.CheckpointStatus != "not_provided" {
		t.Fatalf("unexpected verification: %+v", v)
	}
	cp, ok := v.Checkpoint()
	if !ok {
		t.Fatal("valid verification did not produce checkpoint")
	}
	matched, err := st.VerifyAudit(ctx, &cp)
	if err != nil {
		t.Fatal(err)
	}
	if !matched.OK || matched.CheckpointStatus != "matched" || matched.ExpectedCheckpoint == nil {
		t.Fatalf("checkpoint should match: %+v", matched)
	}

	cp.Head = strings.Repeat("f", 64)
	mismatch, err := st.VerifyAudit(ctx, &cp)
	if err != nil {
		t.Fatal(err)
	}
	if mismatch.OK || mismatch.CheckpointStatus != "mismatch" || mismatch.Reason != "checkpoint_mismatch" {
		t.Fatalf("checkpoint should mismatch: %+v", mismatch)
	}
}

func TestAuditChainEmpty(t *testing.T) {
	st := openTemp(t)
	v, err := st.VerifyAudit(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.EventCount != 0 || v.LastSeq != 0 || v.Head != store.AuditGenesisHash {
		t.Fatalf("unexpected empty verification: %+v", v)
	}
}

func TestAuditChainRejectsDelete(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.AppendAudit(ctx, fixedAuditEntry(1)); err != nil {
		t.Fatal(err)
	}
	_, err := st.db.ExecContext(ctx, `DELETE FROM audit_log WHERE seq = 1`)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("delete should be blocked, got %v", err)
	}
}

func TestAuditChainRejectsInsertOrReplace(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.AppendAudit(ctx, fixedAuditEntry(1)); err != nil {
		t.Fatal(err)
	}
	var recursive int
	if err := st.db.QueryRowContext(ctx, `PRAGMA recursive_triggers`).Scan(&recursive); err != nil {
		t.Fatal(err)
	}
	if recursive != 1 {
		t.Fatalf("recursive_triggers = %d, want 1", recursive)
	}
	// 模拟不受应用 PRAGMA 约束的外部连接：守卫本身必须仍能阻止 REPLACE。
	if _, err := st.db.ExecContext(ctx, `PRAGMA recursive_triggers = OFF`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT OR REPLACE INTO audit_log (seq, id, ts, tenant_id, action, prev_hash, hash)
		 VALUES (1, 'attacker', 1, 'acme', 'delete', 'x', 'y')`,
		`INSERT OR REPLACE INTO audit_log (seq, id, ts, tenant_id, action, prev_hash, hash)
		 VALUES (99, 'event-001', 1, 'acme', 'delete', 'x', 'y')`,
	} {
		if _, err := st.db.ExecContext(ctx, stmt); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("INSERT OR REPLACE should be blocked, got %v", err)
		}
	}
	v, err := st.VerifyAudit(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.EventCount != 1 {
		t.Fatalf("replacement altered chain: %+v", v)
	}
}

func TestAuditChainDetectsTampering(t *testing.T) {
	tests := []struct {
		name   string
		stmt   string
		reason string
	}{
		{"payload", `UPDATE audit_log SET actor = 'attacker' WHERE seq = 2`, "hash_mismatch"},
		{"hash", `UPDATE audit_log SET hash = 'tampered' WHERE seq = 2`, "hash_mismatch"},
		{"previous hash", `UPDATE audit_log SET prev_hash = 'tampered' WHERE seq = 2`, "previous_hash_mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := openTemp(t)
			for i := int64(1); i <= 3; i++ {
				if err := st.AppendAudit(ctx, fixedAuditEntry(i)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := st.db.ExecContext(ctx, `DROP TRIGGER audit_no_update`); err != nil {
				t.Fatal(err)
			}
			if _, err := st.db.ExecContext(ctx, tt.stmt); err != nil {
				t.Fatal(err)
			}
			v, err := st.VerifyAudit(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if v.OK || v.FirstInvalidSeq != 2 || !strings.Contains(v.Reason, tt.reason) {
				t.Fatalf("tamper not located: %+v", v)
			}
		})
	}
}

func TestAuditChainDetectsMiddleDeletion(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	for i := int64(1); i <= 3; i++ {
		if err := st.AppendAudit(ctx, fixedAuditEntry(i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.db.ExecContext(ctx, `DROP TRIGGER audit_no_delete`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM audit_log WHERE seq = 2`); err != nil {
		t.Fatal(err)
	}
	v, err := st.VerifyAudit(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.OK || v.FirstInvalidSeq != 2 || !strings.Contains(v.Reason, "sequence_mismatch") {
		t.Fatalf("middle deletion not located: %+v", v)
	}
}

func TestAuditChainTailDeletionNeedsCheckpoint(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	for i := int64(1); i <= 3; i++ {
		if err := st.AppendAudit(ctx, fixedAuditEntry(i)); err != nil {
			t.Fatal(err)
		}
	}
	before, err := st.VerifyAudit(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	cp, _ := before.Checkpoint()
	if _, err := st.db.ExecContext(ctx, `DROP TRIGGER audit_no_delete`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM audit_log WHERE seq = 3`); err != nil {
		t.Fatal(err)
	}

	withoutCheckpoint, err := st.VerifyAudit(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !withoutCheckpoint.OK || withoutCheckpoint.EventCount != 2 {
		t.Fatalf("remaining prefix should be internally valid: %+v", withoutCheckpoint)
	}
	withCheckpoint, err := st.VerifyAudit(ctx, &cp)
	if err != nil {
		t.Fatal(err)
	}
	if withCheckpoint.OK || withCheckpoint.CheckpointStatus != "mismatch" {
		t.Fatalf("tail truncation should mismatch checkpoint: %+v", withCheckpoint)
	}
}

func TestAuditChainConcurrentAppend(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	const count = 100
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 1; i <= count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := fixedAuditEntry(int64(i))
			if err := st.AppendAudit(ctx, e); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	v, err := st.VerifyAudit(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.EventCount != count || v.LastSeq != count {
		t.Fatalf("concurrent chain invalid: %+v", v)
	}
}

func TestAuditSnapshotPaginatesAndVerifiesOneView(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	const count = 1005
	for i := int64(1); i <= count; i++ {
		if err := st.AppendAudit(ctx, fixedAuditEntry(i)); err != nil {
			t.Fatal(err)
		}
	}
	entries, verification, err := st.AuditSnapshot(ctx, store.AuditFilter{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.OK || verification.EventCount != count || len(entries) != count {
		t.Fatalf("inconsistent paginated snapshot: events=%d entries=%d verification=%+v", verification.EventCount, len(entries), verification)
	}
	last := entries[len(entries)-1]
	if verification.LastSeq != last.Seq || verification.Head != last.Hash {
		t.Fatalf("verification and aggregates came from different views: last=%+v verification=%+v", last, verification)
	}

	// 一个旧时间戳的新事件必须同时出现在下一次校验和聚合中，不能造成 OFFSET 重计。
	older := fixedAuditEntry(count + 1)
	older.Time = time.UnixMilli(0)
	if err := st.AppendAudit(ctx, older); err != nil {
		t.Fatal(err)
	}
	entries, verification, err = st.AuditSnapshot(ctx, store.AuditFilter{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]int, len(entries))
	for _, e := range entries {
		seen[e.ID]++
	}
	if !verification.OK || verification.EventCount != count+1 || len(entries) != count+1 || seen[older.ID] != 1 {
		t.Fatalf("older concurrent-style append not represented exactly once: verification=%+v seen=%d entries=%d", verification, seen[older.ID], len(entries))
	}
}

func TestAuditChainConcurrentStores(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/shared.db"
	const writers = 4
	const perWriter = 25
	stores := make([]*Store, 0, writers)
	for i := 0; i < writers; i++ {
		st, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		stores = append(stores, st)
	}

	var wg sync.WaitGroup
	errs := make(chan error, writers*perWriter)
	for w, st := range stores {
		for i := 0; i < perWriter; i++ {
			wg.Add(1)
			go func(w, i int, st *Store) {
				defer wg.Done()
				e := fixedAuditEntry(int64(w*perWriter + i + 1))
				if err := st.AppendAudit(ctx, e); err != nil {
					errs <- err
				}
			}(w, i, st)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	v, err := stores[0].VerifyAudit(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.EventCount != writers*perWriter {
		t.Fatalf("multi-writer chain invalid: %+v", v)
	}
}

func TestMigrationFromV2BackfillsRealRows(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/v2.db"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	for _, stmts := range [][]string{schemaV1, schemaV2} {
		for _, stmt := range stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	legacy := []store.AuditEntry{fixedAuditEntry(2), fixedAuditEntry(1)}
	for _, e := range legacy {
		_, err := db.ExecContext(ctx,
			`INSERT INTO audit_log (`+legacyAuditColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.ID, ms(e.Time), nullable(e.Actor), e.TenantID, nullable(e.Scope), string(e.Action),
			nullable(e.MemoryID), nullable(e.QueryHash), e.QueryLen, e.ResultCount,
			0, 0, e.Redacted, 0, nullable(e.Detail))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrate real v2 fixture: %v", err)
	}
	defer st.Close()
	v, err := st.VerifyAudit(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.EventCount != 2 {
		t.Fatalf("legacy chain invalid: %+v", v)
	}
	entries, err := st.QueryAudit(ctx, store.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != fixedAuditEntry(1).ID || entries[1].ID != fixedAuditEntry(2).ID {
		t.Fatalf("legacy order/fields changed: %+v", entries)
	}
	if entries[0].Time.UnixMilli() != fixedAuditEntry(1).Time.UnixMilli() || entries[0].MemoryID != "memory-001" {
		t.Fatalf("legacy data changed: %+v", entries[0])
	}

	oldHead := v.Head
	if err := st.AppendAudit(ctx, fixedAuditEntry(3)); err != nil {
		t.Fatal(err)
	}
	entries, err = st.QueryAudit(ctx, store.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[2].Seq != 3 || entries[2].PrevHash != oldHead {
		t.Fatalf("append did not continue migrated chain: %+v", entries)
	}
}

func TestMigrationFromV2PreservesNullAndEmptyIDs(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/v2-null-id.db"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	for _, stmts := range [][]string{schemaV1, schemaV2} {
		for _, stmt := range stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	// SQLite 的非 INTEGER PRIMARY KEY 允许 NULL；空字符串则是另一种合法且不同的值。
	for i, legacyID := range []any{nil, ""} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO audit_log (id, ts, tenant_id, action) VALUES (?, ?, 'acme', 'add')`,
			legacyID, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrate NULL/empty legacy IDs: %v", err)
	}
	defer st.Close()
	v, err := st.VerifyAudit(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.EventCount != 2 {
		t.Fatalf("NULL/empty ID chain invalid: %+v", v)
	}
	entries, err := st.QueryAudit(ctx, store.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || !entries[0].IDNull || entries[1].IDNull || entries[1].ID != "" {
		t.Fatalf("NULL and empty IDs were not preserved distinctly: %+v", entries)
	}

	// 即使调用方伪造 IDNull，新追加路径也必须生成正常的非 NULL ULID。
	e := fixedAuditEntry(3)
	e.IDNull = true
	if err := st.AppendAudit(ctx, e); err != nil {
		t.Fatal(err)
	}
	entries, err = st.QueryAudit(ctx, store.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[2].IDNull || entries[2].ID == "" {
		t.Fatalf("new append must have a non-NULL ID: %+v", entries[2])
	}
	v, err = st.VerifyAudit(ctx, nil)
	if err != nil || !v.OK {
		t.Fatalf("chain after NULL legacy migration: %+v, %v", v, err)
	}
}

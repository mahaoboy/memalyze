package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/audit"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

func seedCLIAudit(t *testing.T, entries []store.AuditEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.db")
	st, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if err := st.AppendAudit(context.Background(), e); err != nil {
			st.Close()
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func cliAuditEntries(n int) []store.AuditEntry {
	entries := make([]store.AuditEntry, 0, n)
	for i := 0; i < n; i++ {
		tenant := "acme"
		if i >= 1001 {
			tenant = "other"
		}
		action := store.AuditAdd
		if i%2 == 1 {
			action = store.AuditSearch
		}
		entries = append(entries, store.AuditEntry{
			ID: fmt.Sprintf("cli-%04d", i), Time: time.UnixMilli(int64(1700000000000 + i)).UTC(),
			Actor: "local", TenantID: tenant, Scope: "default", Action: action,
			MemoryID: "PRIVATE-MEMORY-ID", QueryHash: "PRIVATE-QUERY-HASH", QueryLen: 42,
			Detail: "query=PRIVATE-DETAIL", Redacted: i % 3, Duplicate: i%11 == 0, Degraded: i%13 == 0,
		})
	}
	return entries
}

func TestAuditVerifyCLIAndCheckpoint(t *testing.T) {
	path := seedCLIAudit(t, cliAuditEntries(3))
	var stdout, stderr bytes.Buffer
	code := runAuditIO([]string{"verify", "--json", "--db", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify exit %d: %s", code, stderr.String())
	}
	var v store.AuditVerification
	if err := json.Unmarshal(stdout.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.EventCount != 3 || v.CheckpointStatus != "not_provided" {
		t.Fatalf("unexpected verify JSON: %+v", v)
	}

	checkpoint := filepath.Join(t.TempDir(), "trusted.audit.json")
	stdout.Reset()
	stderr.Reset()
	code = runAuditIO([]string{"verify", "--db", path, "--write-checkpoint", checkpoint}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("write checkpoint exit %d: %s", code, stderr.String())
	}
	info, err := os.Stat(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("checkpoint mode = %o, want 600", info.Mode().Perm())
	}
	stdout.Reset()
	stderr.Reset()
	code = runAuditIO([]string{"verify", "--json", "--db", path, "--checkpoint", checkpoint}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("checkpoint verify exit %d: %s", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.CheckpointStatus != "matched" {
		t.Fatalf("checkpoint did not match: %+v", v)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runAuditIO([]string{"verify", "--db", path, "--write-checkpoint", checkpoint}, &stdout, &stderr); code == 0 {
		t.Fatal("checkpoint overwrite must fail")
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	if _, err := raw.Exec(`DROP TRIGGER audit_no_delete`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`DELETE FROM audit_log WHERE seq = 3`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = runAuditIO([]string{"verify", "--json", "--db", path, "--checkpoint", checkpoint}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("truncated chain exit = %d, stderr=%s", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.OK || v.CheckpointStatus != "mismatch" || v.Reason != "checkpoint_mismatch" {
		t.Fatalf("tail truncation not reported: %+v", v)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("injected stdout failure") }

func TestAuditVerifyPlainOutputError(t *testing.T) {
	path := seedCLIAudit(t, cliAuditEntries(1))
	var stderr bytes.Buffer
	if code := runAuditIO([]string{"verify", "--db", path}, failingWriter{}, &stderr); code != 1 {
		t.Fatalf("plain output failure exit = %d, stderr=%s", code, stderr.String())
	}
}

func TestReadAuditCheckpointRejectsMalformedTrailingAndOversized(t *testing.T) {
	valid, err := json.Marshal(store.AuditCheckpoint{
		SchemaVersion: store.AuditChainVersion, Algorithm: store.AuditChainAlgorithm,
		EventCount: 0, LastSeq: 0, Head: store.AuditGenesisHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"malformed", []byte("{")},
		{"multiple JSON values", append(append([]byte{}, valid...), []byte(" {}")...)},
		{"oversized trailing data", append(append([]byte{}, valid...), []byte(strings.Repeat(" ", (1<<20)+1))...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "checkpoint.json")
			if err := os.WriteFile(path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := readAuditCheckpoint(path); err == nil {
				t.Fatal("invalid checkpoint was accepted")
			}
		})
	}
}

func TestAuditReportCLIPaginatesAndRedactsRawFields(t *testing.T) {
	path := seedCLIAudit(t, cliAuditEntries(1005))
	var stdout, stderr bytes.Buffer
	code := runAuditIO([]string{"report", "--format", "json", "--tenant", "acme", "--db", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, stderr.String())
	}
	var report audit.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.EventCount != 1001 || report.Verification.EventCount != 1005 {
		t.Fatalf("filtered/global counts wrong (pagination regression?): %+v", report)
	}
	for _, forbidden := range []string{"PRIVATE-MEMORY-ID", "PRIVATE-QUERY-HASH", "PRIVATE-DETAIL", "query="} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Errorf("JSON report leaked %q", forbidden)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = runAuditIO([]string{"report", "--format", "html", "--tenant", "acme", "--db", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("HTML report exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "memalyze governance report") || strings.Contains(stdout.String(), "PRIVATE-DETAIL") {
		t.Fatal("invalid HTML governance report")
	}
}

func TestAuditExportIncludesChainFields(t *testing.T) {
	path := seedCLIAudit(t, cliAuditEntries(1))
	var stdout, stderr bytes.Buffer
	code := runAuditIO([]string{"export", "--format", "json", "--db", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export exit %d: %s", code, stderr.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["seq"] != float64(1) || rows[0]["hash"] == "" || rows[0]["prev_hash"] != store.AuditGenesisHash {
		t.Fatalf("chain fields absent from export: %+v", rows)
	}
}

func TestAuditCLIUsageErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{{}, {"unknown"}, {"verify", "extra"}, {"report", "--format", "csv"}} {
		if code := runAuditIO(args, &stdout, &stderr); code != 2 {
			t.Errorf("runAuditIO(%v) = %d, want 2", args, code)
		}
	}
}

func TestBackupRejectsDanglingCheckpointSymlinkBeforeSnapshot(t *testing.T) {
	path := seedCLIAudit(t, cliAuditEntries(1))
	out := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.Symlink("missing-checkpoint-target", out+".audit.json"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if code := runBackup([]string{"--db", path, "--out", out}); code == 0 {
		t.Fatal("dangling checkpoint symlink must fail closed")
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		t.Fatalf("backup must not leave an artifact after checkpoint precheck failure: %v", err)
	}
}

func TestBackupWritesMatchingSnapshotCheckpoint(t *testing.T) {
	path := seedCLIAudit(t, cliAuditEntries(2))
	out := filepath.Join(t.TempDir(), "snapshot.db")
	if code := runBackup([]string{"--db", path, "--out", out}); code != 0 {
		t.Fatalf("backup exit %d", code)
	}
	checkpointPath := out + ".audit.json"
	cp, err := readAuditCheckpoint(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	if cp.EventCount != 2 {
		t.Fatalf("checkpoint = %+v", cp)
	}
	info, err := os.Stat(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("checkpoint mode = %o", info.Mode().Perm())
	}
	snap, err := sqlite.Open(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	v, err := snap.VerifyAudit(context.Background(), cp)
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.Close(); err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.CheckpointStatus != "matched" {
		t.Fatalf("snapshot/checkpoint mismatch: %+v", v)
	}
	if code := runBackup([]string{"--db", path, "--out", out}); code == 0 {
		t.Fatal("backup overwrite must fail")
	}
}

package audit

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/store"
)

func sample() []store.AuditEntry {
	ts := time.Date(2026, 10, 25, 1, 2, 3, 0, time.UTC)
	return []store.AuditEntry{
		{
			ID: "a1", Time: ts, Actor: "local", TenantID: "acme", Scope: "project:x",
			Action: store.AuditAdd, MemoryID: "m1", ResultCount: 1, Redacted: 2, Detail: "note",
		},
		{
			ID: "a2", Time: ts.Add(time.Second), Actor: "claude", TenantID: "acme",
			Action: store.AuditSearch, QueryHash: "deadbeef", QueryLen: 6,
			ResultCount: 1, Degraded: true, Detail: "degrade=query_timeout",
		},
	}
}

func TestWriteCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sample(), FormatCSV); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want header + 2", len(rows))
	}
	if strings.Join(rows[0], ",") != strings.Join(header, ",") {
		t.Fatalf("header mismatch: %v", rows[0])
	}
	if rows[1][2] != "add" || rows[1][12] != "2" || rows[1][10] != "false" {
		t.Fatalf("unexpected row: %v", rows[1])
	}
	if rows[2][10] != "true" || rows[2][8] != "6" {
		t.Fatalf("unexpected degraded row: %v", rows[2])
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sample(), FormatJSON); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0]["action"] != "add" || got[0]["redacted"].(float64) != 2 {
		t.Fatalf("unexpected: %+v", got[0])
	}
	if got[1]["degraded"] != true {
		t.Fatalf("degraded not exported: %+v", got[1])
	}
}

func TestWriteEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, nil, FormatCSV); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "time,id,action") {
		t.Fatalf("missing header: %q", buf.String())
	}
	buf.Reset()
	if err := Write(&buf, nil, FormatJSON); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Fatalf("want [], got %q", buf.String())
	}
}

func TestParseFormat(t *testing.T) {
	for in, want := range map[string]Format{"": FormatCSV, "csv": FormatCSV, "JSON": FormatJSON} {
		got, err := ParseFormat(in)
		if err != nil || got != want {
			t.Fatalf("ParseFormat(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Fatal("expected error")
	}
}

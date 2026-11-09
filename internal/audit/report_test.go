package audit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/store"
)

func reportFixture() ([]store.AuditEntry, store.AuditVerification) {
	entries := []store.AuditEntry{
		{Actor: "zoe", TenantID: "acme", Scope: "project-z", Action: store.AuditAdd, Redacted: 2, Duplicate: true,
			MemoryID: "memory-secret-1", QueryHash: "query-hash-secret", QueryLen: 999, Detail: "query=TOP-SECRET"},
		{Actor: "alice", TenantID: "acme", Scope: "<img src=x onerror=alert(1)>", Action: store.AuditSearch, Redacted: 1, Degraded: true,
			MemoryID: "memory-secret-2", QueryHash: "another-query-secret", QueryLen: 888, Detail: "plaintext-secret"},
		{Actor: "zoe", TenantID: "acme", Scope: "", Action: store.AuditAdd},
		{Actor: "bob", TenantID: "acme", Scope: "project-z", Action: store.AuditDelete},
	}
	return entries, store.AuditVerification{
		OK: true, Algorithm: store.AuditChainAlgorithm, EventCount: 4, LastSeq: 4,
		Head: strings.Repeat("a", 64), CheckpointStatus: "matched",
	}
}

func TestReportAggregationAndPrivacy(t *testing.T) {
	entries, verification := reportFixture()
	r := NewReport(entries, ReportFilters{Tenant: "acme", Actions: []string{"search", "add"}}, verification, time.Unix(0, 0))
	if r.EventCount != 4 || r.Outcomes.RedactionHits != 3 || r.Outcomes.RedactedEvents != 2 ||
		r.Outcomes.DuplicateEvents != 1 || r.Outcomes.DegradedEvents != 1 {
		t.Fatalf("bad aggregate: %+v", r)
	}
	if len(r.Actors) != 3 || r.Actors[0] != (ReportCount{Name: "zoe", Count: 2}) {
		t.Fatalf("actors not sorted: %+v", r.Actors)
	}
	if got := strings.Join(r.Filters.Actions, ","); got != "add,search" {
		t.Fatalf("filters not deterministic: %s", got)
	}

	for _, format := range []ReportFormat{ReportJSON, ReportHTML} {
		var out bytes.Buffer
		if err := WriteReport(&out, r, format); err != nil {
			t.Fatal(err)
		}
		text := out.String()
		for _, forbidden := range []string{
			"memory-secret-1", "memory-secret-2", "query-hash-secret", "another-query-secret",
			"TOP-SECRET", "plaintext-secret", "query=",
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s report leaked %q", format, forbidden)
			}
		}
	}
}

func TestReportHTMLJSONParityAndEscaping(t *testing.T) {
	entries, verification := reportFixture()
	r := NewReport(entries, ReportFilters{}, verification, time.Unix(0, 0))
	var htmlOut bytes.Buffer
	if err := WriteReport(&htmlOut, r, ReportHTML); err != nil {
		t.Fatal(err)
	}
	html := htmlOut.String()
	if strings.Contains(html, "<img src=x onerror=alert(1)>") || !strings.Contains(html, "&lt;img") {
		t.Fatalf("hostile scope was not escaped: %s", html)
	}

	var jsonOut bytes.Buffer
	if err := WriteReport(&jsonOut, r, ReportJSON); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(jsonOut.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.EventCount != r.EventCount || decoded.Outcomes != r.Outcomes ||
		decoded.Verification.Head != r.Verification.Head || len(decoded.Actors) != len(r.Actors) {
		t.Fatalf("HTML/JSON source DTO parity lost: %+v", decoded)
	}
}

func TestReportOfflineAndEmptyArrays(t *testing.T) {
	r := NewReport(nil, ReportFilters{}, store.AuditVerification{
		OK: true, Algorithm: store.AuditChainAlgorithm, Head: store.AuditGenesisHash, CheckpointStatus: "not_provided",
	}, time.Unix(0, 0))
	var out bytes.Buffer
	if err := WriteReport(&out, r, ReportHTML); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"<script", "<link", "src=\"", "href=\"", "@import", "url("} {
		if strings.Contains(strings.ToLower(out.String()), forbidden) {
			t.Errorf("report has external/active resource marker %q", forbidden)
		}
	}
	out.Reset()
	if err := WriteReport(&out, r, ReportJSON); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"actors": []`, `"actions": []`, `"scopes": []`, `"filters"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("empty JSON missing %s: %s", want, out.String())
		}
	}
}

func TestParseReportFormat(t *testing.T) {
	if got, err := ParseReportFormat(""); err != nil || got != ReportHTML {
		t.Fatalf("default = %q, %v", got, err)
	}
	if _, err := ParseReportFormat("csv"); err == nil {
		t.Fatal("report must not accept raw CSV export format")
	}
}

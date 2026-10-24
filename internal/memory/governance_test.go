package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/mahaoboy/memalyze/internal/govern"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/pkg/types"
)

func newGoverned(t *testing.T, mode govern.Mode, plainQuery bool) (*Service, store.Store, store.AuditStore) {
	t.Helper()
	st := newStore(t)
	auditor, ok := st.(store.AuditStore)
	if !ok {
		t.Fatalf("store %T does not implement AuditStore", st)
	}
	r, err := govern.Default()
	if err != nil {
		t.Fatalf("govern.Default: %v", err)
	}
	svc := New(st, nil,
		WithRedactor(r, mode),
		WithAudit(auditor, "tester", plainQuery),
		WithLogger(func(string, ...any) {}),
	)
	return svc, st, auditor
}

const sensitive = "客户张伟手机 13800138000，邮箱 zhangwei@example.com"

func TestAddRedactsBeforeStore(t *testing.T) {
	ctx := context.Background()
	svc, st, _ := newGoverned(t, govern.ModeEnforce, false)

	res, err := svc.Add(ctx, Input{TenantID: "acme", Scope: "crm", Content: sensitive})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !res.Redacted || res.RedactionCount != 2 || res.DryRun {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(res.Redactions) != 2 {
		t.Fatalf("redactions = %v", res.Redactions)
	}

	m, err := st.Get(ctx, "acme", res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.Content, "13800138000") || strings.Contains(m.Content, "zhangwei@example.com") {
		t.Fatalf("plaintext stored: %q", m.Content)
	}
	if !strings.Contains(m.Content, "[REDACTED:phone]") || !strings.Contains(m.Content, "[REDACTED:email]") {
		t.Fatalf("redaction markers missing: %q", m.Content)
	}

	// 原文不能通过关键词检索反查（FTS 里存的也是脱敏文本）
	hits, err := st.SearchKeyword(ctx, types.SearchQuery{TenantID: "acme", Query: "13800138000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("plaintext still searchable: %+v", hits)
	}
}

func TestDryRunKeepsContentButCounts(t *testing.T) {
	ctx := context.Background()
	svc, st, _ := newGoverned(t, govern.ModeDryRun, false)

	res, err := svc.Add(ctx, Input{TenantID: "acme", Content: sensitive})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Redacted || res.RedactionCount != 2 || !res.DryRun {
		t.Fatalf("unexpected result: %+v", res)
	}
	m, _ := st.Get(ctx, "acme", res.ID)
	if m.Content != strings.TrimSpace(sensitive) {
		t.Fatalf("dry-run must not modify content: %q", m.Content)
	}
}

func TestRedactionHappensBeforeDedup(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newGoverned(t, govern.ModeEnforce, false)

	// 两个不同号码脱敏后文本相同 → 第二次应判重复，且不会写入新记录
	first, err := svc.Add(ctx, Input{TenantID: "acme", Scope: "crm", Content: "call 13800138000"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Add(ctx, Input{TenantID: "acme", Scope: "crm", Content: "call 13900139000"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || !second.Duplicate {
		t.Fatalf("expected duplicate after redaction: first=%+v second=%+v", first, second)
	}
}

func TestAuditRecordsAddSearchDelete(t *testing.T) {
	ctx := context.Background()
	svc, _, auditor := newGoverned(t, govern.ModeEnforce, false)

	res, err := svc.Add(ctx, Input{TenantID: "acme", Scope: "crm", Content: sensitive})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "客户 张伟"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "acme", res.ID); err != nil {
		t.Fatal(err)
	}

	entries, err := auditor.QueryAudit(ctx, store.AuditFilter{TenantID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("audit rows = %d, want 3: %+v", len(entries), entries)
	}
	byAction := map[store.AuditAction]store.AuditEntry{}
	for _, e := range entries {
		byAction[e.Action] = e
		if e.Actor != "tester" {
			t.Fatalf("actor = %q", e.Actor)
		}
	}

	add := byAction[store.AuditAdd]
	if add.MemoryID != res.ID || add.ResultCount != 1 || add.Redacted != 2 || add.DryRun {
		t.Fatalf("unexpected add audit: %+v", add)
	}

	search := byAction[store.AuditSearch]
	if len(search.QueryHash) != 64 {
		t.Fatalf("query hash = %q (want sha256 hex)", search.QueryHash)
	}
	if search.QueryLen != len("客户 张伟") {
		t.Fatalf("query len = %d", search.QueryLen)
	}
	if strings.Contains(search.Detail, "客户") {
		t.Fatalf("plaintext query leaked into audit: %q", search.Detail)
	}
	if !search.Degraded || !strings.Contains(search.Detail, "no_embedder") {
		t.Fatalf("expected degraded search audit: %+v", search)
	}

	del := byAction[store.AuditDelete]
	if del.MemoryID != res.ID || del.ResultCount != 1 {
		t.Fatalf("unexpected delete audit: %+v", del)
	}
}

func TestAuditQueryPlainWhenExplicitlyEnabled(t *testing.T) {
	ctx := context.Background()
	svc, _, auditor := newGoverned(t, govern.ModeEnforce, true)

	if _, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "每周日"}); err != nil {
		t.Fatal(err)
	}
	entries, err := auditor.QueryAudit(ctx, store.AuditFilter{TenantID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Detail, "query=每周日") {
		t.Fatalf("plain query not recorded: %+v", entries)
	}
}

func TestSearchEmptyQueryRecordedAsRecent(t *testing.T) {
	ctx := context.Background()
	svc, _, auditor := newGoverned(t, govern.ModeEnforce, false)

	if _, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme"}); err != nil {
		t.Fatal(err)
	}
	entries, err := auditor.QueryAudit(ctx, store.AuditFilter{TenantID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Detail, "list=recent") {
		t.Fatalf("unexpected audit: %+v", entries)
	}
	if entries[0].QueryHash != "" || entries[0].QueryLen != 0 {
		t.Fatalf("empty query should not be hashed: %+v", entries[0])
	}
}

func TestFailedDeleteIsAudited(t *testing.T) {
	ctx := context.Background()
	svc, _, auditor := newGoverned(t, govern.ModeEnforce, false)

	if err := svc.Delete(ctx, "acme", "does-not-exist"); err == nil {
		t.Fatal("expected error")
	}
	entries, err := auditor.QueryAudit(ctx, store.AuditFilter{TenantID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit rows = %d", len(entries))
	}
	if entries[0].ResultCount != 0 || !strings.Contains(entries[0].Detail, "error=") {
		t.Fatalf("unexpected audit: %+v", entries[0])
	}
}

func TestNoRedactorNoAuditStillWorks(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := New(st, nil)

	res, err := svc.Add(ctx, Input{TenantID: "acme", Content: sensitive})
	if err != nil {
		t.Fatal(err)
	}
	if res.Redacted {
		t.Fatal("no redactor configured, should not redact")
	}
	m, _ := st.Get(ctx, "acme", res.ID)
	if m.Content != sensitive {
		t.Fatalf("content changed: %q", m.Content)
	}
}

func TestReembedIsAudited(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	auditor := st.(store.AuditStore)
	r, err := govern.Default()
	if err != nil {
		t.Fatal(err)
	}
	emb := &fakeEmbedder{vec: []float32{1, 0, 0}}
	svc := New(st, emb,
		WithRedactor(r, govern.ModeEnforce),
		WithAudit(auditor, "tester", false),
		WithLogger(func(string, ...any) {}),
	)

	added, err := svc.Add(ctx, Input{TenantID: "acme", Content: "记住：周日休息"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reembed(ctx, "acme", added.ID); err != nil {
		t.Fatalf("reembed: %v", err)
	}

	entries, err := auditor.QueryAudit(ctx, store.AuditFilter{
		TenantID: "acme",
		Actions:  []store.AuditAction{store.AuditReembed},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("reembed audit rows = %d", len(entries))
	}
	if entries[0].MemoryID != added.ID || entries[0].ResultCount != 1 {
		t.Fatalf("unexpected reembed audit: %+v", entries[0])
	}
}

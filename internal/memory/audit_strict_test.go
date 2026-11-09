package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/pkg/types"
)

type stubAuditor struct {
	err     error
	entries []store.AuditEntry
}

func (a *stubAuditor) AppendAudit(_ context.Context, e store.AuditEntry) error {
	a.entries = append(a.entries, e)
	return a.err
}

func (a *stubAuditor) QueryAudit(context.Context, store.AuditFilter) ([]store.AuditEntry, error) {
	return append([]store.AuditEntry(nil), a.entries...), nil
}

func discardLog(string, ...any) {}

func TestAuditStrictNormalizesDefaultTenantAndScope(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	auditor := &stubAuditor{}
	svc := New(st, nil, WithAudit(auditor, "unit-test", false), WithAuditStrict(true), WithLogger(discardLog))
	res, err := svc.Add(ctx, Input{Content: "default scope audit"})
	if err != nil {
		t.Fatal(err)
	}
	if len(auditor.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(auditor.entries))
	}
	e := auditor.entries[0]
	if e.TenantID != "default" || e.Scope != "default" || e.Actor != "unit-test" || e.MemoryID != res.ID {
		t.Fatalf("default audit normalization failed: %+v", e)
	}
	if _, err := st.Get(ctx, "default", res.ID); err != nil {
		t.Fatalf("memory was not stored under normalized tenant: %v", err)
	}
}

func TestAuditStrictAddFailureIsErrorWithoutRollbackClaim(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	auditor := &stubAuditor{err: errors.New("audit offline")}
	svc := New(st, nil, WithAudit(auditor, "unit-test", false), WithAuditStrict(true), WithLogger(discardLog))
	res, err := svc.Add(ctx, Input{TenantID: "acme", Scope: "crm", Content: "write happened before audit"})
	if err == nil || !strings.Contains(err.Error(), "audit offline") {
		t.Fatalf("strict Add error = %v", err)
	}
	if res.ID == "" {
		t.Fatal("strict Add unexpectedly discarded its post-write result")
	}
	if _, getErr := st.Get(ctx, "acme", res.ID); getErr != nil {
		t.Fatalf("test must document that Add side effect remains: %v", getErr)
	}
}

func TestAuditNonStrictAddFailureKeepsLegacySuccess(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	auditor := &stubAuditor{err: errors.New("audit offline")}
	svc := New(st, nil, WithAudit(auditor, "unit-test", false), WithLogger(discardLog))
	res, err := svc.Add(ctx, Input{TenantID: "acme", Content: "legacy behavior"})
	if err != nil || res.ID == "" {
		t.Fatalf("non-strict Add = %+v, %v", res, err)
	}
}

func TestAuditStrictSearchFailureReturnsNoHits(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := New(st, nil, WithLogger(discardLog))
	added, err := svc.Add(ctx, Input{TenantID: "acme", Content: "searchable strict-audit needle"})
	if err != nil {
		t.Fatal(err)
	}
	auditor := &stubAuditor{err: errors.New("audit offline")}
	svc.auditor = auditor
	svc.actor = "unit-test"
	svc.auditStrict = true

	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "needle"})
	if err == nil || !strings.Contains(err.Error(), "audit offline") {
		t.Fatalf("strict Search error = %v", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("strict Search leaked unaudited hits: %+v", res.Hits)
	}

	svc.auditStrict = false
	res, err = svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "needle"})
	if err != nil || len(res.Hits) == 0 || res.Hits[0].Memory.ID != added.ID {
		t.Fatalf("non-strict Search compatibility lost: %+v, %v", res, err)
	}
}

func TestAuditStrictDeleteAndReembedFailuresReturnErrors(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := New(st, nil, WithLogger(discardLog))
	added, err := svc.Add(ctx, Input{TenantID: "acme", Content: "strict delete"})
	if err != nil {
		t.Fatal(err)
	}
	auditor := &stubAuditor{err: errors.New("audit offline")}
	svc.auditor = auditor
	svc.actor = "unit-test"
	svc.auditStrict = true
	if err := svc.Delete(ctx, "acme", added.ID); err == nil || !strings.Contains(err.Error(), "audit offline") {
		t.Fatalf("strict Delete error = %v", err)
	}
	deleted, getErr := st.Get(ctx, "acme", added.ID)
	if getErr != nil || deleted.Status != types.StatusDeleted {
		t.Fatalf("Delete side effect should remain as soft-deleted memory: %+v, %v", deleted, getErr)
	}

	svc.auditor = nil
	reembedTarget, err := svc.Add(ctx, Input{TenantID: "acme", Content: "strict reembed"})
	if err != nil {
		t.Fatal(err)
	}
	svc.auditor = auditor
	embedder := &fakeEmbedder{vec: []float32{1, 0, 0}}
	svc.embedder = embedder
	res, err := svc.Reembed(ctx, "acme", reembedTarget.ID)
	if err == nil || !strings.Contains(err.Error(), "audit offline") {
		t.Fatalf("strict Reembed error = %v", err)
	}
	if !res.Embedded || embedder.calls != 1 {
		t.Fatalf("Reembed side effect should remain visible: %+v calls=%d", res, embedder.calls)
	}
}

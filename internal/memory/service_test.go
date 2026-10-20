package memory

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
	"github.com/mahaoboy/memalyze/pkg/types"
)

// fakeEmbedder 记录调用次数，可注入失败与延迟。
type fakeEmbedder struct {
	vec      []float32
	fail     bool
	delay    time.Duration
	calls    int
	docCalls int
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.calls++
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.fail {
		return nil, errors.New("embedding service unavailable")
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = f.vec
	}
	return out, nil
}

func (f *fakeEmbedder) Model() string { return "fake" }
func (f *fakeEmbedder) Dim() int      { return len(f.vec) }

// fakeDocEmbedder 同时实现可选接口 EmbedDocuments，用来验证优先走「文档侧前缀」路径。
type fakeDocEmbedder struct{ fakeEmbedder }

func (f *fakeDocEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	f.docCalls++
	if f.fail {
		return nil, errors.New("embedding service unavailable")
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = f.vec
	}
	return out, nil
}

func newStore(t *testing.T) store.Store {
	t.Helper()
	s, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAddStoresAndEmbeds(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{1, 0, 0}}
	svc := New(st, emb)

	res, err := svc.Add(ctx, Input{TenantID: "acme", Scope: "project:x", Content: "  许可证定为 AGPL-3.0  "})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if res.ID == "" || res.Duplicate || !res.Embedded || res.EmbeddingFailed {
		t.Fatalf("unexpected result: %+v", res)
	}

	m, err := st.Get(ctx, "acme", res.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.Content != "许可证定为 AGPL-3.0" {
		t.Fatalf("content not trimmed: %q", m.Content)
	}

	hits, err := st.SearchVector(ctx, types.SearchQuery{TenantID: "acme"}, "fake", []float32{1, 0, 0})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != res.ID {
		t.Fatalf("vector not stored or not searchable: %d hits", len(hits))
	}
}

func TestAddDuplicateReturnsExistingID(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{1, 0}}
	svc := New(st, emb)

	first, err := svc.Add(ctx, Input{TenantID: "acme", Content: "周日强制休息"})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	second, err := svc.Add(ctx, Input{TenantID: "acme", Content: "  周日强制休息  "})
	if err != nil {
		t.Fatalf("second add should not error: %v", err)
	}
	if !second.Duplicate {
		t.Fatal("duplicate flag should be set")
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate id = %s, want %s", second.ID, first.ID)
	}
	if emb.calls != 1 {
		t.Fatalf("embedding should run once, ran %d times", emb.calls)
	}
}

func TestAddRejectsEmptyContent(t *testing.T) {
	svc := New(newStore(t), &fakeEmbedder{vec: []float32{1}})
	if _, err := svc.Add(context.Background(), Input{Content: "   "}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	if _, err := New(nil, nil).Add(context.Background(), Input{Content: "x"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("nil store should be invalid, got %v", err)
	}
}

func TestAddEmbedFailureKeepsMemory(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{1, 0}, fail: true}
	svc := New(st, emb)

	res, err := svc.Add(ctx, Input{TenantID: "acme", Content: "审计日志要能导出"})
	if err != nil {
		t.Fatalf("embedding failure must not fail the write: %v", err)
	}
	if !res.EmbeddingFailed || res.Embedded {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.EmbeddingErr == "" {
		t.Fatal("failure reason should be recorded")
	}

	// 记忆本身在，关键词可查
	hits, err := st.SearchKeyword(ctx, types.SearchQuery{TenantID: "acme", Query: "审计"})
	if err != nil {
		t.Fatalf("keyword search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("memory should still be keyword-searchable, got %d", len(hits))
	}
	// 向量没有，所以向量检索为空
	vecHits, err := st.SearchVector(ctx, types.SearchQuery{TenantID: "acme"}, "fake", []float32{1, 0})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(vecHits) != 0 {
		t.Fatalf("no vector should exist, got %d", len(vecHits))
	}
}

func TestAddEmbedTimeout(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{1}, delay: 300 * time.Millisecond}
	svc := New(st, emb, WithEmbedTimeout(20*time.Millisecond))

	start := time.Now()
	res, err := svc.Add(ctx, Input{TenantID: "acme", Content: "慢模型"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !res.EmbeddingFailed {
		t.Fatalf("expected timeout failure, got %+v", res)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("timeout not enforced, took %v", elapsed)
	}
}

func TestAddTTL(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	fixed := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	svc := New(st, &fakeEmbedder{vec: []float32{1}}, WithClock(func() time.Time { return fixed }))

	res, err := svc.Add(ctx, Input{TenantID: "acme", Content: "带 TTL 的记忆", TTL: 90 * time.Minute})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	m, err := st.Get(ctx, "acme", res.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.ExpiresAt == nil || !m.ExpiresAt.Equal(fixed.Add(90*time.Minute)) {
		t.Fatalf("expires_at = %v, want %v", m.ExpiresAt, fixed.Add(90*time.Minute))
	}

	noTTL, err := svc.Add(ctx, Input{TenantID: "acme", Content: "永不过期"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	m2, err := st.Get(ctx, "acme", noTTL.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m2.ExpiresAt != nil {
		t.Fatalf("expires_at should be nil, got %v", m2.ExpiresAt)
	}
}

func TestAddUsesDocumentEmbedder(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeDocEmbedder{fakeEmbedder: fakeEmbedder{vec: []float32{1, 0}}}
	svc := New(st, emb)

	if _, err := svc.Add(ctx, Input{TenantID: "acme", Content: "前缀模型"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if emb.docCalls != 1 {
		t.Fatalf("EmbedDocuments calls = %d, want 1", emb.docCalls)
	}
	if emb.calls != 0 {
		t.Fatalf("plain Embed should not be used when EmbedDocuments exists, calls = %d", emb.calls)
	}
}

func TestReembed(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{0, 1}, fail: true}
	svc := New(st, emb)

	res, err := svc.Add(ctx, Input{TenantID: "acme", Content: "需要补向量的记忆"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !res.EmbeddingFailed {
		t.Fatalf("expected initial failure, got %+v", res)
	}

	emb.fail = false
	again, err := svc.Reembed(ctx, "acme", res.ID)
	if err != nil {
		t.Fatalf("reembed: %v", err)
	}
	if !again.Embedded {
		t.Fatalf("reembed should succeed, got %+v", again)
	}

	hits, err := st.SearchVector(ctx, types.SearchQuery{TenantID: "acme"}, "fake", []float32{0, 1})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != res.ID {
		t.Fatalf("reembedded vector not searchable: %d hits", len(hits))
	}
}

func TestReembedErrors(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	// 没有 embedder
	svc := New(st, nil)
	res, err := New(st, nil).Add(ctx, Input{TenantID: "acme", Content: "无向量"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if res.Embedded {
		t.Fatal("no embedder should mean no vector")
	}
	if _, err := svc.Reembed(ctx, "acme", res.ID); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("want ErrInvalid for missing embedder, got %v", err)
	}

	// 不存在
	emb := &fakeEmbedder{vec: []float32{1}}
	svc2 := New(st, emb)
	if _, err := svc2.Reembed(ctx, "acme", "not-exist"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	// 已删除
	deleted, err := svc2.Add(ctx, Input{TenantID: "acme", Content: "会被删除"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := st.SoftDelete(ctx, "acme", deleted.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc2.Reembed(ctx, "acme", deleted.ID); err == nil {
		t.Fatal("reembedding a deleted memory should fail")
	}
}

func TestAddDefaultsTenantAndScope(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := New(st, nil)

	res, err := svc.Add(ctx, Input{Content: "没给租户和 scope"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	m, err := st.Get(ctx, "default", res.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.Scope != "default" || m.Kind != types.KindNote {
		t.Fatalf("defaults not applied: scope=%s kind=%s", m.Scope, m.Kind)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := New(st, &fakeEmbedder{vec: []float32{1}})

	res, err := svc.Add(ctx, Input{TenantID: "acme", Content: "会被删除的记忆"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := svc.Delete(ctx, "acme", res.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	m, err := st.Get(ctx, "acme", res.ID)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if m.Status != types.StatusDeleted {
		t.Fatalf("status = %s, want deleted", m.Status)
	}

	// 已删除再删：幂等成功（客户端可安全重试）
	if err := svc.Delete(ctx, "acme", res.ID); err != nil {
		t.Fatalf("second delete should be idempotent: %v", err)
	}

	if err := svc.Delete(ctx, "other", res.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-tenant delete should be not found, got %v", err)
	}
	if err := svc.Delete(ctx, "acme", "  "); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("empty id should be invalid, got %v", err)
	}
	if err := New(nil, nil).Delete(ctx, "acme", res.ID); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("nil store should be invalid, got %v", err)
	}
}

func TestWarmup(t *testing.T) {
	emb := &fakeEmbedder{vec: []float32{1, 0}}
	svc := New(nil, emb)

	if err := svc.Warmup(context.Background()); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	if emb.calls != 1 {
		t.Fatalf("warmup should embed once, got %d calls", emb.calls)
	}

	// 没有 embedder 时 warmup 是 no-op（纯关键词模式）
	if err := New(nil, nil).Warmup(context.Background()); err != nil {
		t.Fatalf("nil embedder warmup should be a no-op: %v", err)
	}
}

func TestWarmupFailureIsReturned(t *testing.T) {
	svc := New(nil, &fakeEmbedder{fail: true})
	if err := svc.Warmup(context.Background()); err == nil {
		t.Fatal("warmup should return the embedding error")
	}
}

func TestWarmupTimeout(t *testing.T) {
	emb := &fakeEmbedder{vec: []float32{1}, delay: 300 * time.Millisecond}
	svc := New(nil, emb, WithWarmupTimeout(20*time.Millisecond))

	start := time.Now()
	err := svc.Warmup(context.Background())
	if err == nil {
		t.Fatal("warmup should time out")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("warmup timeout not enforced, took %v", elapsed)
	}
}

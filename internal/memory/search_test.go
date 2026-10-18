package memory

import (
	"context"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/pkg/types"
)

// seed 写入三条记忆，并只给其中需要的几条预置向量。
func seed(t *testing.T, svc *Service, st store.Store) (m1, m2, m3 string) {
	t.Helper()
	ctx := context.Background()
	add := func(content string) string {
		res, err := svc.Add(ctx, Input{TenantID: "acme", Scope: "project:opc", Content: content})
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		return res.ID
	}
	m1 = add("许可证定为 AGPL-3.0 加商业双许可")
	m2 = add("周日强制休息，不写代码")
	m3 = add("审计日志要能导出，按时间倒序")

	// 关键：向量单独预置，避免假 embedder 把所有内容映射成同一个向量
	for id, vec := range map[string][]float32{
		m1: {1, 0, 0},
		m2: {0.9, 0.1, 0},
		m3: {0, 1, 0},
	} {
		if err := st.PutVector(ctx, id, "fake", vec); err != nil {
			t.Fatalf("put vector: %v", err)
		}
	}
	return m1, m2, m3
}

func TestSearchVectorFirst(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{1, 0, 0}}
	svc := New(st, emb)
	m1, m2, _ := seed(t, svc, st)

	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "开源许可", Limit: 3})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Degraded {
		t.Fatalf("should not degrade: %s", res.DegradeReason)
	}
	if len(res.Hits) != 3 {
		t.Fatalf("want 3 hits, got %d", len(res.Hits))
	}
	if res.Hits[0].Memory.ID != m1 || res.Hits[1].Memory.ID != m2 {
		t.Fatalf("vector order broken: %s, %s", res.Hits[0].Memory.ID, res.Hits[1].Memory.ID)
	}
	for _, h := range res.Hits {
		if len(h.MatchedBy) == 0 {
			t.Fatalf("MatchedBy empty for %s", h.Memory.Content)
		}
	}
}

func TestSearchDegradesOnEmbedFailure(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{1, 0, 0}, fail: true}
	svc := New(st, emb)
	m1, m2, _ := seed(t, svc, st)
	_ = m1
	_ = m2

	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "审计"})
	if err != nil {
		t.Fatalf("search should not fail: %v", err)
	}
	if !res.Degraded || res.DegradeReason != DegradeError {
		t.Fatalf("want degraded/vector_error, got %+v", res)
	}
	if len(res.Hits) != 1 || res.Hits[0].Memory.Content != "审计日志要能导出，按时间倒序" {
		t.Fatalf("keyword fallback failed: %v", res.Hits)
	}
}

func TestSearchDegradesOnTimeout(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{1, 0, 0}, delay: 300 * time.Millisecond}
	svc := New(st, emb, WithQueryTimeout(20*time.Millisecond))
	seed(t, svc, st)

	start := time.Now()
	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "审计"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !res.Degraded || res.DegradeReason != DegradeTimeout {
		t.Fatalf("want timeout degradation, got %+v", res)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("timeout not enforced, took %v", elapsed)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("expected keyword result, got %d", len(res.Hits))
	}
}

func TestSearchDegradesWithoutEmbedder(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := New(st, nil)
	seed(t, svc, st)

	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "审计"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !res.Degraded || res.DegradeReason != DegradeNoEmbedder {
		t.Fatalf("want no_embedder degradation, got %+v", res)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("expected keyword result, got %d", len(res.Hits))
	}
}

func TestSearchBackfillsFromKeyword(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{1, 0, 0}}
	svc := New(st, emb)

	// 只给一条记忆向量：向量命中 1 条，剩下靠关键词补
	withVec, err := svc.Add(ctx, Input{TenantID: "acme", Content: "许可证定为 AGPL-3.0"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	noVec, err := svc.Add(ctx, Input{TenantID: "acme", Content: "审计日志要能导出"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	db := st
	if err := db.PutVector(ctx, withVec.ID, "fake", []float32{1, 0, 0}); err != nil {
		t.Fatalf("put: %v", err)
	}

	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "审计", Limit: 3})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Degraded {
		t.Fatalf("vector worked, should not degrade: %s", res.DegradeReason)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("want 2 hits (1 vector + 1 keyword), got %d", len(res.Hits))
	}
	// 向量结果永远在前
	if res.Hits[0].Memory.ID != withVec.ID {
		t.Fatalf("vector hit should come first, got %q", res.Hits[0].Memory.Content)
	}
	if res.Hits[1].Memory.ID != noVec.ID {
		t.Fatalf("keyword hit missing: %q", res.Hits[1].Memory.Content)
	}
	if !contains(res.Hits[1].MatchedBy, "keyword") {
		t.Fatalf("backfilled hit should be marked keyword: %v", res.Hits[1].MatchedBy)
	}
}

func TestSearchMergesMatchedBy(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	emb := &fakeEmbedder{vec: []float32{1, 0, 0}}
	svc := New(st, emb)
	m1, _, _ := seed(t, svc, st)

	// limit 要比向量命中数大，否则不会走关键词补位、也就无法验证 matched_by 合并
	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "许可", Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 || res.Hits[0].Memory.ID != m1 {
		t.Fatalf("expected AGPL memory first, got %v", res.Hits)
	}
	got := res.Hits[0].MatchedBy
	if !contains(got, "vector") || !contains(got, "keyword") {
		t.Fatalf("MatchedBy should merge both paths: %v", got)
	}
}

func TestSearchEmptyQueryListsRecent(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := New(st, &fakeEmbedder{vec: []float32{1, 0}})

	first, err := svc.Add(ctx, Input{TenantID: "acme", Content: "第一条"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	second, err := svc.Add(ctx, Input{TenantID: "acme", Content: "第二条"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Degraded {
		t.Fatal("listing should not be degraded")
	}
	if len(res.Hits) != 2 {
		t.Fatalf("want 2 recent, got %d", len(res.Hits))
	}
	if res.Hits[0].Memory.ID != second.ID || res.Hits[1].Memory.ID != first.ID {
		t.Fatalf("recent order wrong: %s, %s", res.Hits[0].Memory.ID, res.Hits[1].Memory.ID)
	}
	if !contains(res.Hits[0].MatchedBy, "recent") {
		t.Fatalf("recent hits should be marked: %v", res.Hits[0].MatchedBy)
	}
}

func TestSearchValidationAndIsolation(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := New(st, &fakeEmbedder{vec: []float32{1, 0}})
	seed(t, svc, st)

	if _, err := svc.Search(ctx, types.SearchQuery{Query: "x"}); err == nil {
		t.Fatal("missing tenant must be invalid")
	}
	if _, err := New(nil, nil).Search(ctx, types.SearchQuery{TenantID: "acme", Query: "x"}); err == nil {
		t.Fatal("nil store must be invalid")
	}

	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "other", Query: "审计"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("cross-tenant leak: %d hits", len(res.Hits))
	}
}

func TestSearchRespectsLimit(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := New(st, &fakeEmbedder{vec: []float32{1, 0}})
	seed(t, svc, st)

	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "acme", Query: "日志", Limit: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("limit ignored: %d hits", len(res.Hits))
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/search"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/pkg/types"
)

func newStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "memalyze.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, ctx
}

func mustAdd(t *testing.T, s *Store, ctx context.Context, m *types.Memory) string {
	t.Helper()
	id, err := s.Add(ctx, m)
	if err != nil && !errors.Is(err, store.ErrDuplicate) {
		t.Fatalf("add: %v", err)
	}
	return id
}

func TestAddGetRoundTrip(t *testing.T) {
	s, ctx := newStore(t)
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Millisecond)

	m := &types.Memory{
		TenantID:  "acme",
		Scope:     "project:memalyze",
		Content:   "  默认 embedding 换成 qwen3-embedding:0.6b  ",
		Kind:      types.KindFact,
		Source:    "s9-test",
		Metadata:  map[string]any{"importance": 3.0, "tags": []any{"rag"}},
		ExpiresAt: &exp,
	}
	id := mustAdd(t, s, ctx, m)
	if id == "" {
		t.Fatal("id should be generated")
	}

	got, err := s.Get(ctx, "acme", id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "默认 embedding 换成 qwen3-embedding:0.6b" {
		t.Fatalf("content not trimmed: %q", got.Content)
	}
	if got.Kind != types.KindFact || got.Status != types.StatusActive {
		t.Fatalf("kind/status wrong: %s/%s", got.Kind, got.Status)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) {
		t.Fatalf("expires_at lost: %v", got.ExpiresAt)
	}
	if got.Metadata["importance"] != 3.0 {
		t.Fatalf("metadata lost: %v", got.Metadata)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatal("timestamps not set")
	}
}

func TestAddRejectsEmptyContent(t *testing.T) {
	s, ctx := newStore(t)
	if _, err := s.Add(ctx, &types.Memory{Content: "   "}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	if _, err := s.Add(ctx, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("want ErrInvalid for nil, got %v", err)
	}
}

func TestDedupWithinTenantAndScope(t *testing.T) {
	s, ctx := newStore(t)
	first := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Scope: "s1", Content: "同一个内容"})

	dupID, err := s.Add(ctx, &types.Memory{TenantID: "t1", Scope: "s1", Content: "同一个内容"})
	if !errors.Is(err, store.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
	if dupID != first {
		t.Fatalf("duplicate should return existing id %s, got %s", first, dupID)
	}

	// 空白差异不算不同内容
	_, err = s.Add(ctx, &types.Memory{TenantID: "t1", Scope: "s1", Content: "  同一个内容  "})
	if !errors.Is(err, store.ErrDuplicate) {
		t.Fatalf("normalized duplicate should be detected, got %v", err)
	}

	// 换 scope / 换租户 → 可以重复
	if _, err := s.Add(ctx, &types.Memory{TenantID: "t1", Scope: "s2", Content: "同一个内容"}); err != nil {
		t.Fatalf("different scope should insert: %v", err)
	}
	if _, err := s.Add(ctx, &types.Memory{TenantID: "t2", Scope: "s1", Content: "同一个内容"}); err != nil {
		t.Fatalf("different tenant should insert: %v", err)
	}
}

func TestDedupAfterSoftDelete(t *testing.T) {
	s, ctx := newStore(t)
	first := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Scope: "s1", Content: "会被删掉再写一次"})
	if err := s.SoftDelete(ctx, "t1", first); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	second := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Scope: "s1", Content: "会被删掉再写一次"})
	if second == first {
		t.Fatal("soft-deleted memory should not block a new insert")
	}
}

func TestSoftDelete(t *testing.T) {
	s, ctx := newStore(t)
	id := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Content: "要被删除的记忆"})

	if err := s.SoftDelete(ctx, "t1", id); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	got, err := s.Get(ctx, "t1", id)
	if err != nil {
		t.Fatalf("soft-deleted memory should still be readable: %v", err)
	}
	if got.Status != types.StatusDeleted {
		t.Fatalf("status = %s, want deleted", got.Status)
	}

	// 幂等
	if err := s.SoftDelete(ctx, "t1", id); err != nil {
		t.Fatalf("second delete should be a no-op, got %v", err)
	}
	if err := s.SoftDelete(ctx, "t1", "not-exist"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestTenantIsolation(t *testing.T) {
	s, ctx := newStore(t)
	id := mustAdd(t, s, ctx, &types.Memory{TenantID: "acme", Content: "acme 的机密预算三百万"})

	if _, err := s.Get(ctx, "other", id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-tenant get must fail, got %v", err)
	}
	hits, err := s.SearchKeyword(ctx, types.SearchQuery{TenantID: "other", Query: "机密预算"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("cross-tenant search leaked %d hits", len(hits))
	}
}

func TestKeywordSearchChinese(t *testing.T) {
	s, ctx := newStore(t)
	mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Content: "多租户隔离在 Store 层强制，不暴露裸 SQL"})
	mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Content: "周日强制休息，不写代码"})
	id := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Content: "审计日志要能导出，按时间倒序"})

	// 二字符查询：这是 FTS5 trigram 必然漏、二元组能命中的场景
	hits, err := s.SearchKeyword(ctx, types.SearchQuery{TenantID: "t1", Query: "审计"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != id {
		t.Fatalf("want the audit memory, got %d hits", len(hits))
	}
	if got := hits[0].MatchedBy; len(got) != 1 || got[0] != "keyword" {
		t.Fatalf("MatchedBy = %v", got)
	}

	// 多字查询：应命中"多租户"那条
	hits, err = s.SearchKeyword(ctx, types.SearchQuery{TenantID: "t1", Query: "多租户"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one hit for 多租户")
	}
}

func TestKeywordSearchFilters(t *testing.T) {
	s, ctx := newStore(t)
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Scope: "project:x", Content: "项目 x 的记忆", Kind: types.KindFact})
	mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Scope: "project:x:y", Content: "项目 x 子项目的记忆", Kind: types.KindNote})
	mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Scope: "user:alice", Content: "项目 x 的用户记忆"})
	expiredID := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Scope: "project:x", Content: "项目 x 的过期记忆", ExpiresAt: &past})
	validID := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Scope: "project:x", Content: "项目 x 的有效期记忆", ExpiresAt: &future})

	q := types.SearchQuery{TenantID: "t1", Scope: "project:x", ScopePrefix: true, Query: "项目", Limit: 20}
	hits, err := s.SearchKeyword(ctx, q)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	ids := map[string]bool{}
	for _, h := range hits {
		ids[h.Memory.ID] = true
	}
	if !ids[validID] {
		t.Fatal("unexpired memory missing")
	}
	if ids[expiredID] {
		t.Fatal("expired memory leaked into results")
	}
	// project:x 有 2 条未过期（fact + 有效期那条）+ project:x:y 有 1 条 = 3；user:alice 不算
	if len(hits) != 3 {
		t.Fatalf("scope prefix should match 3 (2 in project:x + 1 in project:x:y), got %d", len(hits))
	}
	for _, h := range hits {
		if h.Memory.Scope == "user:alice" {
			t.Fatal("scope prefix leaked user:alice")
		}
	}

	// 精确 scope
	hits, err = s.SearchKeyword(ctx, types.SearchQuery{TenantID: "t1", Scope: "project:x", Query: "项目", IncludeExpired: true})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// project:x 一共 3 条（含过期那条），IncludeExpired 打开后都要返回
	if len(hits) != 3 {
		t.Fatalf("exact scope + include expired should be 3, got %d", len(hits))
	}

	// kind 过滤
	hits, err = s.SearchKeyword(ctx, types.SearchQuery{TenantID: "t1", Query: "项目", Kinds: []types.Kind{types.KindFact}})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Memory.Kind != types.KindFact {
		t.Fatalf("kind filter failed: %d hits", len(hits))
	}

	// 软删除后不可见
	if err := s.SoftDelete(ctx, "t1", validID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	hits, err = s.SearchKeyword(ctx, types.SearchQuery{TenantID: "t1", Scope: "project:x", Query: "有效期"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatal("deleted memory still searchable")
	}
}

func TestKeywordSearchEmptyQuery(t *testing.T) {
	s, ctx := newStore(t)
	hits, err := s.SearchKeyword(ctx, types.SearchQuery{TenantID: "t1", Query: "   "})
	if err != nil || hits != nil {
		t.Fatalf("empty query should be a no-op, got %v %v", hits, err)
	}
	if _, err := s.SearchKeyword(ctx, types.SearchQuery{Query: "x"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("missing tenant must be invalid, got %v", err)
	}
}

func TestVectorSearch(t *testing.T) {
	s, ctx := newStore(t)
	nearID := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Content: "向量最接近的一条"})
	midID := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Content: "向量次接近的一条"})
	farID := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Content: "向量无关的一条"})
	otherID := mustAdd(t, s, ctx, &types.Memory{TenantID: "t2", Content: "别的租户的向量"})

	put := func(id string, vec []float32, model string) {
		t.Helper()
		if err := s.PutVector(ctx, id, model, vec); err != nil {
			t.Fatalf("put vector: %v", err)
		}
	}
	put(nearID, []float32{1, 0, 0}, "m1")
	put(midID, []float32{0.9, 0.1, 0}, "m1")
	put(farID, []float32{0, 1, 0}, "m1")
	put(otherID, []float32{1, 0, 0}, "m1")
	// 换模型后维度不同的脏数据：必须被跳过而不是让整次检索失败
	put(farID, []float32{1, 0}, "legacy")

	hits, err := s.SearchVector(ctx, types.SearchQuery{TenantID: "t1", Limit: 10}, "m1", []float32{1, 0, 0})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("want 3 same-tenant hits, got %d", len(hits))
	}
	if hits[0].Memory.ID != nearID || hits[1].Memory.ID != midID {
		t.Fatalf("ranking wrong: first=%s second=%s", hits[0].Memory.ID, hits[1].Memory.ID)
	}
	if hits[0].Score < hits[1].Score {
		t.Fatalf("scores not descending: %v", hits)
	}

	// 别的模型查不到
	hits, err = s.SearchVector(ctx, types.SearchQuery{TenantID: "t1"}, "m2", []float32{1, 0, 0})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("model isolation failed: %d hits", len(hits))
	}

	// 维度不匹配的存量数据被跳过
	hits, err = s.SearchVector(ctx, types.SearchQuery{TenantID: "t1"}, "legacy", []float32{1, 0, 0})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("mismatched-dim rows must be skipped, got %d", len(hits))
	}
}

func TestVectorSearchLimitAndExpiry(t *testing.T) {
	s, ctx := newStore(t)
	past := time.Now().Add(-time.Minute)
	expiredID := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Content: "过期向量", ExpiresAt: &past})
	liveID := mustAdd(t, s, ctx, &types.Memory{TenantID: "t1", Content: "有效向量"})
	for _, id := range []string{expiredID, liveID} {
		if err := s.PutVector(ctx, id, "m1", []float32{1, 0}); err != nil {
			t.Fatalf("put: %v", err)
		}
	}
	hits, err := s.SearchVector(ctx, types.SearchQuery{TenantID: "t1"}, "m1", []float32{1, 0})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != liveID {
		t.Fatalf("expired memory should be filtered, got %d hits", len(hits))
	}
}

func TestFTSFollowsContentUpdate(t *testing.T) {
	s, ctx := newStore(t)
	m := &types.Memory{TenantID: "t1", Content: "旧内容：苹果"}
	id := mustAdd(t, s, ctx, m)

	// 模拟后续 S10 的更新路径：改 content 时必须同步重算 fts_text（触发器负责同步索引）
	newText := "新内容：香蕉"
	_, err := s.db.ExecContext(ctx,
		`UPDATE memories SET content = ?, fts_text = ?, updated_at = ? WHERE id = ?`,
		newText, search.BigramText(newText), time.Now().UnixMilli(), id)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	_ = ctx

	old, err := s.SearchKeyword(ctx, types.SearchQuery{TenantID: "t1", Query: "苹果"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(old) != 0 {
		t.Fatal("stale token still indexed")
	}
	hits, err := s.SearchKeyword(ctx, types.SearchQuery{TenantID: "t1", Query: "香蕉"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("updated token not indexed, got %d", len(hits))
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "same.db")
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}
		if _, err := s.Add(ctx, &types.Memory{TenantID: "t1", Content: "跨重启的记忆"}); err != nil && !errors.Is(err, store.ErrDuplicate) {
			t.Fatalf("add #%d: %v", i, err)
		}
		s.Close()
	}
	// 第三次打开应能读到
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	hits, err := s.SearchKeyword(ctx, types.SearchQuery{TenantID: "t1", Query: "重启"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("want 1 hit after reopen, got %d", len(hits))
	}
}

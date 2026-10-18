package memory

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/embed"
	"github.com/mahaoboy/memalyze/internal/embed/ollama"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
	"github.com/mahaoboy/memalyze/pkg/types"
)

// TestIntegrationRealEmbedding 用真实本地模型验证 S10 的写入路径。
//
//	MEMALYZE_IT=1 go test ./internal/memory/ -run TestIntegrationRealEmbedding -v
func TestIntegrationRealEmbedding(t *testing.T) {
	if os.Getenv("MEMALYZE_IT") == "" {
		t.Skip("set MEMALYZE_IT=1 to run against local Ollama")
	}
	ctx := context.Background()
	st, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "it.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	model := "qwen3-embedding:0.6b"
	client := ollama.New("http://127.0.0.1:11434", model,
		ollama.WithHTTPClient(&http.Client{Timeout: 120 * time.Second}))
	svc := New(st, client)

	docs := []string{
		"向量检索先用暴力余弦，超过十万条再考虑索引",
		"许可证定为 AGPL-3.0 加商业双许可",
		"每周三固定两小时处理 issue 和社区",
	}
	ids := make([]string, len(docs))
	for i, d := range docs {
		res, err := svc.Add(ctx, Input{TenantID: "it", Scope: "e2e", Content: d, Kind: types.KindFact})
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
		if !res.Embedded || res.EmbeddingFailed {
			t.Fatalf("add %d should embed: %+v", i, res)
		}
		ids[i] = res.ID
	}

	// 用语义改写查询：不共享关键词，只能靠向量
	qvec, err := embed.EmbedQuery(ctx, client, "什么时候该上索引")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	if len(qvec) != 1024 {
		t.Fatalf("query dim = %d, want 1024", len(qvec))
	}
	hits, err := st.SearchVector(ctx, types.SearchQuery{TenantID: "it", Limit: 3}, model, qvec)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("want 3 hits, got %d", len(hits))
	}
	if hits[0].Memory.ID != ids[0] {
		t.Fatalf("expected index-related memory first, got %q", hits[0].Memory.Content)
	}

	// 重复写入：真实链路下也必须只留一条
	dup, err := svc.Add(ctx, Input{TenantID: "it", Scope: "e2e", Content: docs[0]})
	if err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	if !dup.Duplicate || dup.ID != ids[0] {
		t.Fatalf("duplicate handling broken: %+v", dup)
	}
}

// TestIntegrationSearch 用真实模型验证 S11 的「向量优先 + 关键词补位」。
func TestIntegrationSearch(t *testing.T) {
	if os.Getenv("MEMALYZE_IT") == "" {
		t.Skip("set MEMALYZE_IT=1 to run against local Ollama")
	}
	ctx := context.Background()
	st, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "it-search.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	client := ollama.New("http://127.0.0.1:11434", "qwen3-embedding:0.6b",
		ollama.WithHTTPClient(&http.Client{Timeout: 120 * time.Second}))
	svc := New(st, client)

	docs := []string{
		"许可证定为 AGPL-3.0 加商业双许可",
		"周日强制休息，不写代码",
		"审计日志要能导出，按时间倒序",
	}
	ids := make([]string, len(docs))
	for i, d := range docs {
		res, err := svc.Add(ctx, Input{TenantID: "it", Content: d})
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
		ids[i] = res.ID
	}

	// 语义改写：不共享关键词，必须靠向量
	res, err := svc.Search(ctx, types.SearchQuery{TenantID: "it", Query: "哪天不用上班", Limit: 3})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Degraded {
		t.Fatalf("should not degrade: %s", res.DegradeReason)
	}
	if len(res.Hits) == 0 || res.Hits[0].Memory.ID != ids[1] {
		t.Fatalf("expected 周日 memory first, got %+v", res.Hits)
	}

	// 精确词：关键词路径能命中（这里向量也在，mathed_by 至少有一个）
	res, err = svc.Search(ctx, types.SearchQuery{TenantID: "it", Query: "审计", Limit: 3})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 || res.Hits[0].Memory.ID != ids[2] {
		t.Fatalf("expected 审计 memory first, got %+v", res.Hits)
	}

	// 无 embedder → 降级但仍有结果
	degradedSvc := New(st, nil)
	res, err = degradedSvc.Search(ctx, types.SearchQuery{TenantID: "it", Query: "审计", Limit: 3})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !res.Degraded || res.DegradeReason != DegradeNoEmbedder {
		t.Fatalf("want degraded no_embedder, got %+v", res)
	}
	if len(res.Hits) == 0 {
		t.Fatal("keyword fallback should still return results")
	}
}

package sqlite

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/embed/ollama"
	"github.com/mahaoboy/memalyze/pkg/types"
)

// TestIntegrationRealEmbedding 用真实本地模型跑一遍「写入 → 向量 → 检索」。
//
// 默认跳过（依赖 Ollama）。开启方式：
//
//	MEMALYZE_IT=1 go test ./internal/store/sqlite/ -run TestIntegrationRealEmbedding -v
//
// 它验证的是 S9 最容易被忽略的一点：真实 1024 维向量的 BLOB 往返与排序。
func TestIntegrationRealEmbedding(t *testing.T) {
	if os.Getenv("MEMALYZE_IT") == "" {
		t.Skip("set MEMALYZE_IT=1 to run against local Ollama")
	}
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "it.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	model := "qwen3-embedding:0.6b"
	// 首次调用要加载模型（15s 起），集成测试放宽到 2 分钟
	c := ollama.New("http://127.0.0.1:11434", model,
		ollama.WithHTTPClient(&http.Client{Timeout: 120 * time.Second}))

	docs := []string{
		"许可证定为 AGPL-3.0 加商业双许可",
		"周日强制休息，不写代码",
		"审计日志要能导出，按时间倒序",
	}
	vecs, err := c.EmbedDocuments(ctx, docs)
	if err != nil {
		t.Fatalf("embed documents: %v", err)
	}
	if len(vecs) != len(docs) || len(vecs[0]) != 1024 {
		t.Fatalf("unexpected embedding shape: %d docs, dim %d", len(vecs), len(vecs[0]))
	}

	var ids []string
	for i, d := range docs {
		id, err := s.Add(ctx, &types.Memory{TenantID: "it", Scope: "test", Content: d})
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
		if err := s.PutVector(ctx, id, model, vecs[i]); err != nil {
			t.Fatalf("put vector %d: %v", i, err)
		}
		ids = append(ids, id)
	}

	// 向量检索：用"哪天不干活"去问，应该命中"周日强制休息"
	qvec, err := c.EmbedQuery(ctx, "哪天不用上班")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	hits, err := s.SearchVector(ctx, types.SearchQuery{TenantID: "it", Limit: 3}, model, qvec)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("want 3 hits, got %d", len(hits))
	}
	if hits[0].Memory.ID != ids[1] {
		t.Fatalf("expected 周日休息 first, got %q", hits[0].Memory.Content)
	}

	// 关键词检索：二字符"审计"
	kw, err := s.SearchKeyword(ctx, types.SearchQuery{TenantID: "it", Query: "审计"})
	if err != nil {
		t.Fatalf("keyword search: %v", err)
	}
	if len(kw) != 1 || kw[0].Memory.ID != ids[2] {
		t.Fatalf("keyword search failed: %d hits", len(kw))
	}

	// 维度校验：换一个 768 维的假模型写入，再按该模型搜索必须被跳过
	if err := s.PutVector(ctx, ids[0], "legacy-768", make([]float32, 768)); err != nil {
		t.Fatalf("put legacy: %v", err)
	}
	legacy, err := s.SearchVector(ctx, types.SearchQuery{TenantID: "it"}, "legacy-768", qvec)
	if err != nil {
		t.Fatalf("legacy search: %v", err)
	}
	if len(legacy) != 0 {
		t.Fatalf("mismatched dims must be skipped, got %d", len(legacy))
	}
}

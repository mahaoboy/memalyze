package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mahaoboy/memalyze/internal/embed"
	"github.com/mahaoboy/memalyze/internal/memory"
	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
	"github.com/mahaoboy/memalyze/pkg/types"
)

type fakeEmbedder struct {
	vec   []float32
	fail  bool
	calls int
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("ollama unavailable")
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = f.vec
	}
	return out, nil
}

func (f *fakeEmbedder) Model() string { return "fake" }
func (f *fakeEmbedder) Dim() int      { return len(f.vec) }

func newTestService(t *testing.T, e embed.Embedder) (*memory.Service, store.Store) {
	t.Helper()
	st, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "mcp.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return memory.New(st, e), st
}

func TestMemoryAddHandler(t *testing.T) {
	ctx := context.Background()
	emb := &fakeEmbedder{vec: []float32{1, 0}}
	svc, st := newTestService(t, emb)
	tools := newMemoryTools(svc, "acme")

	out, err := tools.add(ctx, memoryAddArgs{
		Content:  "  审计日志要能导出  ",
		Scope:    "project:opc",
		Kind:     "fact",
		TTLHours: 2,
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if out.ID == "" || out.Duplicate || !out.Embedded || out.EmbeddingFailed {
		t.Fatalf("unexpected output: %+v", out)
	}

	m, err := st.Get(ctx, "acme", out.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.Content != "审计日志要能导出" || m.Scope != "project:opc" || m.Kind != types.KindFact {
		t.Fatalf("arguments not converted correctly: %+v", m)
	}
	if m.ExpiresAt == nil {
		t.Fatal("ttl_hours not converted to expires_at")
	}
	if d := time.Until(*m.ExpiresAt); d < 90*time.Minute || d > 2*time.Hour+time.Minute {
		t.Fatalf("expires_at = %v, want about 2h", d)
	}

	// 重复内容：如实回传 duplicate + 已有 id，且不重复向量化
	dup, err := tools.add(ctx, memoryAddArgs{Content: "审计日志要能导出", Scope: "project:opc"})
	if err != nil {
		t.Fatalf("duplicate add should not fail: %v", err)
	}
	if !dup.Duplicate || dup.ID != out.ID {
		t.Fatalf("duplicate not passed through: %+v", dup)
	}
	if emb.calls != 1 {
		t.Fatalf("duplicate should not re-embed, calls = %d", emb.calls)
	}
}

func TestMemoryAddHandlerEmbeddingFailure(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, &fakeEmbedder{vec: []float32{1, 0}, fail: true})
	tools := newMemoryTools(svc, "acme")

	out, err := tools.add(ctx, memoryAddArgs{Content: "写入成功但向量失败"})
	if err != nil {
		t.Fatalf("embedding failure must not fail the tool call: %v", err)
	}
	if out.ID == "" || !out.EmbeddingFailed || out.Embedded {
		t.Fatalf("embedding_failed not passed through: %+v", out)
	}
	if out.EmbeddingError == "" {
		t.Fatal("embedding error detail should be returned")
	}

	// 记忆仍然可被关键词检索命中
	res, err := tools.search(ctx, memorySearchArgs{Query: "向量失败"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].Content != "写入成功但向量失败" {
		t.Fatalf("memory should still be keyword-searchable: %+v", res.Hits)
	}
}

func TestMemorySearchHandlerReportsDegradation(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, &fakeEmbedder{vec: []float32{1, 0}, fail: true})
	tools := newMemoryTools(svc, "acme")

	if _, err := tools.add(ctx, memoryAddArgs{Content: "审计日志要能导出，按时间倒序", Scope: "project:opc"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	res, err := tools.search(ctx, memorySearchArgs{Query: "审计", Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !res.Degraded || res.DegradeReason != memory.DegradeError {
		t.Fatalf("degraded/degrade_reason not passed through: %+v", res)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("keyword fallback should return 1 hit, got %d", len(res.Hits))
	}
	hit := res.Hits[0]
	if hit.Content != "审计日志要能导出，按时间倒序" || hit.Scope != "project:opc" || hit.Kind != string(types.KindNote) {
		t.Fatalf("unexpected hit fields: %+v", hit)
	}
	if !containsString(hit.MatchedBy, "keyword") {
		t.Fatalf("matched_by should contain keyword: %v", hit.MatchedBy)
	}
	if _, err := time.Parse(time.RFC3339, hit.CreatedAt); err != nil {
		t.Fatalf("created_at is not RFC3339: %q (%v)", hit.CreatedAt, err)
	}
}

func TestMemorySearchHandlerWithoutEmbedder(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, nil)
	tools := newMemoryTools(svc, "acme")

	if _, err := tools.add(ctx, memoryAddArgs{Content: "没有 Ollama 也能存"}); err != nil {
		t.Fatalf("add without embedder: %v", err)
	}
	res, err := tools.search(ctx, memorySearchArgs{Query: "Ollama"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !res.Degraded || res.DegradeReason != memory.DegradeNoEmbedder {
		t.Fatalf("want no_embedder degradation, got %+v", res)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("want 1 keyword hit, got %d", len(res.Hits))
	}
}

func TestMemoryDeleteHandler(t *testing.T) {
	ctx := context.Background()
	svc, st := newTestService(t, &fakeEmbedder{vec: []float32{1, 0}})
	tools := newMemoryTools(svc, "acme")

	added, err := tools.add(ctx, memoryAddArgs{Content: "会被删除"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	out, err := tools.delete(ctx, memoryDeleteArgs{ID: added.ID})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !out.Deleted {
		t.Fatal("deleted should be true")
	}

	m, err := st.Get(ctx, "acme", added.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.Status != types.StatusDeleted {
		t.Fatalf("status = %s, want deleted", m.Status)
	}
	list, err := tools.search(ctx, memorySearchArgs{})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list.Hits) != 0 {
		t.Fatalf("deleted memory should not be listed: %+v", list.Hits)
	}

	// 不存在的 id 必须报错，不能假装成功
	if _, err := tools.delete(ctx, memoryDeleteArgs{ID: "missing"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown id should be ErrNotFound, got %v", err)
	}
	// 已删除的 id 幂等成功
	if _, err := tools.delete(ctx, memoryDeleteArgs{ID: added.ID}); err != nil {
		t.Fatalf("delete should be idempotent: %v", err)
	}
	// 空 id 是参数错误
	if _, err := tools.delete(ctx, memoryDeleteArgs{ID: "  "}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("empty id should be ErrInvalid, got %v", err)
	}
}

// TestServerMemoryToolsProtocol 走一次真实 MCP 协议（内存 transport），
// 验证工具可见、可调用、结构化返回能被客户端解码。
func TestServerMemoryToolsProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc, _ := newTestService(t, &fakeEmbedder{vec: []float32{1, 0}})
	server := NewServer("test", svc, "acme")
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0.0.1"}, nil)

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make(map[string]bool, len(tools.Tools))
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"echo", "memory_add", "memory_search", "memory_delete"} {
		if !names[want] {
			t.Errorf("tool %q not exposed; got %v", want, names)
		}
	}

	addRes := callTool(t, ctx, clientSession, "memory_add", map[string]any{
		"content":   "许可证定为 AGPL-3.0",
		"scope":     "project:opc",
		"kind":      "fact",
		"ttl_hours": 24,
	})
	var added memoryAddOutput
	decodeStructured(t, addRes, &added)
	if addRes.IsError || added.ID == "" || !added.Embedded {
		t.Fatalf("memory_add failed: isError=%v out=%+v", addRes.IsError, added)
	}

	searchRes := callTool(t, ctx, clientSession, "memory_search", map[string]any{
		"query": "许可证",
		"limit": 5,
	})
	var found memorySearchOutput
	decodeStructured(t, searchRes, &found)
	if searchRes.IsError || len(found.Hits) != 1 || found.Hits[0].ID != added.ID {
		t.Fatalf("memory_search failed: isError=%v out=%+v", searchRes.IsError, found)
	}
	if found.Degraded || found.DegradeReason != "" {
		t.Fatalf("healthy search should not degrade: %+v", found)
	}

	deleteRes := callTool(t, ctx, clientSession, "memory_delete", map[string]any{"id": added.ID})
	var deleted memoryDeleteOutput
	decodeStructured(t, deleteRes, &deleted)
	if deleteRes.IsError || !deleted.Deleted {
		t.Fatalf("memory_delete failed: isError=%v out=%+v", deleteRes.IsError, deleted)
	}

	// 业务错误作为 tool error 回给模型（IsError=true），而不是协议错误
	missingRes := callTool(t, ctx, clientSession, "memory_delete", map[string]any{"id": "missing"})
	if !missingRes.IsError {
		t.Fatalf("unknown id should be a tool error, got %+v", missingRes)
	}
}

func callTool(t *testing.T, ctx context.Context, cs *mcpsdk.ClientSession, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if res == nil {
		t.Fatalf("call %s: nil result", name)
	}
	return res
}

func decodeStructured(t *testing.T, res *mcpsdk.CallToolResult, out any) {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode structured content: %v (%s)", err, raw)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

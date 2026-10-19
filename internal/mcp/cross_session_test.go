package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestCrossSessionMemorySharing 验证 S13 的关键前提：
// 同一进程内，会话 A 写入的记忆必须能被另一个会话 B 读到。
//
// 这里用两个独立的 MCP client session 连同一个 server（共享同一个 store），
// 模拟 harness 里的两个对话窗口。真正的跨进程验证由 docs/s13-context-injection.md
// 里的 mcporter 两步命令完成。
func TestCrossSessionMemorySharing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	svc, _ := newTestService(t, &fakeEmbedder{vec: []float32{1, 0}})
	server := NewServer("test", svc, "acme")

	sessionA := connectTestClient(t, ctx, server, "session-a")
	sessionB := connectTestClient(t, ctx, server, "session-b")

	// 会话 A：写入一条个人偏好。
	added := callTool(t, ctx, sessionA, "memory_add", map[string]any{
		"content": "周日强制休息",
		"scope":   "project:opc",
		"kind":    "preference",
	})
	var addOut memoryAddOutput
	decodeStructured(t, added, &addOut)
	if added.IsError || addOut.ID == "" || !addOut.Embedded {
		t.Fatalf("session A memory_add failed: isError=%v out=%+v", added.IsError, addOut)
	}

	// 会话 B：此前没有写过任何东西，直接检索。
	found := callTool(t, ctx, sessionB, "memory_search", map[string]any{
		"query": "休息日是哪天",
		"limit": 5,
	})
	var searchOut memorySearchOutput
	decodeStructured(t, found, &searchOut)
	if found.IsError {
		t.Fatalf("session B memory_search returned a tool error")
	}
	if searchOut.Degraded {
		t.Fatalf("unexpected degradation: %s", searchOut.DegradeReason)
	}
	if len(searchOut.Hits) != 1 {
		t.Fatalf("session B got %d hits, want 1: %+v", len(searchOut.Hits), searchOut.Hits)
	}

	hit := searchOut.Hits[0]
	if hit.ID != addOut.ID || !strings.Contains(hit.Content, "周日强制休息") {
		t.Fatalf("session B read the wrong memory: %+v", hit)
	}

	t.Logf("session A wrote id=%s; session B read content=%q matched_by=%v",
		addOut.ID, hit.Content, hit.MatchedBy)
}

func connectTestClient(t *testing.T, ctx context.Context, server *mcpsdk.Server, name string) *mcpsdk.ClientSession {
	t.Helper()

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server for %s: %v", name, err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: name, Version: "0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client %s: %v", name, err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	return clientSession
}

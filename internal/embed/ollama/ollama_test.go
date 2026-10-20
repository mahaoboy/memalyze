package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmbedSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("missing json content type")
		}
		w.Header().Set("Content-Type", "application/json")
		// 两条输入 → 两个 3 维向量
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3],[0.4,0.5,0.6]]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model")
	vecs, err := c.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 3 {
		t.Fatalf("unexpected shape: %v", vecs)
	}
	if c.Dim() != 3 {
		t.Fatalf("dim should follow model output, got %d", c.Dim())
	}
	if c.Model() != "test-model" {
		t.Fatalf("model name mismatch: %s", c.Model())
	}
}

func TestEmbedEmptyInput(t *testing.T) {
	c := New("http://127.0.0.1:1", "")
	vecs, err := c.Embed(context.Background(), nil)
	if err != nil || vecs != nil {
		t.Fatalf("empty input should be a no-op, got %v %v", vecs, err)
	}
}

func TestEmbedHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "missing").Embed(context.Background(), []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}

func TestEmbedCountMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"embeddings":[[0.1]]}`)) // 2 条输入只回 1 条
	}))
	defer srv.Close()

	_, err := New(srv.URL, "m").Embed(context.Background(), []string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "1 embeddings for 2 inputs") {
		t.Fatalf("expected count mismatch error, got %v", err)
	}
}

func TestEmbedTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"embeddings":[[0.1]]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "m", WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
	start := time.Now()
	_, err := c.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("timeout not enforced, took %v", time.Since(start))
	}
}

// TestEmbedSendsKeepAlive 确认 keep_alive 真的进了 /api/embed 请求体。
func TestEmbedSendsKeepAlive(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		got = req.KeepAlive
		_, _ = w.Write([]byte(`{"embeddings":[[0.1]]}`))
	}))
	defer srv.Close()

	// 默认值：30m，避免空闲后重新加载模型。
	if _, err := New(srv.URL, "m").Embed(context.Background(), []string{"a"}); err != nil {
		t.Fatalf("embed default: %v", err)
	}
	if got != "30m" {
		t.Fatalf("default keep_alive = %q, want 30m", got)
	}

	if _, err := New(srv.URL, "m", WithKeepAlive("2h")).Embed(context.Background(), []string{"a"}); err != nil {
		t.Fatalf("embed custom: %v", err)
	}
	if got != "2h" {
		t.Fatalf("custom keep_alive = %q, want 2h", got)
	}
}

// TestEmbedContextDeadline 确认默认 client 不写死 10s 超时，
// 截止时间由 context 控制（冷加载要 20s，但检索仍应 50ms 就放弃）。
func TestEmbedContextDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"embeddings":[[0.1]]}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := New(srv.URL, "m").Embed(ctx, []string{"a"})
	if err == nil {
		t.Fatal("expected context deadline error")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("context deadline not enforced, took %v", time.Since(start))
	}
}

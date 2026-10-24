package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8080":  true,
		"localhost:8080":  true,
		"[::1]:8080":      true,
		":8080":           false,
		"0.0.0.0:8080":    false,
		"192.168.1.10:80": false,
		"example.com:80":  false,
	}
	for addr, want := range cases {
		if got := IsLoopbackAddr(addr); got != want {
			t.Errorf("IsLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestValidateBindRefusesUnauthenticatedPublicListener(t *testing.T) {
	if err := validateBind("127.0.0.1:8080", ""); err != nil {
		t.Fatalf("loopback without token should pass: %v", err)
	}
	if err := validateBind("0.0.0.0:8080", ""); err == nil {
		t.Fatal("expected refusal for public listener without token")
	} else if !strings.Contains(err.Error(), "MEMALYZE_HTTP_TOKEN") {
		t.Fatalf("error should name the missing variable: %v", err)
	}
	if err := validateBind("0.0.0.0:8080", "s3cret"); err != nil {
		t.Fatalf("public listener with token should pass: %v", err)
	}
	if err := validateBind("", "x"); err == nil {
		t.Fatal("empty addr should fail")
	}
}

func TestHealthz(t *testing.T) {
	h := HTTPHandler(HTTPOptions{Addr: "127.0.0.1:0", Version: "test"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestMCPRequiresTokenWhenConfigured(t *testing.T) {
	h := HTTPHandler(HTTPOptions{Version: "test", Token: "s3cret"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: status = %d, want 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("missing WWW-Authenticate header")
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", rec.Code)
	}

	// 正确 token 会进入 MCP handler（空 body 不是合法握手，所以只要不是 401 即可）
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer s3cret")
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("valid token rejected: %d %s", rec.Code, rec.Body.String())
	}
}

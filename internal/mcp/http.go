package mcp

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mahaoboy/memalyze/internal/memory"
)

// HTTPOptions 是 HTTP 传输的配置（S16：让 Docker 部署有真正的入口）。
type HTTPOptions struct {
	// Addr 是监听地址，如 ":8080" 或 "127.0.0.1:8080"。
	Addr string
	// Version 写进 MCP 的 implementation 信息。
	Version string
	// Tenant 是默认租户。
	Tenant string
	// Token 非空时，/mcp 要求 Authorization: Bearer <token>。
	Token string
	// Service 是业务层。
	Service *memory.Service
}

// ServeHTTP 在 addr 上提供 Streamable HTTP 传输，阻塞直到 ctx 取消。
//
// 安全默认：绑定非回环地址时必须配置 Token，否则拒绝启动——
// 一个没有认证的「企业记忆库」比没有它更危险。
func ServeHTTP(ctx context.Context, opts HTTPOptions) error {
	if err := validateBind(opts.Addr, opts.Token); err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              opts.Addr,
		Handler:           HTTPHandler(opts),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			return fmt.Errorf("mcp: http shutdown: %w", err)
		}
		return nil
	}
}

// HTTPHandler 组装路由（单独导出，便于用 httptest 做无端口测试）。
func HTTPHandler(opts HTTPOptions) http.Handler {
	streamable := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return NewServer(opts.Version, opts.Service, opts.Tenant)
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", requireToken(opts.Token, streamable))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","version":%q}`+"\n", opts.Version)
	})
	return mux
}

func requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="memalyze"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// validateBind 拒绝「对外监听但没有 token」的组合。
func validateBind(addr, token string) error {
	if strings.TrimSpace(addr) == "" {
		return errors.New("mcp: empty http address")
	}
	if token != "" {
		return nil
	}
	if IsLoopbackAddr(addr) {
		return nil
	}
	return fmt.Errorf("mcp: refusing to listen on %q without MEMALYZE_HTTP_TOKEN: "+
		"an unauthenticated memory store exposed to the network is a data breach waiting to happen", addr)
}

// IsLoopbackAddr 报告监听地址是否只对本机可见。
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	switch strings.Trim(host, "[]") {
	case "", "0.0.0.0", "::":
		return false
	case "localhost":
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

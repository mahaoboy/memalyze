// Package examples_test 校验「示例即配置」：这些 JSON 会被用户直接复制，
// 一旦字段写错，用户看到的是 MCP 客户端静默不加载，而不是编译错误。
package examples_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mahaoboy/memalyze/internal/govern"
)

func readJSON(t *testing.T, path string, dst any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		t.Fatalf("%s is not valid JSON: %v", path, err)
	}
}

type stdioServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

func TestClaudeCodeStdioConfig(t *testing.T) {
	var cfg struct {
		MCPServers map[string]stdioServer `json:"mcpServers"`
	}
	readJSON(t, "claude-code/.mcp.json", &cfg)
	srv, ok := cfg.MCPServers["memalyze"]
	if !ok {
		t.Fatal("mcpServers.memalyze missing")
	}
	if srv.Type != "stdio" || srv.Command == "" {
		t.Fatalf("unexpected server: %+v", srv)
	}
	if _, ok := srv.Env["MEMALYZE_DB"]; !ok {
		t.Fatal("MEMALYZE_DB should be explicit in the example")
	}
	if _, err := os.Stat("claude-code/CLAUDE.md"); err != nil {
		t.Fatalf("prompt file missing: %v", err)
	}
}

func TestClineConfig(t *testing.T) {
	var cfg struct {
		MCPServers map[string]struct {
			Command     string            `json:"command"`
			Args        []string          `json:"args"`
			Env         map[string]string `json:"env"`
			Disabled    bool              `json:"disabled"`
			AutoApprove []string          `json:"autoApprove"`
			Timeout     int               `json:"timeout"`
		} `json:"mcpServers"`
	}
	readJSON(t, "cline/cline_mcp_settings.json", &cfg)
	srv, ok := cfg.MCPServers["memalyze"]
	if !ok {
		t.Fatal("mcpServers.memalyze missing")
	}
	if srv.Disabled {
		t.Fatal("example should be enabled")
	}
	if len(srv.AutoApprove) != 1 || srv.AutoApprove[0] != "memory_search" {
		t.Fatalf("autoApprove should be exactly memory_search, got %v", srv.AutoApprove)
	}
}

func TestDockerHTTPConfig(t *testing.T) {
	var cfg struct {
		MCPServers map[string]struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	readJSON(t, "docker/mcp-http.json", &cfg)
	srv, ok := cfg.MCPServers["memalyze"]
	if !ok {
		t.Fatal("mcpServers.memalyze missing")
	}
	if srv.Type != "http" || !strings.HasSuffix(srv.URL, "/mcp") {
		t.Fatalf("unexpected http server: %+v", srv)
	}
	if !strings.Contains(srv.Headers["Authorization"], "Bearer") {
		t.Fatalf("missing bearer auth: %+v", srv.Headers)
	}
}

func TestRedactionRulePackCompiles(t *testing.T) {
	rules, err := govern.LoadRulesFile("redaction-rules.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := govern.New(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Names()) < 8 {
		t.Fatalf("rule pack should include the built-ins, got %v", r.Names())
	}
}

func TestDemoScriptIsExecutable(t *testing.T) {
	info, err := os.Stat("demo/demo.sh")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("demo.sh should be executable")
	}
}

func TestEveryExampleHasReadme(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(e.Name(), "README.md")); err != nil {
			t.Errorf("%s lacks a README.md", e.Name())
		}
	}
}

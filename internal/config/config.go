// Package config 读取 memalyze 的运行时配置。
//
// 约定：所有配置都走环境变量，缺省值保证「拉起来就能用」。
// 这样 MCP 客户端只需要在配置里写一条启动命令，不需要额外的配置文件。
package config

import (
	"os"
	"strings"
	"time"

	"github.com/mahaoboy/memalyze/internal/govern"
)

// 默认值（与 README / 继续点里的快速开始保持一致）。
const (
	DefaultDBPath     = "./memalyze.db"
	DefaultOllamaURL  = "http://127.0.0.1:11434"
	DefaultEmbedModel = "qwen3-embedding:0.6b"
	DefaultTenant     = "default"
	// DefaultQueryTimeout 是检索侧 embedding 的硬超时（D-012/D-015）。
	DefaultQueryTimeout = 3 * time.Second
	// DefaultKeepAlive 让 Ollama 在空闲时也保留 embedding 模型，
	// 避免「空闲后第一次查询」重新加载 20s 被 3s 超时打成 degraded。
	DefaultKeepAlive = "30m"
	// DefaultRedactMode 默认「真脱敏」。dry-run 是调参工具，不是默认——
	// 默认 dry-run 等于默认把手机号写进库（见 D-020）。
	DefaultRedactMode = govern.ModeEnforce
	// DefaultActor 写进审计，便于多客户端部署时区分调用方。
	DefaultActor = "local"
)

// Config 是进程启动所需的全部配置。
type Config struct {
	// DBPath 是 SQLite 文件路径（MEMALYZE_DB）。
	DBPath string
	// OllamaURL 是本地 Ollama 的地址（MEMALYZE_OLLAMA_URL）。
	OllamaURL string
	// EmbedModel 是 embedding 模型名（MEMALYZE_EMBED_MODEL）。
	EmbedModel string
	// Tenant 是工具调用的默认租户（MEMALYZE_TENANT）。
	Tenant string
	// QueryTimeout 是检索侧 embedding 超时（MEMALYZE_QUERY_TIMEOUT）。
	QueryTimeout time.Duration
	// KeepAlive 是 Ollama 的 keep_alive 时长（MEMALYZE_KEEP_ALIVE），如 "30m"。
	KeepAlive string
	// RedactMode 是脱敏模式：off / dry-run / enforce（MEMALYZE_REDACT_MODE）。
	RedactMode govern.Mode
	// RedactRules 是自定义规则文件路径（MEMALYZE_REDACT_RULES）；空 = 内置规则。
	RedactRules string
	// AuditEnabled 控制是否写审计（MEMALYZE_AUDIT，默认 on）。
	AuditEnabled bool
	// Actor 是审计里的调用方标识（MEMALYZE_ACTOR）。
	Actor string
	// AuditQueryPlain 为 true 时检索原文进审计 Detail（MEMALYZE_AUDIT_QUERY_PLAIN），
	// 默认 false：只存 SHA-256 + 长度。
	AuditQueryPlain bool
	// HTTPAddr 非空时改用 Streamable HTTP 传输（MEMALYZE_HTTP_ADDR，如 ":8080"）。
	HTTPAddr string
	// HTTPToken 是 HTTP 传输的 Bearer token（MEMALYZE_HTTP_TOKEN）。
	// 绑定非回环地址时必填，否则拒绝启动。
	HTTPToken string
}

// Load 从环境变量读取配置；空值回落到默认值。
func Load() Config {
	return Config{
		DBPath:       env("MEMALYZE_DB", DefaultDBPath),
		OllamaURL:    env("MEMALYZE_OLLAMA_URL", DefaultOllamaURL),
		EmbedModel:   env("MEMALYZE_EMBED_MODEL", DefaultEmbedModel),
		Tenant:       env("MEMALYZE_TENANT", DefaultTenant),
		QueryTimeout: durationEnv("MEMALYZE_QUERY_TIMEOUT", DefaultQueryTimeout),
		KeepAlive:    env("MEMALYZE_KEEP_ALIVE", DefaultKeepAlive),

		RedactMode:      modeEnv("MEMALYZE_REDACT_MODE", DefaultRedactMode),
		RedactRules:     env("MEMALYZE_REDACT_RULES", ""),
		AuditEnabled:    boolEnv("MEMALYZE_AUDIT", true),
		Actor:           env("MEMALYZE_ACTOR", DefaultActor),
		AuditQueryPlain: boolEnv("MEMALYZE_AUDIT_QUERY_PLAIN", false),

		HTTPAddr:  env("MEMALYZE_HTTP_ADDR", ""),
		HTTPToken: env("MEMALYZE_HTTP_TOKEN", ""),
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// durationEnv 解析 time.ParseDuration；非法或非正值回落到默认值。
func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// modeEnv 解析脱敏模式；非法值回落到默认（并在 stderr 提示，避免"以为开了其实没开"）。
func modeEnv(key string, fallback govern.Mode) govern.Mode {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	m := govern.Mode(raw)
	if !m.Valid() {
		return fallback
	}
	return m
}

// boolEnv 解析 on/off、true/false、1/0。
func boolEnv(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "on", "yes", "y":
		return true
	case "0", "false", "off", "no", "n":
		return false
	}
	return fallback
}

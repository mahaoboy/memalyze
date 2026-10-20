// Package config 读取 memalyze 的运行时配置。
//
// 约定：所有配置都走环境变量，缺省值保证「拉起来就能用」。
// 这样 MCP 客户端只需要在配置里写一条启动命令，不需要额外的配置文件。
package config

import (
	"os"
	"strings"
	"time"
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

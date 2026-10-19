// Package config 读取 memalyze 的运行时配置。
//
// 约定：所有配置都走环境变量，缺省值保证「拉起来就能用」。
// 这样 MCP 客户端只需要在配置里写一条启动命令，不需要额外的配置文件。
package config

import (
	"os"
	"strings"
)

// 默认值（与 README / 继续点里的快速开始保持一致）。
const (
	DefaultDBPath     = "./memalyze.db"
	DefaultOllamaURL  = "http://127.0.0.1:11434"
	DefaultEmbedModel = "qwen3-embedding:0.6b"
	DefaultTenant     = "default"
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
}

// Load 从环境变量读取配置；空值回落到默认值。
func Load() Config {
	return Config{
		DBPath:     env("MEMALYZE_DB", DefaultDBPath),
		OllamaURL:  env("MEMALYZE_OLLAMA_URL", DefaultOllamaURL),
		EmbedModel: env("MEMALYZE_EMBED_MODEL", DefaultEmbedModel),
		Tenant:     env("MEMALYZE_TENANT", DefaultTenant),
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

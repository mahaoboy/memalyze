package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{
		"MEMALYZE_DB", "MEMALYZE_OLLAMA_URL", "MEMALYZE_EMBED_MODEL", "MEMALYZE_TENANT",
		"MEMALYZE_QUERY_TIMEOUT", "MEMALYZE_KEEP_ALIVE",
	} {
		t.Setenv(key, "")
	}

	got := Load()
	want := Config{
		DBPath:       DefaultDBPath,
		OllamaURL:    DefaultOllamaURL,
		EmbedModel:   DefaultEmbedModel,
		Tenant:       DefaultTenant,
		QueryTimeout: DefaultQueryTimeout,
		KeepAlive:    DefaultKeepAlive,
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("MEMALYZE_DB", "/tmp/custom.db")
	t.Setenv("MEMALYZE_OLLAMA_URL", "http://ollama.internal:11434")
	t.Setenv("MEMALYZE_EMBED_MODEL", "nomic-embed-text")
	t.Setenv("MEMALYZE_TENANT", "acme")
	t.Setenv("MEMALYZE_QUERY_TIMEOUT", "25s")
	t.Setenv("MEMALYZE_KEEP_ALIVE", "2h")

	got := Load()
	want := Config{
		DBPath:       "/tmp/custom.db",
		OllamaURL:    "http://ollama.internal:11434",
		EmbedModel:   "nomic-embed-text",
		Tenant:       "acme",
		QueryTimeout: 25 * time.Second,
		KeepAlive:    "2h",
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadInvalidQueryTimeoutFallsBack(t *testing.T) {
	t.Setenv("MEMALYZE_QUERY_TIMEOUT", "not-a-duration")
	if got := Load().QueryTimeout; got != DefaultQueryTimeout {
		t.Fatalf("QueryTimeout = %v, want default %v", got, DefaultQueryTimeout)
	}

	t.Setenv("MEMALYZE_QUERY_TIMEOUT", "-5s")
	if got := Load().QueryTimeout; got != DefaultQueryTimeout {
		t.Fatalf("negative QueryTimeout = %v, want default %v", got, DefaultQueryTimeout)
	}
}

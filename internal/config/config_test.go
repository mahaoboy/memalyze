package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"MEMALYZE_DB", "MEMALYZE_OLLAMA_URL", "MEMALYZE_EMBED_MODEL", "MEMALYZE_TENANT"} {
		t.Setenv(key, "")
	}

	got := Load()
	want := Config{
		DBPath:     DefaultDBPath,
		OllamaURL:  DefaultOllamaURL,
		EmbedModel: DefaultEmbedModel,
		Tenant:     DefaultTenant,
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

	got := Load()
	want := Config{
		DBPath:     "/tmp/custom.db",
		OllamaURL:  "http://ollama.internal:11434",
		EmbedModel: "nomic-embed-text",
		Tenant:     "acme",
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

package config

import (
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/internal/govern"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{
		"MEMALYZE_DB", "MEMALYZE_OLLAMA_URL", "MEMALYZE_EMBED_MODEL", "MEMALYZE_TENANT",
		"MEMALYZE_QUERY_TIMEOUT", "MEMALYZE_KEEP_ALIVE",
		"MEMALYZE_REDACT_MODE", "MEMALYZE_REDACT_RULES", "MEMALYZE_AUDIT",
		"MEMALYZE_ACTOR", "MEMALYZE_AUDIT_QUERY_PLAIN",
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
		RedactMode:   DefaultRedactMode,
		AuditEnabled: true,
		Actor:        DefaultActor,
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
	t.Setenv("MEMALYZE_REDACT_MODE", "dry-run")
	t.Setenv("MEMALYZE_REDACT_RULES", "/etc/memalyze/rules.json")
	t.Setenv("MEMALYZE_AUDIT", "off")
	t.Setenv("MEMALYZE_ACTOR", "claude-code")
	t.Setenv("MEMALYZE_AUDIT_QUERY_PLAIN", "1")

	got := Load()
	want := Config{
		DBPath:          "/tmp/custom.db",
		OllamaURL:       "http://ollama.internal:11434",
		EmbedModel:      "nomic-embed-text",
		Tenant:          "acme",
		QueryTimeout:    25 * time.Second,
		KeepAlive:       "2h",
		RedactMode:      govern.ModeDryRun,
		RedactRules:     "/etc/memalyze/rules.json",
		AuditEnabled:    false,
		Actor:           "claude-code",
		AuditQueryPlain: true,
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

func TestLoadInvalidRedactModeFallsBack(t *testing.T) {
	t.Setenv("MEMALYZE_REDACT_MODE", "banana")
	if got := Load().RedactMode; got != DefaultRedactMode {
		t.Fatalf("RedactMode = %v, want default %v", got, DefaultRedactMode)
	}
}

func TestLoadRedactModeOff(t *testing.T) {
	t.Setenv("MEMALYZE_REDACT_MODE", "OFF")
	if got := Load().RedactMode; got != govern.ModeOff {
		t.Fatalf("RedactMode = %v, want off", got)
	}
}

func TestBoolEnvVariants(t *testing.T) {
	for raw, want := range map[string]bool{
		"on": true, "true": true, "1": true, "yes": true,
		"off": false, "false": false, "0": false, "no": false,
	} {
		t.Setenv("MEMALYZE_AUDIT", raw)
		if got := Load().AuditEnabled; got != want {
			t.Fatalf("MEMALYZE_AUDIT=%q → %v, want %v", raw, got, want)
		}
	}
}

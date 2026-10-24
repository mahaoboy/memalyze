package main

import (
	"testing"
	"time"

	"github.com/mahaoboy/memalyze/pkg/types"
)

func TestParseBoundDateOnlyEndCoversWholeDay(t *testing.T) {
	start, err := parseBound("2026-10-01", false)
	if err != nil {
		t.Fatal(err)
	}
	if start.Hour() != 0 || start.Minute() != 0 {
		t.Fatalf("start = %v", start)
	}
	end, err := parseBound("2026-10-01", true)
	if err != nil {
		t.Fatal(err)
	}
	if end.Hour() != 23 || end.Minute() != 59 || end.Second() != 59 {
		t.Fatalf("end = %v", end)
	}
	if end.Sub(start) != 24*time.Hour-time.Millisecond {
		t.Fatalf("range = %v", end.Sub(start))
	}
}

func TestParseBoundAcceptsRFC3339AndSpaceForm(t *testing.T) {
	for _, in := range []string{"2026-10-01T12:30:00+08:00", "2026-10-01 12:30:00", "2026-10-01 12:30"} {
		if _, err := parseBound(in, false); err != nil {
			t.Fatalf("parseBound(%q): %v", in, err)
		}
	}
	if _, err := parseBound("yesterday", false); err == nil {
		t.Fatal("expected error for garbage input")
	}
}

func TestDBPathOrFallsBackToConfig(t *testing.T) {
	t.Setenv("MEMALYZE_DB", "/tmp/from-env.db")
	if got := dbPathOr(""); got != "/tmp/from-env.db" {
		t.Fatalf("got %q", got)
	}
	if got := dbPathOr("/tmp/explicit.db"); got != "/tmp/explicit.db" {
		t.Fatalf("got %q", got)
	}
}

func TestViewOfMemory(t *testing.T) {
	now := time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC)
	exp := now.Add(time.Hour)
	v := viewOf(types.Memory{
		ID: "m1", TenantID: "acme", Scope: "crm", Kind: types.KindFact,
		Status: types.StatusActive, Content: "x", CreatedAt: now, ExpiresAt: &exp,
	})
	if v.ID != "m1" || v.Kind != "fact" || v.CreatedAt != "2026-10-25T01:00:00Z" {
		t.Fatalf("unexpected view: %+v", v)
	}
	if v.ExpiresAt != "2026-10-25T02:00:00Z" {
		t.Fatalf("expires_at = %q", v.ExpiresAt)
	}
	if v := viewOf(types.Memory{CreatedAt: now}); v.ExpiresAt != "" {
		t.Fatalf("expires_at should be empty: %q", v.ExpiresAt)
	}
}

func TestShortID(t *testing.T) {
	if got := shortID("0123456789ABCDEF"); got != "0123456789…" {
		t.Fatalf("got %q", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

package id

import (
	"strings"
	"testing"
	"time"
)

func TestNewFormat(t *testing.T) {
	got := New()
	if len(got) != encodedLen {
		t.Fatalf("length = %d, want %d", len(got), encodedLen)
	}
	for _, c := range got {
		if !strings.ContainsRune(crockford, c) {
			t.Fatalf("invalid char %q in %q", c, got)
		}
	}
}

func TestNewUnique(t *testing.T) {
	seen := make(map[string]bool, 2000)
	for i := 0; i < 2000; i++ {
		s := New()
		if seen[s] {
			t.Fatalf("duplicate id at iteration %d: %s", i, s)
		}
		seen[s] = true
	}
}

func TestSortableByTime(t *testing.T) {
	// 同一毫秒内随机部分不保证有序，所以跨毫秒比较
	a := New()
	time.Sleep(3 * time.Millisecond)
	b := New()
	if !(a < b) {
		t.Fatalf("expected %s < %s (lexicographic by time)", a, b)
	}
}

func TestTimeRoundTrip(t *testing.T) {
	before := time.Now().UnixMilli()
	s := New()
	after := time.Now().UnixMilli()

	got, err := Time(s)
	if err != nil {
		t.Fatalf("Time: %v", err)
	}
	ms := got.UnixMilli()
	if ms < before || ms > after {
		t.Fatalf("decoded ms %d not in [%d,%d]", ms, before, after)
	}
}

func TestTimeRejectsGarbage(t *testing.T) {
	for _, c := range []string{"", "short", strings.Repeat("I", 26), strings.Repeat("Z", 27)} {
		if _, err := Time(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}

package types

import "testing"

func TestKindValid(t *testing.T) {
	for _, k := range []Kind{KindFact, KindPreference, KindEpisode, KindNote} {
		if !k.Valid() {
			t.Errorf("%q should be valid", k)
		}
	}
	for _, k := range []Kind{"", "Note", "fact ", "unknown"} {
		if k.Valid() {
			t.Errorf("%q should be invalid", k)
		}
	}
}

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusActive, StatusArchived, StatusDeleted} {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []Status{"", "Active", "gone"} {
		if s.Valid() {
			t.Errorf("%q should be invalid", s)
		}
	}
}

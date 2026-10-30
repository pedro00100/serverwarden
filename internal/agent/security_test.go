package agent

import (
	"slices"
	"testing"
	"time"
)

func TestAlertsByHour(t *testing.T) {
	now := time.Date(2026, 10, 30, 14, 30, 0, 0, time.UTC)
	b := []byte(`[
		{"created_at": "2026-10-30T14:10:00Z"},
		{"created_at": "2026-10-30T14:29:00Z"},
		{"created_at": "2026-10-30T12:00:00Z"},
		{"created_at": "2026-10-29T15:00:00Z"},
		{"created_at": "2026-10-29T10:00:00Z"},
		{"created_at": "not a date"}
	]`)
	total, hours := alertsByHour(b, now)
	if total != 6 {
		t.Errorf("total = %d, want 6", total)
	}
	want := make([]int, 24)
	want[23], want[21], want[0] = 2, 1, 1
	if !slices.Equal(hours, want) {
		t.Errorf("hours = %v, want %v", hours, want)
	}

	if total, hours := alertsByHour([]byte("null"), now); total != 0 || len(hours) != 24 {
		t.Errorf("empty list: %d %v", total, hours)
	}
	if _, hours := alertsByHour([]byte("garbage"), now); hours != nil {
		t.Errorf("garbage: %v", hours)
	}
}

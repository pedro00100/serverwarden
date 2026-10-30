package domain

import "testing"

func TestNewFinding(t *testing.T) {
	f := NewFinding(LevelWarning, "disk_reallocated", Args{"device": "sda", "count": 147})
	if f.Text != "Disk sda: 147 reallocated sectors" {
		t.Errorf("text = %q", f.Text)
	}
}

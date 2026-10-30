package agent

import "testing"

func TestCountUpdates(t *testing.T) {
	apt := "NOTE: This is only a simulation!\nInst libc6 [2.41-1] (2.41-2 Debian:13)\nInst tzdata [a] (b)\nConf libc6 (2.41-2)\n"
	if n := countPrefix("Inst ")(apt); n != 2 {
		t.Errorf("apt = %d", n)
	}
	dnf := "\nkernel.x86_64   6.9.1-200.fc40   updates\nvim-minimal.x86_64  2:9.1-1.fc40  updates\nObsoleting Packages\ngrub2.x86_64  1:2.06  updates\n"
	if n := countDnf(dnf); n != 2 {
		t.Errorf("dnf = %d", n)
	}
	if n := countNonEmpty("linux 6.9.1 -> 6.9.2\n\nvim 9.1 -> 9.2\n"); n != 2 {
		t.Errorf("non-empty = %d", n)
	}
}

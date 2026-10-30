package password

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	h, err := Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("format: %s", h)
	}
	if !Verify(h, "correct horse battery") {
		t.Error("the right password must verify")
	}
	if Verify(h, "correct horse batterY") {
		t.Error("a wrong password must not verify")
	}
	other, _ := Hash("correct horse battery")
	if other == h {
		t.Error("each hash needs its own salt")
	}
}

func TestPolicyAndGarbage(t *testing.T) {
	if _, err := Hash("short"); err == nil {
		t.Error("short passwords must be rejected")
	}
	for _, bad := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$AA$AA", "$argon2id$v=19$m=999999999,t=1,p=1$AA$AA"} {
		if Verify(bad, "anything long enough") {
			t.Errorf("%q must never verify", bad)
		}
	}
}

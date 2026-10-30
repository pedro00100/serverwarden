package i18n

import "testing"

func TestCatalogsMatch(t *testing.T) {
	for k := range en {
		if _, ok := es[k]; !ok {
			t.Errorf("es is missing %q", k)
		}
	}
	for k := range es {
		if _, ok := en[k]; !ok {
			t.Errorf("es has %q, which en lacks", k)
		}
	}
}

func TestT(t *testing.T) {
	if got := T("es", "finding.disk_usage", map[string]any{"pct": 91}); got != "Disco al 91 %" {
		t.Errorf("es: %q", got)
	}
	if got := T("fr", "finding.disk_usage", map[string]any{"pct": 91}); got != "Disk at 91%" {
		t.Errorf("fallback to en: %q", got)
	}
	if got := T("en", "no.such.key", nil); got != "no.such.key" {
		t.Errorf("missing key: %q", got)
	}
}

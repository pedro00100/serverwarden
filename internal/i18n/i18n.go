// Package i18n renders server-side messages (notifications) in the configured
// language. The web UI has its own catalog in web/src/lib/i18n.
package i18n

import (
	"fmt"
	"strings"
)

var catalogs = map[string]map[string]string{"en": en, "es": es}

// T renders key in lang, falling back to English and then to the key itself.
func T(lang, key string, args map[string]any) string {
	text, ok := catalogs[lang][key]
	if !ok {
		if text, ok = en[key]; !ok {
			text = key
		}
	}
	for k, v := range args {
		text = strings.ReplaceAll(text, "{"+k+"}", fmt.Sprint(v))
	}
	return text
}

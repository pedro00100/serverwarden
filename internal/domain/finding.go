package domain

import "github.com/pedro00100/serverwarden/internal/i18n"

// Finding is one concrete reason why a server is not perfect. Code and Args let
// the UI and notifications translate it; Text is the English fallback.
type Finding struct {
	Level Level  `json:"level"`
	Code  string `json:"code"`
	Args  Args   `json:"args,omitempty"`
	Text  string `json:"text"`
}

type Args = map[string]any

func NewFinding(l Level, code string, args Args) Finding {
	return Finding{Level: l, Code: code, Args: args, Text: i18n.T("en", "finding."+code, args)}
}

package notify

import (
	"context"
	"strings"
)

var ntfyType = &Type{
	ID: "ntfy", Name: "ntfy",
	Fields: []Field{
		{Key: "server", Label: "Server", Kind: "url", Required: true, Default: "https://ntfy.sh"},
		{Key: "topic", Label: "Topic", Kind: "text", Required: true},
		{Key: "token", Label: "Access token", Kind: "text", Secret: true, Help: "Only for protected topics."},
	},
	send: func(ctx context.Context, s map[string]string, m Message) error {
		priority := map[string]int{"info": 3, "warning": 4, "alert": 5}[m.Event.Level]
		if m.Event.Resolved {
			priority = 3
		}
		headers := map[string]string{}
		if s["token"] != "" {
			headers["Authorization"] = "Bearer " + s["token"]
		}
		// JSON publishing (POST to the server root) keeps UTF-8 titles intact.
		return postJSON(ctx, strings.TrimRight(s["server"], "/"), map[string]any{
			"topic": s["topic"], "title": m.Title, "message": m.Body, "priority": priority,
		}, headers)
	},
}

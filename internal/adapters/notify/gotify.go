package notify

import (
	"context"
	"strings"
)

var gotifyType = &Type{
	ID: "gotify", Name: "Gotify",
	Fields: []Field{
		{Key: "server", Label: "Server", Kind: "url", Required: true},
		{Key: "token", Label: "App token", Kind: "text", Secret: true, Required: true},
	},
	send: func(ctx context.Context, s map[string]string, m Message) error {
		priority := map[string]int{"info": 2, "warning": 5, "alert": 8}[m.Event.Level]
		return postJSON(ctx, strings.TrimRight(s["server"], "/")+"/message", map[string]any{
			"title": m.Title, "message": m.Body, "priority": priority,
		}, map[string]string{"X-Gotify-Key": s["token"]})
	},
}

package notify

import "context"

var webhookType = &Type{
	ID: "webhook", Name: "Webhook",
	Fields: []Field{
		{Key: "url", Label: "URL", Kind: "url", Secret: true, Required: true},
		{Key: "header", Label: "Header name", Kind: "text", Help: "Optional, e.g. Authorization."},
		{Key: "header_value", Label: "Header value", Kind: "text", Secret: true},
	},
	send: func(ctx context.Context, s map[string]string, m Message) error {
		e := m.Event
		body := map[string]any{
			"panel": m.Panel, "kind": e.Kind, "level": e.Level, "server": e.Server, "key": e.Key, "args": e.Args,
			"resolved": e.Resolved, "repeat": e.Repeat, "time": e.Time, "title": m.Title, "message": m.Body,
		}
		var headers map[string]string
		if s["header"] != "" {
			headers = map[string]string{s["header"]: s["header_value"]}
		}
		return postJSON(ctx, s["url"], body, headers)
	},
}

package notify

import "context"

var discordType = &Type{
	ID: "discord", Name: "Discord",
	Fields: []Field{
		{Key: "url", Label: "Webhook URL", Kind: "url", Secret: true, Required: true, Help: "Channel settings → Integrations → Webhooks. Add ?thread_id=… to post in a forum thread."},
	},
	send: func(ctx context.Context, s map[string]string, m Message) error {
		return postJSON(ctx, s["url"], map[string]string{"username": m.Panel, "content": truncate(m.Body, 2000)}, nil)
	},
}

package notify

import "context"

var slackType = &Type{
	ID: "slack", Name: "Slack",
	Fields: []Field{
		{Key: "url", Label: "Incoming webhook URL", Kind: "url", Secret: true, Required: true},
	},
	send: func(ctx context.Context, s map[string]string, m Message) error {
		return postJSON(ctx, s["url"], map[string]string{"text": truncate(m.Body, 3000)}, nil)
	},
}

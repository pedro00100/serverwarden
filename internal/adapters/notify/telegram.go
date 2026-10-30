package notify

import (
	"context"
	"net/url"
)

var telegramType = &Type{
	ID: "telegram", Name: "Telegram",
	Fields: []Field{
		{Key: "token", Label: "Bot token", Kind: "text", Secret: true, Required: true, Help: "Create a bot with @BotFather."},
		{Key: "chat_id", Label: "Chat ID", Kind: "text", Required: true, Help: "Your user, group or channel ID."},
	},
	send: func(ctx context.Context, s map[string]string, m Message) error {
		endpoint := "https://api.telegram.org/bot" + url.PathEscape(s["token"]) + "/sendMessage"
		return postJSON(ctx, endpoint, map[string]any{
			"chat_id": s["chat_id"], "text": truncate(m.Body, 4096), "disable_web_page_preview": true,
		}, nil)
	},
}

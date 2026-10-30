package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
)

func TestRender(t *testing.T) {
	e := domain.NewEvent("health", "warning", "finding.disk_usage", domain.Args{"pct": 91})
	e.Server = "nas"

	m, err := Render("Lab", "es", "", e)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Problema en nas" || m.Body != "⚠️ Problema en nas\nDisco al 91 %" {
		t.Errorf("default template: %q / %q", m.Title, m.Body)
	}

	e.Resolved = true
	m, _ = Render("Lab", "en", "[{{.Level}}] {{.Server}}: {{.Text}}{{if .Resolved}} (ok){{end}}", e)
	if m.Body != "[warning] nas: Disk at 91% (ok)" || m.Title != "Resolved on nas" {
		t.Errorf("custom template: %q / %q", m.Body, m.Title)
	}

	if _, err := Render("Lab", "en", "{{.Nope}}", e); err == nil {
		t.Error("an unknown template field must fail")
	}
}

func TestAccepts(t *testing.T) {
	ch := config.Channel{Enabled: true, MinLevel: "warning", Events: []string{"health"}}
	cases := []struct {
		kind, level string
		want        bool
	}{
		{"health", "warning", true},
		{"health", "alert", true},
		{"health", "info", false},
		{"login", "alert", false},
	}
	for _, c := range cases {
		if got := Accepts(ch, domain.Event{Kind: c.kind, Level: c.level}); got != c.want {
			t.Errorf("%s/%s = %v", c.kind, c.level, got)
		}
	}
	ch.Enabled = false
	if Accepts(ch, domain.Event{Kind: "health", Level: "alert"}) {
		t.Error("a disabled channel must accept nothing")
	}
}

func TestValidate(t *testing.T) {
	ok := config.Channel{Name: "n", Type: "ntfy", Settings: map[string]string{"topic": "lab"}}
	if err := Validate(ok); err != nil {
		t.Errorf("ntfy with default server: %v", err)
	}
	cases := map[string]config.Channel{
		"unknown type":  {Name: "x", Type: "pager"},
		"missing field": {Name: "x", Type: "telegram", Settings: map[string]string{"token": "t"}},
		"bad url":       {Name: "x", Type: "discord", Settings: map[string]string{"url": "discord.com/hook"}},
		"bad number":    {Name: "x", Type: "email", Settings: map[string]string{"host": "h", "port": "abc", "from": "a", "to": "b"}},
		"bad option":    {Name: "x", Type: "email", Settings: map[string]string{"host": "h", "security": "ssl3", "from": "a", "to": "b"}},
		"unknown key":   {Name: "x", Type: "slack", Settings: map[string]string{"url": "https://h/x", "color": "red"}},
	}
	for name, ch := range cases {
		if err := Validate(ch); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestMaskAndKeepSecrets(t *testing.T) {
	stored := config.Channel{ID: "tg", Type: "telegram", Settings: map[string]string{"token": "123:secret", "chat_id": "42"}}
	masked := Mask(stored)
	if masked.Settings["token"] != Masked || masked.Settings["chat_id"] != "42" {
		t.Fatalf("mask: %v", masked.Settings)
	}
	if stored.Settings["token"] != "123:secret" {
		t.Fatal("Mask must not modify the original")
	}
	back := KeepSecrets(masked, []config.Channel{stored})
	if back.Settings["token"] != "123:secret" {
		t.Errorf("keep: %v", back.Settings)
	}
	stranger := KeepSecrets(config.Channel{ID: "new", Type: "telegram", Settings: map[string]string{"token": Masked}}, []config.Channel{stored})
	if stranger.Settings["token"] != "" {
		t.Error("a masked value must never copy another channel's secret")
	}
}

func capture(t *testing.T) (*httptest.Server, func() (*http.Request, map[string]any)) {
	var mu sync.Mutex
	var req *http.Request
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		req, body = r, map[string]any{}
		_ = json.Unmarshal(b, &body)
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)
	return srv, func() (*http.Request, map[string]any) { mu.Lock(); defer mu.Unlock(); return req, body }
}

func dispatcher(channels ...config.Channel) *Dispatcher {
	c := &config.Config{General: config.General{Title: "Lab", Language: "en"}}
	c.Notifications.Channels = channels
	return NewDispatcher(func() *config.Config { return c }, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestProviders(t *testing.T) {
	srv, last := capture(t)
	cases := []struct {
		ch    config.Channel
		check func(r *http.Request, b map[string]any) bool
	}{
		{config.Channel{Name: "d", Type: "discord", Settings: map[string]string{"url": srv.URL}},
			func(_ *http.Request, b map[string]any) bool {
				return strings.Contains(b["content"].(string), "Test notification")
			}},
		{config.Channel{Name: "n", Type: "ntfy", Settings: map[string]string{"server": srv.URL, "topic": "lab", "token": "tk"}},
			func(r *http.Request, b map[string]any) bool {
				return b["topic"] == "lab" && r.Header.Get("Authorization") == "Bearer tk"
			}},
		{config.Channel{Name: "g", Type: "gotify", Settings: map[string]string{"server": srv.URL, "token": "gk"}},
			func(r *http.Request, b map[string]any) bool {
				return r.URL.Path == "/message" && r.Header.Get("X-Gotify-Key") == "gk"
			}},
		{config.Channel{Name: "w", Type: "webhook", Settings: map[string]string{"url": srv.URL, "header": "Authorization", "header_value": "Bearer x"}},
			func(r *http.Request, b map[string]any) bool {
				return b["kind"] == "test" && r.Header.Get("Authorization") == "Bearer x"
			}},
	}
	for _, c := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := dispatcher().Test(ctx, c.ch); err != nil {
			t.Errorf("%s: %v", c.ch.Type, err)
		} else if r, b := last(); !c.check(r, b) {
			t.Errorf("%s: unexpected request %v %v", c.ch.Type, r.Header, b)
		}
		cancel()
	}
}

func TestFailureIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid webhook token", http.StatusUnauthorized)
	}))
	defer srv.Close()
	err := dispatcher().Test(context.Background(), config.Channel{Name: "d", Type: "discord", Settings: map[string]string{"url": srv.URL}})
	if err == nil || !strings.Contains(err.Error(), "invalid webhook token") {
		t.Errorf("err = %v", err)
	}
}

func TestErrorsNeverContainSecrets(t *testing.T) {
	secret := "https://127.0.0.1:1/api/webhooks/123/very-secret-token"
	err := dispatcher().Test(context.Background(), config.Channel{Name: "d", Type: "discord", Settings: map[string]string{"url": secret}})
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(err.Error(), "very-secret-token") {
		t.Errorf("the error leaks the secret: %v", err)
	}
	err = redact(errors.New(`Post "https://api.telegram.org/bot123:ABC%2Fdef/sendMessage": timeout`), []string{"123:ABC/def"})
	if strings.Contains(err.Error(), "ABC") {
		t.Errorf("escaped secrets must be removed too: %v", err)
	}
}

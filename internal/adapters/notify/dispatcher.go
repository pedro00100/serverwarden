package notify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/i18n"
)

const DefaultTemplate = "{{.Icon}} {{.Title}}\n{{.Text}}"

// TemplateData is what a channel template can use.
type TemplateData struct {
	Panel, Kind, Level, Server, Title, Text, Icon string
	Resolved, Repeat                              bool
	Time                                          time.Time
}

// Dispatcher implements ports.Notifier: every event goes to the log and to each
// enabled channel whose filters accept it.
type Dispatcher struct {
	settings func() *config.Config
	log      *slog.Logger
}

func NewDispatcher(settings func() *config.Config, log *slog.Logger) *Dispatcher {
	return &Dispatcher{settings: settings, log: log}
}

func (d *Dispatcher) Notify(e domain.Event) {
	c := d.settings()
	if c == nil {
		d.log.Info("notification", "kind", e.Kind, "level", e.Level, "key", e.Key)
		return
	}
	d.log.Info("notification", "kind", e.Kind, "level", e.Level, "text", i18n.T("en", e.Key, e.Args))
	for _, ch := range c.Notifications.Channels {
		if !Accepts(ch, e) {
			continue
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := d.send(ctx, c, ch, e); err != nil {
				d.log.Warn("notification failed", "channel", ch.Name, "error", err)
			}
		}()
	}
}

// Test sends a test event through one channel and waits for the result.
func (d *Dispatcher) Test(ctx context.Context, ch config.Channel) error {
	c := d.settings()
	if c == nil {
		return errors.New("settings not loaded")
	}
	if err := Validate(ch); err != nil {
		return err
	}
	e := domain.NewEvent("test", "info", "test.message", domain.Args{"panel": c.General.Title})
	return d.send(ctx, c, ch, e)
}

func Accepts(ch config.Channel, e domain.Event) bool {
	return ch.Enabled &&
		domain.LevelRank(e.Level) >= domain.LevelRank(ch.MinLevel) &&
		(len(ch.Events) == 0 || slices.Contains(ch.Events, e.Kind))
}

func (d *Dispatcher) send(ctx context.Context, c *config.Config, ch config.Channel, e domain.Event) error {
	t := lookup(ch.Type)
	if t == nil {
		return fmt.Errorf("unknown channel type %q", ch.Type)
	}
	m, err := Render(c.General.Title, c.General.Language, ch.Template, e)
	if err != nil {
		return err
	}
	settings := map[string]string{}
	var secrets []string
	for _, f := range t.Fields {
		settings[f.Key] = value(ch.Settings, f)
		if f.Secret {
			secrets = append(secrets, settings[f.Key])
		}
	}
	return redact(t.send(ctx, settings, m), secrets)
}

// redact removes secret values from an error before it reaches a log or the UI:
// HTTP client errors quote the full URL, and for Discord or Telegram the URL is the secret.
func redact(err error, secrets []string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, s := range secrets {
		if len(s) >= 4 {
			msg = strings.ReplaceAll(msg, s, Masked)
			msg = strings.ReplaceAll(msg, url.PathEscape(s), Masked)
		}
	}
	return errors.New(msg)
}

// Render builds the message for an event in the given language and template.
func Render(panel, lang, tmpl string, e domain.Event) (Message, error) {
	data := TemplateData{
		Panel: panel, Kind: e.Kind, Level: e.Level, Server: e.Server,
		Text: i18n.T(lang, e.Key, e.Args), Resolved: e.Resolved, Repeat: e.Repeat, Time: e.Time,
	}
	switch {
	case e.Kind == "health" && e.Resolved:
		data.Title = i18n.T(lang, "alert.resolved", domain.Args{"server": e.Server})
	case e.Kind == "health" && e.Repeat:
		data.Title = i18n.T(lang, "alert.repeat", domain.Args{"server": e.Server})
	case e.Kind == "health":
		data.Title = i18n.T(lang, "alert.firing", domain.Args{"server": e.Server})
	default:
		data.Title = panel
	}
	data.Icon = icon(e)
	if tmpl == "" {
		tmpl = DefaultTemplate
	}
	t, err := template.New("message").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return Message{}, err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return Message{}, err
	}
	return Message{Panel: panel, Title: data.Title, Body: b.String(), Event: e}, nil
}

func icon(e domain.Event) string {
	switch {
	case e.Kind == "test":
		return "🧪"
	case e.Resolved:
		return "✅"
	case e.Level == "alert":
		return "🚨"
	case e.Level == "warning":
		return "⚠️"
	}
	return "ℹ️"
}

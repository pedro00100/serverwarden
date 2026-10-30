// Package notify sends events to the configured channels (Discord, Telegram,
// ntfy…). Each channel type declares its fields, so the UI can draw its form
// and adding a provider only takes a new file here.
package notify

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
)

// Masked replaces secrets when settings are sent to the UI. Saving it back
// keeps the stored value.
const Masked = "••••••••"

type Message struct {
	Panel string
	Title string
	Body  string
	Event domain.Event
}

type sender func(ctx context.Context, s map[string]string, m Message) error

type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"` // text | url | number | select
	Secret   bool     `json:"secret,omitempty"`
	Required bool     `json:"required"`
	Default  string   `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"`
	Help     string   `json:"help,omitempty"`
}

type Type struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
	send   sender
}

var types = []*Type{discordType, telegramType, ntfyType, slackType, gotifyType, emailType, webhookType}

func Types() []*Type { return types }

func lookup(id string) *Type {
	for _, t := range types {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// value returns the setting or the field's default.
func value(s map[string]string, f Field) string {
	if v := s[f.Key]; v != "" {
		return v
	}
	return f.Default
}

func Validate(ch config.Channel) error {
	t := lookup(ch.Type)
	if t == nil {
		return fmt.Errorf("unknown channel type %q", ch.Type)
	}
	var errs []error
	for _, f := range t.Fields {
		v := value(ch.Settings, f)
		if v == "" {
			if f.Required {
				errs = append(errs, fmt.Errorf("%s: %s is required", ch.Name, f.Label))
			}
			continue
		}
		switch f.Kind {
		case "url":
			if u, err := url.Parse(v); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				errs = append(errs, fmt.Errorf("%s: %s must be an http(s) URL", ch.Name, f.Label))
			}
		case "number":
			if _, err := strconv.Atoi(v); err != nil {
				errs = append(errs, fmt.Errorf("%s: %s must be a number", ch.Name, f.Label))
			}
		case "select":
			if !slices.Contains(f.Options, v) {
				errs = append(errs, fmt.Errorf("%s: %s must be one of %v", ch.Name, f.Label, f.Options))
			}
		}
	}
	for k := range ch.Settings {
		if !slices.ContainsFunc(t.Fields, func(f Field) bool { return f.Key == k }) {
			errs = append(errs, fmt.Errorf("%s: unknown setting %q", ch.Name, k))
		}
	}
	return errors.Join(errs...)
}

// IsSecret tells the settings store which fields to keep sealed on disk.
func IsSecret(channelType, key string) bool { return isSecret(lookup(channelType), key) }

func isSecret(t *Type, key string) bool {
	return t != nil && slices.ContainsFunc(t.Fields, func(f Field) bool { return f.Key == key && f.Secret })
}

// Mask hides every secret value of the channel.
func Mask(ch config.Channel) config.Channel {
	t := lookup(ch.Type)
	out := ch
	out.Settings = map[string]string{}
	for k, v := range ch.Settings {
		if isSecret(t, k) && v != "" {
			v = Masked
		}
		out.Settings[k] = v
	}
	return out
}

// KeepSecrets restores the stored secrets that came back masked from the UI.
func KeepSecrets(ch config.Channel, stored []config.Channel) config.Channel {
	t := lookup(ch.Type)
	i := slices.IndexFunc(stored, func(s config.Channel) bool { return s.ID == ch.ID && s.Type == ch.Type })
	for k, v := range ch.Settings {
		if v != Masked || !isSecret(t, k) {
			continue
		}
		if i >= 0 {
			ch.Settings[k] = stored[i].Settings[k]
		} else {
			ch.Settings[k] = ""
		}
	}
	return ch
}

// Registry implements ports.Channels.
type Registry struct{ *Dispatcher }

func (Registry) Validate(ch config.Channel) error      { return Validate(ch) }
func (Registry) Mask(ch config.Channel) config.Channel { return Mask(ch) }
func (Registry) KeepSecrets(ch config.Channel, stored []config.Channel) config.Channel {
	return KeepSecrets(ch, stored)
}

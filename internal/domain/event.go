package domain

import "time"

// Event is something worth notifying. Key and Args identify the message so it
// can be rendered in the configured language.
type Event struct {
	Kind     string    `json:"kind"`  // login | action | health | cve | settings | test
	Level    string    `json:"level"` // info | warning | alert
	Server   string    `json:"server,omitempty"`
	Key      string    `json:"key"`
	Args     Args      `json:"args,omitempty"`
	Resolved bool      `json:"resolved,omitempty"`
	Repeat   bool      `json:"repeat,omitempty"`
	Time     time.Time `json:"time"`
}

func NewEvent(kind, level, key string, args Args) Event {
	return Event{Kind: kind, Level: level, Key: key, Args: args, Time: time.Now()}
}

// LevelRank orders notification levels from least to most severe.
func LevelRank(l string) int {
	switch l {
	case "warning":
		return 1
	case "alert":
		return 2
	}
	return 0
}

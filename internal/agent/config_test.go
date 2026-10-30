package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	c, err := parseConfig([]byte(`{
	  "actions": {"check-updates": {"name": "Check updates", "command": ["/usr/bin/systemctl", "start", "apt-daily.service"]}},
	  "backups": [{"name": "edge", "repository": "/srv/backups/edge", "password_file": "/root/edge.pass"}],
	  "notify": ["/usr/local/bin/notify", "{level}", "{text}"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Actions["check-updates"].Command[0] != "/usr/bin/systemctl" || len(c.Backups) != 1 {
		t.Errorf("parsed: %+v", c)
	}
}

func TestExamples(t *testing.T) {
	files, _ := filepath.Glob("../../examples/*/agent-*.json")
	if len(files) == 0 {
		t.Fatal("no example agent configurations found")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseConfig(b); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := map[string]struct{ json, want string }{
		"relative command": {`{"actions": {"x": {"name": "X", "command": ["systemctl"]}}}`, "absolute path"},
		"bad id":           {`{"actions": {"Do It": {"name": "X", "command": ["/bin/true"]}}}`, "id must be"},
		"no name":          {`{"actions": {"x": {"command": ["/bin/true"]}}}`, "name is required"},
		"backup":           {`{"backups": [{"name": "b", "repository": "rel", "password_file": "/p"}]}`, "backups[0]"},
		"unknown field":    {`{"action": {}}`, "unknown field"},
		"ssh-key no id":    {`{"action_auth": "ssh-key"}`, "needs server"},
		"bad action auth":  {`{"action_auth": "pin"}`, "action_auth"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseConfig([]byte(c.json))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v; want %q", err, c.want)
			}
		})
	}
}

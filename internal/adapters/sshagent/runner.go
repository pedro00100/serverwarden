package sshagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/pedro00100/serverwarden/internal/domain"
)

// Runner uses a key separate from the monitoring one and its own connection
// for every action.
type Runner struct{ targets *Targets }

func NewRunner(targets *Targets) *Runner { return &Runner{targets: targets} }

// Run sends the request and its signature on the agent's standard input.
func (r *Runner) Run(ctx context.Context, server string, req domain.ActionRequest, assertion []byte) (domain.ActionResult, error) {
	var res domain.ActionResult
	t, ok := r.targets.Get(server)
	if !ok || t.ActionKey == "" {
		return res, fmt.Errorf("no action key for %q", server)
	}
	body := map[string]any{"request": req}
	if assertion != nil {
		body["assertion"] = json.RawMessage(assertion)
	}
	in, err := json.Marshal(body)
	if err != nil {
		return res, err
	}
	c, err := dial(ctx, t, t.ActionKey)
	if err != nil {
		return res, fmt.Errorf("%s: %w", server, err)
	}
	defer c.Close()
	out, err := run(ctx, c, "action", bytes.NewReader(in))
	if err != nil {
		return res, fmt.Errorf("%s: %w", server, err)
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return res, fmt.Errorf("%s: invalid response: %w", server, err)
	}
	return res, nil
}

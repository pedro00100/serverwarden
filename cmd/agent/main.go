// Command agent prints the local server state as JSON. The panel runs it over
// SSH with a key restricted to this command; the only client input it reads is
// the mode from SSH_ORIGINAL_COMMAND: empty, "security", "packages", "images"
// or "image <id>". The last one writes an image archive instead of JSON, and
// only if this server's config allows it.
//
// "action" mode is only enabled by argument, i.e. by the forced command of the
// separate action key (command="… action"), so the monitoring key cannot reach it.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pedro00100/serverwarden/internal/agent"
	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/domain"
)

func main() {
	cfg, cfgErr := agent.LoadConfig(brand.AgentConfig)
	if len(os.Args) > 1 && os.Args[1] == "action" {
		if cfgErr != nil {
			emit(domain.ActionResult{Message: "Rejected by the server: " + cfgErr.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		emit(agent.RunAction(ctx, os.Stdin, cfg))
		return
	}
	// Monitoring keeps working with a broken config; the error is reported instead.
	if cfgErr != nil {
		cfg = &agent.Config{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	args := strings.Fields(os.Getenv("SSH_ORIGINAL_COMMAND"))
	if len(args) == 0 {
		args = os.Args[1:]
	}
	mode := ""
	if len(args) > 0 {
		mode = args[0]
	}
	switch mode {
	case "security":
		emit(agent.CollectSecurity(ctx, cfg))
	case "packages":
		emit(agent.CollectPackages())
	case "images":
		images, err := agent.Images(ctx)
		if err != nil {
			fail(err)
		}
		emit(images)
	case "image":
		if len(args) != 2 {
			fail(errors.New("usage: image <id>"))
		}
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := agent.ExportImage(ctx, cfg, args[1], os.Stdout); err != nil {
			fail(err)
		}
	default:
		s := agent.Collect(ctx, cfg)
		if cfgErr != nil {
			s.ConfigError = cfgErr.Error()
		}
		emit(s)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func emit(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		os.Exit(1)
	}
}

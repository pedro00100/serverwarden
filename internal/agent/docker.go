package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
)

const dockerSocket = "/var/run/docker.sock"

var healthRe = regexp.MustCompile(`\((healthy|unhealthy|health: starting)\)`)

func dockerClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(c, "unix", dockerSocket)
		}},
	}
}

func containers(ctx context.Context) []domain.Container {
	client := dockerClient()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json?all=1", nil)
	resp, err := client.Do(req)
	if err != nil {
		return []domain.Container{}
	}
	defer resp.Body.Close()
	var list []struct {
		Names  []string
		Image  string
		State  string
		Status string
	}
	if json.NewDecoder(resp.Body).Decode(&list) != nil {
		return []domain.Container{}
	}
	out := []domain.Container{}
	for _, c := range list {
		x := domain.Container{Image: c.Image, State: c.State, Status: c.Status}
		if len(c.Names) > 0 {
			x.Name = strings.TrimPrefix(c.Names[0], "/")
		}
		if m := healthRe.FindStringSubmatch(c.Status); m != nil {
			x.Health = strings.TrimPrefix(m[1], "health: ")
		}
		if i := strings.Index(x.Image, "@sha256:"); i > 0 {
			x.Image = x.Image[:i]
		}
		if x.State == "running" && strings.Contains(strings.ToLower(x.Image), "vault") && nameRe.MatchString(x.Name) {
			x.Vault = vaultStatus(ctx, client, x.Name)
		}
		out = append(out, x)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

var (
	nameRe     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	vaultEnvRe = regexp.MustCompile(`VAULT_[A-Z_]+=[A-Za-z0-9_./:-]+`)
)

// vaultStatus runs the same "vault status" as the container's healthcheck, with
// its VAULT_* variables (mutual TLS certificates, address). No token, read-only.
func vaultStatus(ctx context.Context, client *http.Client, name string) *domain.VaultStatus {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+name+"/json", nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var info struct {
		Config struct{ Healthcheck *struct{ Test []string } }
	}
	if json.NewDecoder(resp.Body).Decode(&info) != nil {
		return nil
	}
	args := []string{"exec", name, "env"}
	if hc := info.Config.Healthcheck; hc != nil {
		args = append(args, vaultEnvRe.FindAllString(strings.Join(hc.Test, " "), -1)...)
	}
	args = append(args, "vault", "status", "-format=json")
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	raw, _ := exec.CommandContext(c, docker, args...).Output() // exits 2 when sealed
	var st struct {
		Initialized bool   `json:"initialized"`
		Sealed      bool   `json:"sealed"`
		Version     string `json:"version"`
		Progress    int    `json:"progress"`
		T           int    `json:"t"`
	}
	if json.Unmarshal(raw, &st) != nil {
		return nil
	}
	return &domain.VaultStatus{Initialized: st.Initialized, Sealed: st.Sealed, Version: st.Version, Progress: st.Progress, Threshold: st.T}
}

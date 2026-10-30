package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/pedro00100/serverwarden/internal/domain"
)

var imageIDRe = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// Images lists the images of running containers, with what the scanner needs
// to scan exactly the image in use: its registry digests and platform.
func Images(ctx context.Context) ([]domain.Image, error) {
	client := dockerClient()
	var list []struct{ Image, ImageID string }
	if err := dockerGet(ctx, client, "/containers/json", &list); err != nil {
		return nil, err
	}
	out := []domain.Image{}
	seen := map[string]bool{}
	for _, c := range list {
		if !imageIDRe.MatchString(c.ImageID) || seen[c.ImageID] {
			continue
		}
		seen[c.ImageID] = true
		var info struct {
			RepoDigests               []string
			Os, Architecture, Variant string
		}
		if err := dockerGet(ctx, client, "/images/"+c.ImageID+"/json", &info); err != nil {
			return nil, err
		}
		ref := c.Image
		if i := strings.Index(ref, "@sha256:"); i > 0 {
			ref = ref[:i]
		}
		img := domain.Image{Ref: ref, ID: c.ImageID, Digests: info.RepoDigests, Platform: platform(info.Os, info.Architecture, info.Variant)}
		if img.Digests == nil {
			img.Digests = []string{}
		}
		// The digest of the repository the container names goes first.
		repo := repository(ref)
		sort.SliceStable(img.Digests, func(a, b int) bool {
			return repository(img.Digests[a]) == repo && repository(img.Digests[b]) != repo
		})
		out = append(out, img)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Ref < out[b].Ref })
	return out, nil
}

// ExportImage writes the image (as `docker save` would) to w. Only allowed
// with image_export, and only for an image a running container uses.
func ExportImage(ctx context.Context, cfg *Config, id string, w io.Writer) error {
	if !cfg.ImageExport {
		return errors.New("this server does not export images (image_export is off)")
	}
	if !imageIDRe.MatchString(id) {
		return errors.New("malformed image id")
	}
	images, err := Images(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(images, func(x domain.Image) bool { return x.ID == id })
	if i < 0 {
		return errors.New("no running container uses that image")
	}
	q := url.Values{}
	if p := images[i].Platform; p != "" {
		f := strings.SplitN(p, "/", 3)
		v := map[string]string{"os": f[0], "architecture": f[1]}
		if len(f) == 3 {
			v["variant"] = f[2]
		}
		b, _ := json.Marshal(v)
		q.Set("platform", string(b))
	}
	client := dockerClient()
	client.Timeout = 0 // large images take minutes; ctx bounds it
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/images/"+id+"/get?"+q.Encode(), nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("docker: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

func dockerGet(ctx context.Context, client *http.Client, path string, dst any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker: %s %s", path, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(dst)
}

func platform(os, arch, variant string) string {
	if os == "" || arch == "" {
		return ""
	}
	if variant != "" {
		return os + "/" + arch + "/" + variant
	}
	return os + "/" + arch
}

// repository strips the tag or digest: "ghcr.io/a/b:1" and "ghcr.io/a/b@sha256:…" give "ghcr.io/a/b".
func repository(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	return ref
}

package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLatest(t *testing.T) {
	cases := []struct {
		status     int
		body, want string
		wantErr    bool
	}{
		{200, `{"tag_name":"v0.8.1","body":"notes"}`, "0.8.1", false},
		{200, `{"tag_name":"0.8.1"}`, "", true},
		{200, `{"tag_name":"v0.8.1-rc.1"}`, "", true},
		{200, `{"tag_name":"v0.8.1\"><script>"}`, "", true},
		{200, `{"tag_name":"v` + strings.Repeat("9", 2<<20) + `"}`, "", true},
		{200, `not json`, "", true},
		{404, `{"message":"Not Found"}`, "", true},
	}
	for _, c := range cases {
		var ua, auth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ua, auth = r.UserAgent(), r.Header.Get("Authorization")
			w.WriteHeader(c.status)
			w.Write([]byte(c.body))
		}))
		g := &GitHub{URL: srv.URL, Client: srv.Client()}
		got, err := g.Latest(context.Background())
		srv.Close()
		if got != c.want || (err != nil) != c.wantErr {
			t.Errorf("%d %.40q: got %q, %v", c.status, c.body, got, err)
		}
		if ua != "serverwarden" || auth != "" {
			t.Errorf("the request must not identify the panel: user agent %q, authorization %q", ua, auth)
		}
	}
}

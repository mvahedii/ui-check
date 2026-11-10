package figma

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseLink(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{"https://www.figma.com/design/Ab12/Frame?node-id=42-11", "42:11"},
		{"https://figma.com/file/Ab12?node-id=42%3A11", "42:11"},
		{"https://figma.com/design/Ab12", ""},
	} {
		got, err := ParseLink(tc.url)
		if err != nil || got != (Link{"Ab12", tc.want}) {
			t.Errorf("ParseLink(%q) = %+v, %v", tc.url, got, err)
		}
	}
	for _, raw := range []string{
		"http://figma.com/design/Ab12",
		"https://figma.com.evil.test/design/Ab12",
		"https://figma.com@evil.test/design/Ab12",
		"https://figma.com/design/Ab12?node-id=%xx",
		"https://figma.com/design/Ab12?node-id=1-2&node-id=3-4",
	} {
		if _, err := ParseLink(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestLoadNode(t *testing.T) {
	for _, status := range []int{200, 302, 401, 403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/files/Ab12/nodes" ||
					r.URL.Query().Get("ids") != "42:11" || r.Header.Get("X-Figma-Token") != "mock-token" {
					t.Error("unexpected Figma request")
				}
				if status == 302 {
					w.Header().Set("Location", "/redirected")
				}
				w.Header().Set("Retry-After", "20")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"nodes":{"42:11":{"document":{"id":"42:11","type":"FRAME","absoluteBoundingBox":{"x":10,"y":20,"width":390,"height":844}}}}}`)
			}))
			defer server.Close()
			client := NewClient()
			client.base = server.URL
			node, err := client.LoadNode(t.Context(), Link{"Ab12", "42:11"}, "mock-token")
			if status == 200 {
				if err != nil || node.ID != "42:11" || node.Box == nil || node.Box.Width != 390 {
					t.Fatalf("unexpected node %+v, %v", node, err)
				}
				return
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != status || apiErr.RetryAfter != "20" {
				t.Fatalf("unexpected API error %v", err)
			}
		})
	}
}

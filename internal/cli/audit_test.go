package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ui-check/internal/audit"
	"ui-check/internal/demo"
)

func TestAuditChrome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(demo.Handler))
	defer server.Close()
	design := filepath.Join("..", "..", "testdata", "designs", "signup.json")

	for _, tc := range []struct {
		name, route, failOn string
		status              audit.Status
		code                int
		actual              float64
		findings            int
	}{
		{"mismatch", "/signup", "minor", audit.Fail, 1, 14, 1},
		{"equal", "/same", "minor", audit.Pass, 0, 12, 0},
		{"wrong-route", "/wrong", "minor", audit.Inconclusive, 3, 0, 0},
		{"ambiguous", "/ambiguous", "minor", audit.Inconclusive, 3, 0, 0},
		{"measures-geometry-not-padding", "/margin", "minor", audit.Fail, 1, 14, 1},
		{"major-policy", "/signup", "major", audit.Pass, 0, 14, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			if base := os.Getenv("UICHECK_E2E_OUT"); base != "" {
				out = filepath.Join(base, tc.name)
			}
			var stdout, stderr bytes.Buffer
			code := Run(t.Context(), []string{
				"audit", "--design-snapshot", design, "--target", server.URL,
				"--route", tc.route, "--out", out, "--no-ai", "--fail-on", tc.failOn,
			}, &stdout, &stderr)
			if code != tc.code || stderr.Len() != 0 {
				t.Fatalf("exit %d (want %d), stdout=%q stderr=%q", code, tc.code, &stdout, &stderr)
			}
			status, dir, ok := strings.Cut(strings.TrimSpace(stdout.String()), " · ")
			if !ok || status != string(tc.status) || filepath.Dir(dir) != out {
				t.Fatalf("unexpected stdout: %q", &stdout)
			}
			t.Logf("exit=%d %s", code, strings.TrimSpace(stdout.String()))
			var report audit.Report
			if err := json.Unmarshal(readArtifact(t, dir, "report.json"), &report); err != nil {
				t.Fatal(err)
			}
			if report.Status != tc.status || report.SchemaVersion != 1 || report.Width != 390 ||
				report.Height != 844 || report.DPR != 1 || len(report.Findings) != tc.findings {
				t.Fatalf("unexpected report: %+v", report)
			}
			html := readArtifact(t, dir, "index.html")
			if !bytes.Contains(html, []byte("UI Check · "+tc.status)) {
				t.Fatal("HTML disagrees with JSON status")
			}
			for _, name := range []string{"assets/target.png", "evidence/signup.png", "evidence/design.json", "evidence/dom.json"} {
				if len(readArtifact(t, dir, name)) == 0 {
					t.Fatalf("empty artifact %s", name)
				}
			}
			if tc.status == audit.Inconclusive {
				if report.Coverage != 0 || len(report.Unverified) != 1 || report.Measurement != nil || report.Confidence != nil {
					t.Fatalf("ambiguous result should abstain: %+v", report)
				}
				return
			}
			m := report.Measurement
			if report.Coverage != 1 || m == nil || m.Expected != 12 || m.Actual != tc.actual ||
				m.Delta != tc.actual-12 || m.Unit != "css-px" || m.Verification != "verified" || len(m.Evidence) != 4 {
				t.Fatalf("bad measurement: %+v", m)
			}
			for _, ref := range m.Evidence {
				if !bytes.Contains(readArtifact(t, dir, ref.Artifact), []byte(`"id": "`+ref.ID+`"`)) {
					t.Fatalf("unresolved evidence: %+v", ref)
				}
			}
		})
	}
}

func TestAuditOperationalError(t *testing.T) {
	for _, tc := range []struct {
		target, route string
	}{
		{"https://example.com", ""},
		{"http://127.0.0.1:8042", "//example.com"},
		{"http://127.0.0.1:8042", "/signup?token=secret"},
		{"http://127.0.0.1:8042", ""},
	} {
		var stdout, stderr bytes.Buffer
		code := Run(t.Context(), []string{
			"audit", "--design-snapshot", "missing-snapshot.json", "--target", tc.target, "--route", tc.route,
		}, &stdout, &stderr)
		if code != 2 || stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), "ERROR:") || strings.Contains(stderr.String(), "token=secret") {
			t.Fatalf("exit %d stdout=%q stderr=%q", code, &stdout, &stderr)
		}
	}
}

func readArtifact(t *testing.T, dir, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

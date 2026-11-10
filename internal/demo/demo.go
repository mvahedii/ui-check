package demo

import (
	"fmt"
	"net/http"
)

// Handler serves the same plain fixture for the spike and the CLI E2E tests.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	inset, childMargin := 14, 0
	switch r.URL.Path {
	case "/wrong":
		fmt.Fprint(w, "<!doctype html><body><p>Unrelated page</p>")
		return
	case "/same":
		inset = 12
	case "/margin":
		inset, childMargin = 12, 2
	case "/signup", "/ambiguous":
	default:
		http.NotFound(w, r)
		return
	}
	fmt.Fprintf(w, `<!doctype html><style>
body { margin: 0; font: 16px sans-serif }
section { padding: %dpx 10px 10px; border-top: 3px solid #445 }
button { display: block; margin-top: %dpx }
</style><section data-testid="signup-section"><button data-testid="signup-button">Sign up</button>`, inset, childMargin)
	if r.URL.Path == "/ambiguous" {
		fmt.Fprint(w, `<button data-testid="signup-button">Other</button>`)
	}
	fmt.Fprint(w, "</section>")
}

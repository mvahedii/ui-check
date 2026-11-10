package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ui-check/internal/demo"
)

func TestCapture(t *testing.T) {
	path, err := Find("")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(demo.Handler))
	defer server.Close()
	target, err := LocalURL(server.URL + "/signup")
	if err != nil {
		t.Fatal(err)
	}
	scene, err := Capture(t.Context(), path, target, 390, 844)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(scene.PNG))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 390 || img.Bounds().Dy() != 844 || scene.DPR != 1 || len(scene.Elements) != 2 {
		t.Fatalf("unexpected capture %v, %d elements, DPR %g", img.Bounds(), len(scene.Elements), scene.DPR)
	}
	parent, child := scene.Elements[0], scene.Elements[1]
	if parent.PaddingTop != 14 || parent.BorderTop != 3 || child.Y-parent.Y-parent.BorderTop != 14 {
		t.Fatalf("unexpected browser geometry: %+v %+v", parent, child)
	}
	t.Logf("%s viewport=%dx%d DPR=%g parentY=%g borderTop=%g childY=%g", scene.Version, scene.Width, scene.Height, scene.DPR, parent.Y, parent.BorderTop, child.Y)
	if dir := os.Getenv("UICHECK_SPIKE_OUT"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		dom, err := json.MarshalIndent(scene, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		for name, contents := range map[string][]byte{"dom.json": dom, "capture.png": scene.PNG} {
			if err := os.WriteFile(filepath.Join(dir, name), contents, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestCaptureBlocksCrossOriginRedirect(t *testing.T) {
	path, err := Find("")
	if err != nil {
		t.Fatal(err)
	}
	reached := make(chan struct{}, 1)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case reached <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/unexpected", http.StatusFound)
	}))
	defer server.Close()
	target, err := LocalURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Capture(t.Context(), path, target, 390, 844); err == nil {
		t.Fatal("accepted cross-origin redirect")
	}
	select {
	case <-reached:
		t.Fatal("redirect destination was contacted")
	default:
	}
}

func TestCaptureCancellation(t *testing.T) {
	path, err := Find("")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	target, err := LocalURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Capture(ctx, path, target, 390, 844)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("capture did not return after cancellation")
	}
}

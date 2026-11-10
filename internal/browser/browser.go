package browser

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

//go:embed dom.js
var extractDOM string

type Element struct {
	ID                string  `json:"id"`
	ParentID          string  `json:"parentId"`
	Visible           bool    `json:"visible"`
	X                 float64 `json:"x"`
	Y                 float64 `json:"y"`
	Width             float64 `json:"width"`
	Height            float64 `json:"height"`
	BorderTop         float64 `json:"borderTop"`
	PaddingTop        float64 `json:"paddingTop"`
	GeometrySupported bool    `json:"geometrySupported"`
}

type Scene struct {
	URL             string    `json:"url"`
	Version         string    `json:"browserVersion"`
	OS              string    `json:"os"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	DPR             float64   `json:"dpr"`
	Elements        []Element `json:"elements"`
	Truncated       bool      `json:"truncated"`
	BlockedRequests bool      `json:"blockedRequests"`
	PNG             []byte    `json:"-"`
}

func Find(path string) (string, error) {
	if path == "" {
		path = os.Getenv("UICHECK_BROWSER_PATH")
	}
	if path != "" {
		found, err := exec.LookPath(path)
		if err != nil {
			return "", errors.New("browser override is not executable")
		}
		return found, nil
	}

	candidates := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Chromium.app/Contents/MacOS/Chromium")
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			if dir := os.Getenv(env); dir != "" {
				candidates = append(candidates, dir+`\Google\Chrome\Application\chrome.exe`)
			}
		}
	}
	for _, candidate := range candidates {
		if found, err := exec.LookPath(candidate); err == nil {
			return found, nil
		}
	}
	return "", errors.New("Chrome/Chromium not found; install it or set --browser-path")
}

// LocalURL keeps the measured proof limited to explicitly selected local pages.
func LocalURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("target must be a loopback HTTP(S) URL without credentials, query or fragment")
	}
	ip := net.ParseIP(u.Hostname())
	if !strings.EqualFold(u.Hostname(), "localhost") && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("this milestone supports only trusted loopback targets")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	u.Host = strings.ToLower(u.Host)
	return u, nil
}

func session(ctx context.Context, path string) (context.Context, func()) {
	opts := append([]chromedp.ExecAllocatorOption(nil), chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path))
	allocator, stopAllocator := chromedp.NewExecAllocator(ctx, opts...)
	browser, stopBrowser := chromedp.NewContext(allocator)
	deadline, stopDeadline := context.WithTimeout(browser, 20*time.Second)
	return deadline, func() {
		stopDeadline()
		stopBrowser()
		stopAllocator()
	}
}

func Doctor(ctx context.Context, path string) (string, error) {
	ctx, stop := session(ctx, path)
	defer stop()
	return chromedp.Run(ctx, func(ctx context.Context, target *chromedp.Target) (string, error) {
		version, err := cdp.Call(ctx, target, cdpbrowser.GetVersion, cdp.Empty{})
		return version.Product, err
	})
}

func Capture(ctx context.Context, path string, target *url.URL, width, height int) (Scene, error) {
	ctx, stop := session(ctx, path)
	defer stop()

	scene, err := chromedp.Run(ctx, func(ctx context.Context, tab *chromedp.Target) (Scene, error) {
		return capture(ctx, tab, target, width, height)
	})
	if err != nil {
		return Scene{}, fmt.Errorf("Chrome capture: %w", err)
	}
	if scene.Width != width || scene.Height != height || scene.DPR != 1 {
		return Scene{}, errors.New("unexpected viewport or DPR")
	}
	return scene, nil
}

func capture(ctx context.Context, tab *chromedp.Target, target *url.URL, width, height int) (Scene, error) {
	ctx, cancel := context.WithCancel(ctx)
	events := cdp.Events(ctx, tab, fetch.RequestPaused)
	done := make(chan struct{})
	var blocked atomic.Bool
	go func() {
		defer close(done)
		for paused, err := range events {
			if err != nil || handleRequest(ctx, tab, target, paused, &blocked) != nil {
				cancel()
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-done
	}()

	version, err := cdp.Call(ctx, tab, cdpbrowser.GetVersion, cdp.Empty{})
	if err != nil {
		return Scene{}, err
	}
	if _, err := cdp.Call(ctx, tab, fetch.Enable, fetch.EnableParams{}); err != nil {
		return Scene{}, err
	}
	if _, err := chromedp.EmulateViewport(int64(width), int64(height), chromedp.EmulateScale(1))(ctx, tab); err != nil {
		return Scene{}, err
	}
	response, err := chromedp.RunResponse(ctx, chromedp.Navigate(target.String()))
	if err != nil {
		return Scene{}, err
	}
	if response == nil || response.Status < 200 || response.Status >= 300 {
		return Scene{}, errors.New("target did not return a successful HTTP response")
	}
	location, err := chromedp.Location()(ctx, tab)
	if err != nil {
		return Scene{}, err
	}
	final, err := LocalURL(location)
	if err != nil || final.Scheme != target.Scheme || final.Host != target.Host || final.EscapedPath() != target.EscapedPath() {
		return Scene{}, errors.New("browser navigated away from the requested loopback route")
	}
	if _, err := chromedp.Evaluate[bool](`document.fonts.ready.then(() => true)`, chromedp.EvalAwaitPromise)(ctx, tab); err != nil {
		return Scene{}, err
	}
	scene, err := chromedp.Evaluate[Scene](extractDOM)(ctx, tab)
	if err != nil {
		return Scene{}, err
	}
	png, err := cdp.Call(ctx, tab, page.CaptureScreenshot, page.CaptureScreenshotParams{})
	if err != nil {
		return Scene{}, err
	}
	scene.Version, scene.URL, scene.OS, scene.PNG = version.Product, location, runtime.GOOS, png.Data
	scene.BlockedRequests = blocked.Load()
	return scene, nil
}

// Intercept page HTTP requests, including redirect hops, before dispatch.
func handleRequest(ctx context.Context, tab *chromedp.Target, target *url.URL, paused fetch.EventRequestPaused, blocked *atomic.Bool) error {
	u, err := url.Parse(paused.Request.URL)
	if err == nil && u.Scheme == target.Scheme && u.Host == target.Host && u.User == nil && paused.Request.Method == "GET" {
		_, err := cdp.Call(ctx, tab, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: paused.RequestID})
		return err
	}
	blocked.Store(true)
	_, err = cdp.Call(ctx, tab, fetch.FailRequest, fetch.FailRequestParams{
		RequestID: paused.RequestID, ErrorReason: network.ErrorReasonBlockedByClient,
	})
	return err
}

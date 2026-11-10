# M0 · Browser and Figma feasibility

Review the [M0 roadmap gate](../../fidelitycheck-docs/05-ROADMAP-FA.md) in the roadmap, [technical design](../../fidelitycheck-docs/03-TRD-FA.md) and [implementation brief](../../fidelitycheck-docs/04-IMPLEMENTATION-PROMPT.md).

## Implementation and decisions

- [CLI entry point](../../cmd/ui-check/main.go) and [doctor](../../internal/cli/cli.go) use Go's standard `flag` package. Browser selection gives precedence to `--browser-path`, the environment override and then discovery.
- [Browser capture](../../internal/browser/browser.go) uses chromedp v0.21.0 with an isolated Chrome process and bounded CDP context. [DOM extraction](../../internal/browser/dom.js) runs inside Chrome. The spike captures a 390 × 844 CSS px viewport at DPR 1, including rendered rectangles and computed border/padding.
- [Figma spike](../../internal/figma/figma.go) parses HTTPS design and legacy file links and calls the node endpoint. The client limits response size, preserves `Retry-After` for callers and refuses redirects. The tests use a loopback mock and check the method, URL, token header, decoded node and HTTP errors.
- The module uses a local import path until a public repository is chosen. Go 1.26.10 and the browser driver are pinned in [go.mod](../../go.mod).

## Reproduce

Run these commands from the project root with a compatible browser installed.

```sh
go build -o ui-check ./cmd/ui-check
./ui-check doctor
UICHECK_SPIKE_OUT="$PWD/.ui-check/m0" go test -count=1 -v ./internal/browser -run '^TestCapture$'
go test -count=1 -v ./internal/figma
```

Inspect `.ui-check/m0/capture.png` and `.ui-check/m0/dom.json`. The fixture places a 3 CSS px border above the 14 CSS px inset. A useful review check is to subtract the observed border from the observed child-to-parent rectangle difference.

## Review focus

- Check cleanup of the browser and its temporary profile on success and cancellation.
- Verify that URL validation rejects host confusion and token-bearing Figma redirects.
- Check that viewport dimensions, CSS geometry and backing PNG dimensions agree at DPR 1.
- Live Figma access, image export and retries belong to later integration work. This phase uses a mocked node response.

## Observed on this workspace

Go 1.26.10 on macOS arm64, chromedp v0.21.0 and Chrome 156.0.8078.80. The capture produced a 390 × 844 PNG and DOM observations with DPR 1, parent Y 0, top border 3 and child Y 17. The Figma mock cases passed for 200, 302, 401, 403, 404, 429 and 500. `doctor` launched Chrome and returned its product version over CDP. The capture tests also passed cancellation and blocked an HTTP redirect to a second loopback origin before that origin received a request. Linux and Windows browser execution still require platform-specific verification.

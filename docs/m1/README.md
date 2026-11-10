# M1 · Deterministic measured audit

Review the [M1 roadmap gate](../../fidelitycheck-docs/05-ROADMAP-FA.md), [product requirements](../../fidelitycheck-docs/02-PRD-FA.md), [technical design](../../fidelitycheck-docs/03-TRD-FA.md) and [implementation brief](../../fidelitycheck-docs/04-IMPLEMENTATION-PROMPT.md). This phase depends on [M0](../m0/README.md).

## Reproduce with the built CLI

Start the fixture server from the project root in the first terminal.

```sh
go run ./examples/fixture-web
```

In a second terminal, build and run the audit. The final line should print `1` for the verified mismatch.

```sh
go build -o ui-check ./cmd/ui-check
./ui-check audit \
  --design-snapshot testdata/designs/signup.json \
  --target http://127.0.0.1:8042/signup \
  --no-ai --fail-on minor
printf '%s\n' "$?"
```

Open `index.html` in the run directory printed by the CLI. Keep its `assets/` and `evidence/` directories next to it. Run directories are unique and published after the files have been written. The raw design snapshot, synthetic reference PNG, DOM capture and target screenshot travel with the report.

## Implementation map

| Review area | Files |
|---|---|
| CLI flags, validation and exit codes | [audit command](../../internal/cli/audit.go), [main](../../cmd/ui-check/main.go) |
| Versioned snapshot and safe local PNG import | [loader](../../internal/audit/snapshot.go), [snapshot schema](../../schema/design-snapshot-v1.schema.json), [synthetic fixture](../../testdata/designs/signup.json) |
| Matching, measurement, coverage and severity policy | [comparison](../../internal/audit/audit.go), [browser capture](../../internal/browser/browser.go), [DOM extraction](../../internal/browser/dom.js) |
| Evidence checks and offline artifacts | [writer](../../internal/audit/report.go), [template](../../internal/audit/report.html), [report schema](../../schema/audit-report-v1.schema.json) |
| Real browser and mock fixtures | [CLI E2E](../../internal/cli/audit_test.go), [web fixture](../../internal/demo/demo.go) |

## Acceptance and review notes

- The measured top inset is `child.y − parent.y − parent.topBorder`. This defines the parent edge as the inside edge of its top border. The synthetic design coordinates yield `15 − 0 − 3 = 12`. Chrome measures `17 − 0 − 3 = 14`. The fixture reference PNG was captured from the local `/same` page as a visual stand-in for the synthetic design.
- The design-to-CSS scale is 1:1 at the snapshot viewport. DPR is 1. The validator resolves both design node IDs and both DOM IDs against the evidence and recomputes expected, actual and delta before writing a verified number. The `/margin` fixture produces an observed inset of 14 with computed padding of 12.
- A match requires unique test IDs, a direct DOM parent-child relationship and visible, supported geometry. Coverage describes the single supported pair. `matchConfidence=1` records that the explicit mapping passed those checks. Unknown coverage produces `INCONCLUSIVE` and omits numeric measurement and confidence.
- Tolerance defaults to 1 CSS px and a finding appears above that absolute delta. The fixture's finding is `minor`. The example uses `--fail-on minor` so the process exits 1. Under the default `major` policy the same finding remains in the report and the run exits 0.
- Operational errors exit 2 with a diagnostic on stderr. Completed reports carry `PASS`, `FAIL` or `INCONCLUSIVE`. The M1 browser scope is trusted loopback HTML. The CDP rule blocks cross-origin HTTP requests and non-GET requests in the captured tab. The milestone assumes the loopback fixture is trusted.
- The M1 snapshot schema models one explicit synthetic pair. Live Figma import, general matching, crawl, typography, image diff and provider integrations are later phases.

## Verification commands

```sh
UICHECK_E2E_OUT="$PWD/.ui-check/m1" go test -count=1 -v ./internal/cli
go test -count=1 ./...
go vet ./...
go build ./cmd/ui-check
```

| Fixture | Observed decision |
|---|---|
| `/signup`, fail on minor | `1 FAIL`, verified 12 → 14, delta +2 |
| `/same` | `0 PASS`, measured 12 → 12 |
| `/wrong` | `3 INCONCLUSIVE`, missing pair |
| `/ambiguous` | `3 INCONCLUSIVE`, duplicate ID |
| `/margin`, fail on minor | `1 FAIL`, measured 14 with computed padding 12 |
| `/signup`, fail on major | `0 PASS`, verified minor finding preserved |

The recorded run passed the test suite, vet and build on macOS arm64 with Go 1.26.10 and Chrome 156.0.8078.80. The separately built CLI produced the sample report and returned process exit code 1. Browser checks require permission to start Chrome and listen on a loopback port.

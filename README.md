# UI Check

A Go CLI that checks a web page against a design snapshot, measures rendered layout differences in Chrome, and creates an evidence-backed report.

**Project status · M1 working prototype.** The included demo checks one top inset between a parent and its child on a trusted local page. It expects **12 CSS px**, measures **14 CSS px** in Chrome, and reports a verified **+2 CSS px** difference. The demo runs without credentials or an AI provider.

## Quick start

You need Go 1.26.10 or newer and an installed Chrome or Chromium browser. Run these commands from the repository root.

```sh
go build -o ui-check ./cmd/ui-check
./ui-check doctor
```

If browser discovery fails, pass `--browser-path /path/to/chrome` to `doctor` and `audit`, or set `UICHECK_BROWSER_PATH`.

Start the included demo site in a separate terminal.

```sh
go run ./examples/fixture-web
```

Run the audit from the repository root.

```sh
./ui-check audit \
  --design-snapshot testdata/designs/signup.json \
  --target http://127.0.0.1:8042/signup \
  --no-ai --fail-on minor
```

This example prints `FAIL · .ui-check/report/run-…` and exits with code `1`. The mismatch is intentional. Try `/same` for a passing check or `/wrong` for an inconclusive result.

## Inputs and reports

- **Design** uses a [versioned JSON snapshot](schema/design-snapshot-v1.schema.json) and a sibling reference PNG. The [included fixture](testdata/designs/signup.json) defines the viewport, design node IDs, geometry and matching `data-testid` values.
- **Target** is an HTTP(S) loopback URL for a trusted page. The audit matches the parent and its direct child by unique test IDs, then measures the distance from the inside edge of the parent’s top border to the top of the child.
- **Output** is a separate directory per run. Open `index.html` for the offline report, or read `report.json` for [versioned machine-readable results](schema/audit-report-v1.schema.json). Keep the screenshot and evidence directories with the report so its links resolve.

The example uses `--fail-on minor` because the sample finding has minor severity. By default, findings above the 1 CSS px tolerance appear in the report and the exit code changes to `1` only when a verified finding meets the default `major` severity threshold.

| Exit code | Status | Meaning |
| --- | --- | --- |
| `0` | `PASS` | The pair was verified and no finding met the failure policy. |
| `1` | `FAIL` | A verified finding met the failure policy. |
| `2` | `ERROR` | An operational problem prevented the audit from completing. |
| `3` | `INCONCLUSIVE` | The audit could not establish a reliable match or measurement. |

## Development and review

```sh
go test -count=1 ./...
go vet ./...
go build ./cmd/ui-check
```

Browser tests launch Chrome and start fixture servers on loopback ports.

- [M0 review guide](docs/m0/README.md) covers browser capture, Figma REST mocks, acceptance criteria and reproduction commands.
- [M1 review guide](docs/m1/README.md) covers measurement, matching, report evidence and real-browser test cases.
- [Product and technical specs](fidelitycheck-docs/00-START-HERE-FA.md) and the [roadmap](fidelitycheck-docs/05-ROADMAP-FA.md) describe the planned Figma import, broader web audits and optional AI-assisted matching.

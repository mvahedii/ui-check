package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"

	"ui-check/internal/audit"
	"ui-check/internal/browser"
)

func runAudit(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("audit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	snapshot := flags.String("design-snapshot", "", "M1 structured design snapshot")
	target := flags.String("target", "", "trusted loopback HTTP(S) URL")
	route := flags.String("route", "", "same-origin absolute path override")
	path := flags.String("browser-path", "", "Chrome or Chromium executable")
	out := flags.String("out", ".ui-check/report", "parent directory for completed reports")
	noAI := flags.Bool("no-ai", true, "run without AI")
	policy := audit.Policy{}
	flags.Float64Var(&policy.Tolerance, "tolerance", 1, "top inset tolerance in CSS px")
	flags.StringVar(&policy.FailOn, "fail-on", "major", "failure severity: minor, major or critical")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return audit.Error.ExitCode()
	}
	if flags.NArg() != 0 || *snapshot == "" || *target == "" || *out == "" || !*noAI {
		return auditError(stderr, errors.New("use --design-snapshot and --target with a local no-AI audit"))
	}
	if err := policy.Validate(); err != nil {
		return auditError(stderr, err)
	}
	u, err := browser.LocalURL(*target)
	if err != nil {
		return auditError(stderr, err)
	}
	if *route != "" {
		relative, err := url.Parse(*route)
		if err != nil || relative.Host != "" || relative.IsAbs() || !strings.HasPrefix(*route, "/") || strings.HasPrefix(*route, "//") {
			return auditError(stderr, errors.New("route must be a same-origin absolute path"))
		}
		u, err = browser.LocalURL(u.ResolveReference(relative).String())
		if err != nil {
			return auditError(stderr, err)
		}
	}
	design, err := audit.LoadSnapshot(*snapshot)
	if err != nil {
		return auditError(stderr, err)
	}
	executable, err := browser.Find(*path)
	if err != nil {
		return auditError(stderr, err)
	}
	scene, err := browser.Capture(ctx, executable, u, design.Width, design.Height)
	if err != nil {
		return auditError(stderr, err)
	}
	result := audit.Compare(design, scene, policy)
	dir, err := audit.WriteReport(*out, result, design, scene)
	if err != nil {
		return auditError(stderr, err)
	}
	fmt.Fprintf(stdout, "%s · %s\n", result.Status, dir)
	return result.Status.ExitCode()
}

func auditError(w io.Writer, err error) int {
	fmt.Fprintf(w, "ERROR: %s\n", err)
	return audit.Error.ExitCode()
}

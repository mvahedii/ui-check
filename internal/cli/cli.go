package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"ui-check/internal/browser"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprintln(stdout, "Usage: ui-check <doctor|audit> [flags]")
		return 0
	}
	if args[0] == "audit" {
		return runAudit(ctx, args[1:], stdout, stderr)
	}
	if args[0] != "doctor" {
		fmt.Fprintln(stderr, "Unknown command. See ui-check --help.")
		return 2
	}
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("browser-path", "", "Chrome or Chromium executable")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "doctor does not accept positional arguments")
		return 2
	}
	executable, err := browser.Find(*path)
	if err == nil {
		var product string
		product, err = browser.Doctor(ctx, executable)
		if err == nil {
			fmt.Fprintf(stdout, "Chrome/Chromium: %s\nExecutable: %s\n", product, executable)
			return 0
		}
	}
	fmt.Fprintf(stderr, "ERROR: %s\n", err)
	return 2
}

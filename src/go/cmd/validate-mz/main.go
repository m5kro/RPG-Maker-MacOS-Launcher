package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rpgm-launcher/nwjs-helper/internal/validation"
)

func run(ctx context.Context, args []string, out io.Writer) int {
	flags := flag.NewFlagSet("validate-mz", flag.ContinueOnError)
	flags.SetOutput(out)
	var c validation.Config
	flags.StringVar(&c.Root, "root", "", "nonexistent absolute output root (parent must exist)")
	flags.StringVar(&c.FixtureZIP, "fixture-zip", "", "existing reviewed unacro commit 81abd876 ZIP; pinned SHA-256")
	flags.StringVar(&c.Runtime, "runtime", "", "existing absolute NW.js v0.94.1 directory; copied, never downloaded")
	flags.BoolVar(&c.ReviewedFixture, "reviewed-fixture", false, "I reviewed this public fixture and consent to execute it; isolation is NOT a sandbox")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || c.Root == "" || c.FixtureZIP == "" || c.Runtime == "" || !c.ReviewedFixture {
		fmt.Fprintln(out, "required: --root ABS --fixture-zip ABS --runtime ABS/v0.94.1 --reviewed-fixture")
		return 2
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	if err := validation.Run(ctx, c); err != nil {
		fmt.Fprintln(out, "FAIL:", err)
		return 1
	}
	fmt.Fprintln(out, "PASS:", c.Root+"/evidence")
	return 0
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout)
	stop()
	os.Exit(code)
}

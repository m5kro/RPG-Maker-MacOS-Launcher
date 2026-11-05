package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"rpgm-launcher/nwjs-helper/internal/launcher"
)

func main() {
	os.Exit(process())
}

func process() int {
	// A disappearing GUI pipe must return EPIPE, not terminate before staging cleanup.
	signal.Ignore(syscall.SIGPIPE)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return launcher.RunCLI(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

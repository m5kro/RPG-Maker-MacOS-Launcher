package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"rpgm-launcher/nwjs-helper/internal/launcher"
)

func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] != "launch" {
		fmt.Fprintln(errOut, "usage: rpgm-cli launch --root ABS --game ABS --resources ABS [--runtime VERSION] [--wait]")
		return 2
	}
	flags := flag.NewFlagSet("launch", flag.ContinueOnError)
	flags.SetOutput(errOut)
	root := flags.String("root", "", "absolute launcher-managed runtime root")
	gamePath := flags.String("game", "", "absolute path to a game copy (may be modified)")
	resources := flags.String("resources", "", "absolute path to launcher JavaScript resources")
	version := flags.String("runtime", "", "NW.js version; defaults to saved selection")
	wait := flags.Bool("wait", false, "wait for game exit; interruption does not stop the game")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*root) || !filepath.IsAbs(*gamePath) || !filepath.IsAbs(*resources) {
		fmt.Fprintln(errOut, "--root, --game and --resources must be absolute paths; no positional arguments allowed")
		return 2
	}
	fail := func(err error) int { fmt.Fprintln(errOut, "error:", err); return 1 }
	game, err := launcher.DetectGame(*gamePath)
	if err != nil {
		return fail(err)
	}
	settings, err := launcher.LoadSettings(*root)
	if err != nil {
		return fail(err)
	}
	if *version != "" {
		settings.LastSelectedVersion = *version
	}
	manager, err := launcher.NewManager(*root, *resources, errOut)
	if err != nil {
		return fail(err)
	}
	process, err := manager.Launch(ctx, game, settings)
	if err != nil {
		return fail(err)
	}
	encoder := json.NewEncoder(out)
	if err = encoder.Encode(map[string]any{"event": "started", "pid": process.PID, "engine": game.Engine, "log": process.LogPath, "warnings": process.Warnings}); err != nil {
		return fail(err)
	}
	if !*wait {
		return 0
	}
	code, err := process.Wait(ctx)
	if ctx.Err() != nil {
		return fail(ctx.Err())
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return fail(err)
		}
	}
	if err = encoder.Encode(exitedEvent(code, err)); err != nil {
		return fail(err)
	}
	if code != 0 {
		return 1
	}
	return 0
}

func exitedEvent(code int, err error) map[string]any {
	event := map[string]any{"event": "exited", "exit_code": code}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			event["signal"] = status.Signal().String()
		}
	}
	return event
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

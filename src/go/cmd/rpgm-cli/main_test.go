package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
)

func TestExitedEvent(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		want         map[string]any
	}{
		{"success", "exit 0", map[string]any{"event": "exited", "exit_code": 0}},
		{"nonzero", "exit 7", map[string]any{"event": "exited", "exit_code": 7}},
		{"signal", "kill -TERM $$", map[string]any{"event": "exited", "exit_code": -1, "signal": syscall.SIGTERM.String()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c", tc.script)
			err := cmd.Run()
			if cmd.ProcessState == nil {
				t.Fatalf("process did not run: %v", err)
			}
			if got := exitedEvent(cmd.ProcessState.ExitCode(), err); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("exitedEvent = %#v, want %#v (wait error: %v)", got, tc.want, err)
			}
		})
	}
}

func TestLaunchRejectsInvalidArguments(t *testing.T) {
	cases := []struct {
		name string
		args func(root, game, resources string) []string
	}{
		{"no command", func(_, _, _ string) []string { return nil }},
		{"unknown command", func(_, _, _ string) []string { return []string{"unknown"} }},
		{"no options", func(_, _, _ string) []string { return []string{"launch"} }},
		{"missing root", func(_, g, r string) []string { return []string{"launch", "--game", g, "--resources", r} }},
		{"missing game", func(root, _, r string) []string { return []string{"launch", "--root", root, "--resources", r} }},
		{"missing resources", func(root, g, _ string) []string { return []string{"launch", "--root", root, "--game", g} }},
		{"relative root", func(_, g, r string) []string {
			return []string{"launch", "--root", "relative", "--game", g, "--resources", r}
		}},
		{"relative game", func(root, _, r string) []string {
			return []string{"launch", "--root", root, "--game", "relative", "--resources", r}
		}},
		{"relative resources", func(root, g, _ string) []string {
			return []string{"launch", "--root", root, "--game", g, "--resources", "relative"}
		}},
		{"extra argument", func(root, g, r string) []string {
			return []string{"launch", "--root", root, "--game", g, "--resources", r, "extra"}
		}},
		{"unknown option", func(root, g, r string) []string {
			return []string{"launch", "--root", root, "--game", g, "--resources", r, "--unknown"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, game, resources := t.TempDir(), t.TempDir(), t.TempDir()
			pkg := filepath.Join(game, "package.json")
			original := []byte("{\"name\":\"valuable\"}\n")
			if err := os.WriteFile(pkg, original, 0600); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			if code := run(context.Background(), tc.args(root, game, resources), &out, &errOut); code == 0 {
				t.Fatalf("invalid arguments accepted; stdout=%q stderr=%q", out.String(), errOut.String())
			}
			after, err := os.ReadFile(pkg)
			if err != nil || !bytes.Equal(after, original) {
				t.Fatalf("package.json changed: %q, %v", after, err)
			}
		})
	}
}

func TestLaunchRejectsInvalidGame(t *testing.T) {
	root, game, resources := t.TempDir(), t.TempDir(), t.TempDir()
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"launch", "--root", root, "--game", game, "--resources", resources}, &out, &errOut); code == 0 {
		t.Fatalf("invalid game accepted; stdout=%q stderr=%q", out.String(), errOut.String())
	}
	if _, err := os.Stat(filepath.Join(game, "package.json")); !os.IsNotExist(err) {
		t.Fatalf("invalid game package.json unexpectedly created or inaccessible: %v", err)
	}
}

func TestLaunchMissingRuntimePreservesPackage(t *testing.T) {
	root, game, resources := t.TempDir(), t.TempDir(), t.TempDir()
	pkg := filepath.Join(game, "package.json")
	original := []byte("{\"name\":\"valuable\"}\n")
	if err := os.WriteFile(pkg, original, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	args := []string{"launch", "--root", root, "--game", game, "--resources", resources, "--runtime", "v1.2.3", "--wait"}
	if code := run(context.Background(), args, &out, &errOut); code == 0 {
		t.Fatalf("missing runtime accepted; stdout=%q stderr=%q", out.String(), errOut.String())
	}
	after, err := os.ReadFile(pkg)
	if err != nil || !bytes.Equal(after, original) {
		t.Fatalf("package.json changed: %q, %v", after, err)
	}
}

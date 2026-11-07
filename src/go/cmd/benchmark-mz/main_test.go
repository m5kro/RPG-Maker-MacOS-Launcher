package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyGame(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	if err := os.WriteFile(filepath.Join(src, "file"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := copyGame(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "file"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(src, "file"))
	if err != nil || string(b) != "original" {
		t.Fatalf("source: %q %v", b, err)
	}
	if err := copyGame(src, dst); err == nil {
		t.Fatal("accepted existing destination")
	}
	if err := os.Symlink("file", filepath.Join(src, "link")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation requires Windows privileges: %v", err)
		}
		t.Fatal(err)
	}
	if err := copyGame(src, filepath.Join(t.TempDir(), "copy")); err == nil {
		t.Fatal("accepted symlink")
	}
}
func TestRuntimeBinary(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "nwjs.app")
	macBinary := filepath.Join(app, "Contents", "MacOS", "nwjs")
	if err := os.MkdirAll(filepath.Dir(macBinary), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{macBinary, filepath.Join(root, "nw.EXE"), filepath.Join(root, "not-runtime.txt")} {
		if err := os.WriteFile(name, []byte("test"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		path, platform, want string
	}{
		{app, "darwin", macBinary},
		{filepath.Join(root, "nw.EXE"), "windows", filepath.Join(root, "nw.EXE")},
		{root, "windows", ""},
		{filepath.Join(root, "missing.exe"), "windows", ""},
		{filepath.Join(root, "not-runtime.txt"), "windows", ""},
		{app, "linux", ""},
	} {
		t.Run(tc.platform+"/"+filepath.Base(tc.path), func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.platform == "darwin" && tc.want != "" {
				t.Skip("Windows cannot emulate Unix executable permission bits")
			}
			got, err := runtimeBinary(tc.path, tc.platform)
			if tc.want == "" {
				if err == nil {
					t.Fatal("accepted invalid runtime")
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestOutputOutsideGame(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	for _, output := range []string{game, filepath.Join(game, "results")} {
		if err := checkOutputOutsideGame(game, output); err == nil {
			t.Fatalf("accepted output inside game: %s", output)
		}
	}
	for _, output := range []string{root, filepath.Join(root, "game-results")} {
		if err := checkOutputOutsideGame(game, output); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS == "windows" {
		if err := checkOutputOutsideGame(`C:\game`, `D:\results`); err != nil {
			t.Fatal(err)
		}
		if err := checkOutputOutsideGame(`C:\game`, `c:\GAME\results`); err == nil {
			t.Fatal("accepted case-insensitive descendant")
		}
	}
}

func TestInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--warmup", "-1"}, {"--duration", "0"}} {
		if err := run(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

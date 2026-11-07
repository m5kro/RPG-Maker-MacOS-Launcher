package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogCompatibility(t *testing.T) {
	u := newTestUI(t)
	root := u.manager.Root
	legacy := filepath.Join(root, "log.txt")
	if err := os.WriteFile(legacy, []byte("旧ログ"), 0600); err != nil {
		t.Fatal(err)
	}
	u.log("操作ログ")
	if err := os.WriteFile(filepath.Join(root, "game.log"), []byte("ゲームログ"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := New(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	text, err := next.Logs()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"旧ログ", "操作ログ", "ゲームログ"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	b, err := os.ReadFile(legacy)
	if err != nil || string(b) != "旧ログ" {
		t.Fatalf("legacy changed: %q %v", b, err)
	}
}

func TestLauncherLogPermissions(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0644, 0620, 0601} {
		t.Run(mode.String(), func(t *testing.T) {
			u := newTestUI(t)
			path := filepath.Join(u.manager.Root, "launcher.log")
			if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			_, err := u.logs.Write([]byte("secret"))
			if (err == nil) != (mode == 0600) {
				t.Fatalf("mode %v: %v", mode, err)
			}
			want := "old"
			if mode == 0600 {
				want += "secret"
			}
			if b, err := os.ReadFile(path); err != nil || string(b) != want {
				t.Fatalf("log contents: %q %v", b, err)
			}
		})
	}
}

func TestLauncherLogRejectsSymlink(t *testing.T) {
	u := newTestUI(t)
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(u.manager.Root, "launcher.log")); err != nil {
		t.Fatal(err)
	}
	if _, err := u.logs.Write([]byte("bad")); err == nil {
		t.Fatal("symlink accepted")
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "unchanged" {
		t.Fatalf("target changed: %q %v", b, err)
	}
}

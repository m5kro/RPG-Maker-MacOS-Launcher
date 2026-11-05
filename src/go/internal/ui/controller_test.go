package ui

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rpgm-launcher/nwjs-helper/internal/launcher"
)

func newTestUI(t *testing.T) *UI {
	t.Helper()
	u, err := New(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestBusyCancelWaitsForCleanupAndClose(t *testing.T) {
	u := newTestUI(t)
	entered, cleanup, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		_ = u.operation("test", func(ctx context.Context) error { close(entered); <-ctx.Done(); <-cleanup; return ctx.Err() })
		close(finished)
	}()
	<-entered
	if err := u.operation("second", func(context.Context) error { t.Error("ran second operation"); return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("second = %v", err)
	}
	u.CancelOperation()
	if !u.State().Busy || !u.State().Cancelling {
		t.Fatal("idle before cleanup")
	}
	quit := make(chan struct{})
	u.closeAfterCancel(func() { close(quit) })
	select {
	case <-quit:
		t.Fatal("quit before cleanup")
	case <-time.After(20 * time.Millisecond):
	}
	close(cleanup)
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("quit not called")
	}
	<-finished
	if u.State().Busy {
		t.Fatal("still busy")
	}
	if err := u.operation("after closing", func(context.Context) error { return nil }); !errors.Is(err, ErrClosing) {
		t.Fatalf("closing = %v", err)
	}
}

func TestFormTypesExactAndForce(t *testing.T) {
	u := newTestUI(t)
	a := launcher.DefaultAdvancedSettings()
	a.MKXPZ["JITMaxCache"] = json.Number("9007199254740993123456789")
	a.MKXPZ["windowTitle"] = "001984"
	a.MKXPZ["solidFonts"] = []any{"123", json.Number("9007199254740993123")}
	a.MKXPZ["bindingNames"] = map[string]any{"日本語": json.Number("9007199254740993123")}
	a.MKXPZ["future"] = "0012"
	a.MKXPZEnabled["windowTitle"] = "True"
	a.MKXPZEnabled["future"] = true
	if err := launcher.SaveAdvancedSettings(u.manager.Root, a); err != nil {
		t.Fatal(err)
	}
	fields, err := u.Advanced("MKXPZ", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 62 {
		t.Fatalf("fields %d", len(fields))
	}
	if err = u.SaveAdvanced("MKXPZ", fields); err != nil {
		t.Fatal(err)
	}
	b, err := launcher.LoadAdvancedSettings(u.manager.Root)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"JITMaxCache", "windowTitle", "solidFonts", "bindingNames", "future"} {
		x, _ := json.Marshal(a.MKXPZ[key])
		y, _ := json.Marshal(b.MKXPZ[key])
		if string(x) != string(y) {
			t.Errorf("%s: %s != %s", key, x, y)
		}
	}
	if b.MKXPZEnabled["windowTitle"] != "True" || b.MKXPZEnabled["future"] != true {
		t.Fatal("enabled types lost")
	}
	for i := range fields {
		if fields[i].Key == "rgssVersion" {
			fields[i].EnabledJSON = "false"
		}
	}
	if err = u.SaveAdvanced("MKXPZ", fields); err == nil {
		t.Fatal("Force changed")
	}
}

func TestFormDefaultsAndInvalidInput(t *testing.T) {
	u := newTestUI(t)
	for engine, count := range map[string]int{"MKXPZ": 61, "EasyRPG": 56} {
		fields, err := u.Advanced(engine, false)
		if err != nil || len(fields) != count {
			t.Fatalf("%s: %d %v", engine, len(fields), err)
		}
	}
	fields, _ := u.Advanced("MKXPZ", false)
	for i := range fields {
		if fields[i].Key == "windowTitle" {
			fields[i].ValueJSON = "1984"
		}
	}
	if err := u.SaveAdvanced("MKXPZ", fields); err == nil {
		t.Fatal("string coerced to number")
	}
	if _, err := u.Advanced("unknown", false); err == nil {
		t.Fatal("unknown engine")
	}
	for _, req := range []Request{{Action: "unknown"}, {Action: "install", Component: "../bad", Approved: true}, {Action: "install", Component: "v0.94.1"}, {Action: "uninstall-all"}, {Action: "export", Destination: "relative", Name: "x", Approved: true}} {
		if _, err := u.Run(req); err == nil {
			t.Errorf("accepted %#v", req)
		}
	}
	if _, err := u.SelectFolder("relative"); err == nil {
		t.Fatal("relative game accepted")
	}
	if err := u.SavePreferences(Preferences{Version: "../bad"}); err == nil {
		t.Fatal("invalid version")
	}
}

func TestCorruptSettingsArePreserved(t *testing.T) {
	u := newTestUI(t)
	path := filepath.Join(u.manager.Root, "mkxpz-advanced.json")
	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := u.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.SettingsError == "" {
		t.Fatal("no recovery message")
	}
	if _, err = u.Advanced("MKXPZ", false); err == nil {
		t.Fatal("invalid settings hidden")
	}
	if err = u.SaveAdvanced("MKXPZ", nil); err == nil {
		t.Fatal("overwrote corrupt settings")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{broken" {
		t.Fatal("corrupt settings removed")
	}
}

func TestInstallRejectsCorruptSettingsBeforeTouchingRuntime(t *testing.T) {
	u := newTestUI(t)
	if err := os.WriteFile(filepath.Join(u.manager.Root, "settings.json"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(u.manager.Root, "v0.94.1"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err := u.Run(Request{Action: "install", Component: "v0.94.1", Approved: true})
	if err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("must validate settings first, got %v", err)
	}
}

func TestResetKeepsUnknownSettingsAndForce(t *testing.T) {
	u := newTestUI(t)
	a := launcher.DefaultAdvancedSettings()
	a.MKXPZ["future"] = map[string]any{"keep": "0012"}
	a.MKXPZEnabled["future"] = true
	a.MKXPZ["windowTitle"] = "changed"
	if err := launcher.SaveAdvancedSettings(u.manager.Root, a); err != nil {
		t.Fatal(err)
	}
	fields, err := u.Advanced("MKXPZ", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = u.SaveAdvanced("MKXPZ", fields); err != nil {
		t.Fatal(err)
	}
	b, err := launcher.LoadAdvancedSettings(u.manager.Root)
	if err != nil {
		t.Fatal(err)
	}
	if b.MKXPZ["windowTitle"] != "Custom Title" || raw(b.MKXPZ["future"]) != raw(a.MKXPZ["future"]) || b.MKXPZEnabled["rgssVersion"] != "Force" {
		t.Fatalf("reset %#v", b)
	}
}

func TestSavePreferencesAllowsMovedRememberedFolder(t *testing.T) {
	u := newTestUI(t)
	if err := u.SavePreferences(Preferences{Folder: filepath.Join(t.TempDir(), "moved"), Version: "v0.94.1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := u.Run(Request{Action: "launch"}); err == nil {
		t.Fatal("missing folder launched")
	}
}

func TestLogsBounded(t *testing.T) {
	u := newTestUI(t)
	_, _ = u.logs.Write([]byte(strings.Repeat("日本語", 100000)))
	if n := len(u.logs.String()); n > maxLogBytes {
		t.Fatalf("log %d", n)
	}
}

func TestDestructiveAndExportRouting(t *testing.T) {
	for _, engine := range []string{"NWJS", "MKXPZ", "EasyRPG"} {
		t.Run(engine, func(t *testing.T) {
			u := newTestUI(t)
			game := t.TempDir()
			dest := t.TempDir()
			exe, file, body := "v0.94.1/nwjs.app/Contents/MacOS/nwjs", "package.json", `{"name":"test","main":"index.html"}`
			if engine == "MKXPZ" {
				exe, file, body = "Z-universal.app/Contents/MacOS/Z-universal", "Game.ini", "[Game]\nRTP=Standard\n"
			}
			if engine == "EasyRPG" {
				exe, file, body = "EasyRPG Player.app/Contents/MacOS/EasyRPG Player", "RPG_RT.ldb", "game"
			}
			path := filepath.Join(u.manager.Root, exe)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			arm := make([]byte, 32)
			binary.LittleEndian.PutUint32(arm, 0xfeedfacf)
			binary.LittleEndian.PutUint32(arm[4:], 0x0100000c)
			binary.LittleEndian.PutUint32(arm[12:], 2)
			if err := os.WriteFile(path, arm, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(game, file), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := u.SelectFolder(game); err != nil {
				t.Fatal(err)
			}
			if err := u.SavePreferences(Preferences{Folder: game, Version: "v0.94.1", DisableProtection: true}); err != nil {
				t.Fatal(err)
			}
			if engine == "MKXPZ" {
				a := launcher.DefaultAdvancedSettings()
				for _, key := range []string{"RTP", "preloadScript", "midiSoundFont"} {
					a.MKXPZEnabled[key] = false
				}
				if err := launcher.SaveAdvancedSettings(u.manager.Root, a); err != nil {
					t.Fatal(err)
				}
			}
			result, err := u.Run(Request{Action: "export", Destination: dest, Name: "日本語", Approved: true})
			if err != nil {
				t.Fatal(err)
			}
			realDest, err := filepath.EvalSymlinks(dest)
			if err != nil {
				t.Fatal(err)
			}
			if result.Path != filepath.Join(realDest, "日本語.app") {
				t.Fatal(result)
			}
			original, _ := os.ReadFile(filepath.Join(game, file))
			if string(original) != body {
				t.Fatal("source changed")
			}
			c := engine
			if c == "NWJS" {
				c = "v0.94.1"
			}
			if _, err := u.Run(Request{Action: "uninstall", Component: c, Approved: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("component not removed")
			}
			if _, err := u.Run(Request{Action: "uninstall-all", Approved: true}); err != nil {
				t.Fatal(err)
			}
			snap, err := u.Snapshot()
			if err != nil || snap.Preferences.Version != "" {
				t.Fatalf("stale settings: %#v %v", snap, err)
			}
			if _, err := os.Stat(filepath.Join(game, file)); err != nil {
				t.Fatal("game removed")
			}
		})
	}
}

func TestActionRoutingOfflineAndNoImplicitDownload(t *testing.T) {
	u := newTestUI(t)
	game := t.TempDir()
	if err := os.WriteFile(filepath.Join(game, "package.json"), []byte(`{"name":"日本語"}`), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := u.SelectFolder(game)
	if err != nil || g.Engine != launcher.NWJS {
		t.Fatalf("game %v %v", g, err)
	}
	if err = u.SavePreferences(Preferences{Folder: game, Version: "v0.94.1", PatchGetPixel: true}); err != nil {
		t.Fatal(err)
	}
	result, err := u.Run(Request{Action: "missing"})
	if err != nil || len(result.Missing) != 1 || result.Missing[0] != "v0.94.1" {
		t.Fatalf("missing %#v %v", result, err)
	}
	if _, err = u.Run(Request{Action: "launch"}); err == nil {
		t.Fatal("launched without runtime")
	}
	if _, err = os.Stat(filepath.Join(u.manager.Root, "v0.94.1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("download happened")
	}
	snap, err := u.Snapshot()
	if err != nil || snap.Preferences.Version != "v0.94.1" {
		t.Fatalf("offline %#v %v", snap, err)
	}
}

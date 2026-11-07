package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func engineContractSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || d.Name() == ".nwjs-install.lock" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		result[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestEngineContractExportAllAndOriginals(t *testing.T) {
	for _, engine := range []GameEngine{NWJS, MKXPZ, EasyRPG} {
		t.Run(string(engine), func(t *testing.T) {
			m := engineContractManager(t)
			g := engineContractGame(t, engine)
			engineContractRuntime(t, m, engine)
			if engine == MKXPZ {
				engineContractInstallSupport(t, m)
				a := DefaultAdvancedSettings()
				a.MKXPZEnabled["fontSub"] = true
				if err := SaveAdvancedSettings(m.Root, a); err != nil {
					t.Fatal(err)
				}
			}
			if engine != MKXPZ {
				engineContractWrite(t, filepath.Join(g.Path, ".rpgm-resources", "user.txt"), []byte("user asset"), 0600)
			}
			before := engineContractSnapshot(t, g.Path)
			runtimeBefore := engineContractSnapshot(t, m.Root)
			dest := t.TempDir()
			out, err := m.Export(context.Background(), g, Settings{LastSelectedVersion: "v1.2.3", PatchGetPixel: true, OptimizeSpace: true}, dest, "Example", nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, engineContractSnapshot(t, g.Path)) {
				t.Fatal("source game changed")
			}
			if !reflect.DeepEqual(runtimeBefore, engineContractSnapshot(t, m.Root)) {
				t.Fatal("source runtime changed")
			}
			gameDir := filepath.Join(out, "Contents", "Resources", "app.nw")
			if engine == MKXPZ {
				gameDir = filepath.Join(out, "Contents", "Game")
			}
			if engine == EasyRPG {
				gameDir = filepath.Join(out, "Contents", "Resources", "game")
			}
			for _, p := range []string{"Game.exe", "save/keep", "nw.dll", "game_en.exe", "assets/English.json", "js/main.js", "js/plugins/Cheat_Menu.js"} {
				if b, _ := os.ReadFile(filepath.Join(gameDir, p)); string(b) != before[p] {
					t.Fatal("asset not preserved", p)
				}
			}
			if engine == NWJS {
				b, _ := os.ReadFile(filepath.Join(gameDir, "package.json"))
				if !bytes.Contains(b, []byte("tempname")) {
					t.Fatal(string(b))
				}
				b, _ = os.ReadFile(filepath.Join(gameDir, "js", "rmmz_core.js"))
				if !bytes.Contains(b, []byte("RPGM-Launcher getPixel")) {
					t.Fatal("patch missing")
				}
			}
			if engine == MKXPZ {
				b, _ := os.ReadFile(filepath.Join(gameDir, "mkxp.json"))
				var c map[string]any
				if err = json.Unmarshal(b, &c); err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(b, []byte(`"Arial>Open Sans"`)) || !reflect.DeepEqual(c["fontSub"], DefaultAdvancedSettings().MKXPZ["fontSub"]) {
					t.Fatalf("fontSub not preserved literally: %s", b)
				}
				if c["gameFolder"] != "./" {
					t.Fatal(c)
				}
				paths, _ := engineStringList(c["preloadScript"])
				if len(paths) != 1 || !strings.HasSuffix(paths[0], "preload.rb") {
					t.Fatal(c)
				}
				if _, err = os.Stat(filepath.Join(gameDir, paths[0])); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(b), m.Root) || strings.Contains(string(b), g.Path) {
					t.Fatal("absolute source leaked", string(b))
				}
			}
			snapshot := engineContractSnapshot(t, out)
			if _, err = m.Export(context.Background(), g, Settings{LastSelectedVersion: "v1.2.3"}, dest, "Example", nil); err == nil {
				t.Fatal("existing overwritten")
			}
			if !reflect.DeepEqual(snapshot, engineContractSnapshot(t, out)) {
				t.Fatal("existing output modified")
			}
		})
	}
}
func TestEngineContractExportCancellationAll(t *testing.T) {
	for _, engine := range []GameEngine{NWJS, MKXPZ, EasyRPG} {
		t.Run(string(engine), func(t *testing.T) {
			m := engineContractManager(t)
			g := engineContractGame(t, engine)
			engineContractRuntime(t, m, engine)
			if engine == MKXPZ {
				engineContractInstallSupport(t, m)
			}
			before := engineContractSnapshot(t, g.Path)
			for _, phase := range []string{"copy", "publish"} {
				dest := t.TempDir()
				stop := errors.New("user cancelled")
				calls := 0
				_, err := m.Export(context.Background(), g, Settings{LastSelectedVersion: "v1.2.3"}, dest, "Test", func(p Progress) error {
					calls++
					if p.Phase == phase && (phase != "copy" || calls > 2) {
						return stop
					}
					return nil
				})
				if !errors.Is(err, stop) {
					t.Fatalf("%s: %v", phase, err)
				}
				entries, _ := os.ReadDir(dest)
				if len(entries) != 0 {
					t.Fatal("partial output", entries)
				}
				if !reflect.DeepEqual(before, engineContractSnapshot(t, g.Path)) {
					t.Fatal("original modified")
				}
			}
		})
	}
}
func TestEngineContractExportPaths(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, NWJS)
	engineContractRuntime(t, m, NWJS)
	s := Settings{LastSelectedVersion: "v1.2.3"}
	dest := t.TempDir()
	for _, name := range []string{"../escape", "a/b", "a\\b", "", ".", "..", " spaced ", "bad\x00", "bad\tname", "bad\x01name", string([]byte{0xff})} {
		if _, err := m.Export(context.Background(), g, s, dest, name, nil); err == nil {
			t.Fatal("invalid name accepted", name)
		}
	}
	if _, err := m.Export(context.Background(), g, s, g.Path, "nested", nil); err == nil {
		t.Fatal("recursive output accepted")
	}
	outside := t.TempDir()
	engineContractWrite(t, filepath.Join(outside, "save"), []byte("outside"), 0600)
	if err := os.Symlink(outside, filepath.Join(g.Path, "outside")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Export(context.Background(), g, s, dest, "Outward", nil); err == nil {
		t.Fatal("outward symlink accepted")
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatal("preflight wrote output")
	}
}
func TestEngineContractExportMKXPCustomAndUnsupported(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, MKXPZ)
	engineContractRuntime(t, m, MKXPZ)
	a := DefaultAdvancedSettings()
	rtp := t.TempDir()
	engineContractWrite(t, filepath.Join(rtp, "tiles"), []byte("custom RTP"), 0600)
	engineContractWrite(t, filepath.Join(g.Path, "scripts", "pre.rb"), []byte("# pre"), 0600)
	engineContractWrite(t, filepath.Join(g.Path, "font.sf2"), engineContractSF2(), 0600)
	a.MKXPZ["RTP"] = []any{rtp}
	a.MKXPZ["preloadScript"] = []any{"scripts/pre.rb"}
	a.MKXPZ["midiSoundFont"] = "font.sf2"
	if err := SaveAdvancedSettings(m.Root, a); err != nil {
		t.Fatal(err)
	}
	out, err := m.Export(context.Background(), g, Settings{}, t.TempDir(), "Custom", nil)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "Contents/Game/.rpgm-resources/rtp-0/tiles")); string(b) != "custom RTP" {
		t.Fatal("custom RTP lost")
	}
	a.MKXPZ["preloadScript"] = []any{filepath.Join(rtp, "tiles")}
	if err = SaveAdvancedSettings(m.Root, a); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if _, err = m.Export(context.Background(), g, Settings{}, dest, "Unsupported", nil); err == nil {
		t.Fatal("external preload silently substituted")
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatal("unsupported settings wrote output")
	}
}
func TestEngineContractEasyOptionLookingValueIsNotAPathOption(t *testing.T) {
	g := engineContractGame(t, EasyRPG)
	a := DefaultAdvancedSettings()
	a.EasyRPGEnabled["--encoding"] = true
	a.EasyRPG["--encoding"] = "--soundfont"
	args, err := exportEasyArguments(g, a)
	if err != nil || !reflect.DeepEqual(args, []string{"--encoding", "--soundfont"}) {
		t.Fatalf("value interpreted as option: %v %v", args, err)
	}
}
func TestEngineContractEasyExportQuotingAndExternal(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, EasyRPG)
	engineContractRuntime(t, m, EasyRPG)
	a := DefaultAdvancedSettings()
	value := "ja'\"; $(touch UNWANTED) `touch BAD` $HOME\nline"
	a.EasyRPGEnabled["--encoding"] = "True"
	a.EasyRPG["--encoding"] = value
	a.EasyRPGEnabled["--seed"] = true
	a.EasyRPG["--seed"] = json.Number("0")
	if err := SaveAdvancedSettings(m.Root, a); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	out, err := m.Export(context.Background(), g, Settings{}, dest, "Quoted & name", nil)
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(out, "Contents/MacOS/launch"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(script, []byte("/usr/bin/arch -arm64")) || !bytes.Contains(script, []byte(shellQuote(value))) {
		t.Fatal(string(script))
	}
	// Execute the exact generated quoting with printf, without launching a real game.
	cmd := exec.Command("/bin/sh", "-c", "printf '%s' "+shellQuote(value))
	cmd.Dir = dest
	got, err := cmd.Output()
	if err != nil || string(got) != value {
		t.Fatalf("quote %q %v", got, err)
	}
	a.EasyRPGEnabled["--soundfont"] = true
	a.EasyRPG["--soundfont"] = "/outside/font.sf2"
	if err = SaveAdvancedSettings(m.Root, a); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	if _, err = m.Export(context.Background(), g, Settings{}, empty, "Bad", nil); err == nil {
		t.Fatal("external resource accepted")
	}
	entries, _ := os.ReadDir(empty)
	if len(entries) != 0 {
		t.Fatal("preflight wrote output")
	}
}
func TestEngineContractCopyInternalSymlinkChain(t *testing.T) {
	source := t.TempDir()
	engineContractWrite(t, filepath.Join(source, "data"), []byte("data"), 0600)
	for name, target := range map[string]string{"a": "b", "b": "data"} {
		if err := os.Symlink(target, filepath.Join(source, name)); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(t.TempDir(), "copy")
	if err := engineCopyTree(context.Background(), source, dest, nil); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "a")); err != nil || string(b) != "data" {
		t.Fatal(string(b), err)
	}
}
func TestEngineContractExportRejectsOutputInsideRTPBeforeCopy(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, MKXPZ)
	engineContractRuntime(t, m, MKXPZ)
	a := DefaultAdvancedSettings()
	rtp := t.TempDir()
	engineContractWrite(t, filepath.Join(rtp, "tiles"), []byte("tiles"), 0600)
	a.MKXPZ["RTP"] = []any{rtp}
	a.MKXPZEnabled["preloadScript"] = false
	a.MKXPZEnabled["midiSoundFont"] = false
	if err := SaveAdvancedSettings(m.Root, a); err != nil {
		t.Fatal(err)
	}
	calls := 0
	_, err := m.Export(context.Background(), g, Settings{}, rtp, "Recursive", func(Progress) error { calls++; return errors.New("stop") })
	if err == nil || calls != 0 {
		t.Fatalf("not rejected before copy: calls=%d err=%v", calls, err)
	}
}
func TestEngineContractExportContextCancel(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, EasyRPG)
	engineContractRuntime(t, m, EasyRPG)
	dest := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := m.Export(ctx, g, Settings{}, dest, "Cancelled", func(p Progress) error { cancel(); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) > 0 {
		t.Fatal("partial output")
	}
}

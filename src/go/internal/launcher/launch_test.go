package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEngineContractEasyArgumentsTypes(t *testing.T) {
	a := AdvancedSettings{EasyRPGEnabled: map[string]any{}, EasyRPG: map[string]any{}}
	values := map[string]any{"--true": true, "--false": false, "--zero": json.Number("0"), "--one": json.Number("1"), "--float": json.Number("4.0"), "--string": "0012", "--boolean-string": "false", "--empty": "", "--quote": "'; $(touch bad)\n\""}
	for k, v := range values {
		a.EasyRPGEnabled[k] = "Force"
		a.EasyRPG[k] = v
	}
	args, err := easyArguments(a)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--boolean-string", "false", "--float", "4.0", "--one", "1", "--quote", "'; $(touch bad)\n\"", "--string", "0012", "--true", "--zero", "0"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%q", args)
	}
	a.EasyRPG["--zero"] = []any{1}
	if _, err = easyArguments(a); err == nil {
		t.Fatal("array accepted")
	}
}
func TestEngineContractLaunchArgumentsAndExit(t *testing.T) {
	for _, engine := range []GameEngine{NWJS, MKXPZ, EasyRPG} {
		t.Run(string(engine), func(t *testing.T) {
			m := engineContractManager(t)
			g := engineContractGame(t, engine)
			engineContractRuntime(t, m, engine)
			if engine == MKXPZ {
				engineContractInstallSupport(t, m)
			}
			var program string
			var args []string
			m.command = func(p string, a ...string) *exec.Cmd {
				program = p
				args = append([]string(nil), a...)
				return exec.Command("/bin/sh", "-c", "printf 'cwd=%s\\n' \"$PWD\"; exit 7")
			}
			p, err := m.Launch(context.Background(), g, Settings{LastSelectedVersion: "v1.2.3", PatchGetPixel: true})
			if err != nil {
				t.Fatal(err)
			}
			code, err := p.Wait(context.Background())
			if code != 7 || err == nil {
				t.Fatalf("exit %d %v", code, err)
			}
			log, err := os.ReadFile(p.LogPath)
			if err != nil || !strings.Contains(string(log), g.Path) {
				t.Fatalf("log %q %v", log, err)
			}
			if engine == NWJS {
				if !strings.HasSuffix(program, "/nwjs") || !reflect.DeepEqual(args, []string{g.Path}) {
					t.Fatalf("%s %q", program, args)
				}
			} else {
				if program != "/usr/bin/arch" || args[0] != "-arm64" {
					t.Fatalf("%s %q", program, args)
				}
			}
			if _, err = os.Stat(filepath.Join(g.Path, "Game.exe")); err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(filepath.Join(g.Path, "save", "keep")); string(b) != "precious" {
				t.Fatal("save changed")
			}
		})
	}
}
func TestEngineContractGameLifetimeAndLock(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, EasyRPG)
	engineContractRuntime(t, m, EasyRPG)
	gate := filepath.Join(t.TempDir(), "exit")
	m.command = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("/bin/sh", "-c", "while [ ! -f \"$1\" ]; do sleep 0.02; done; exit 0", "game", gate)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p, err := m.Launch(ctx, g, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Signal(os.Kill)
	cancel()
	time.Sleep(30 * time.Millisecond)
	select {
	case <-p.Done:
		t.Fatal("game died with launch context")
	default:
	}
	if _, err = p.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = m.UninstallAll(context.Background()); err == nil {
		t.Fatal("removed live runtime")
	}
	engineContractWrite(t, gate, nil, 0600)
	waitCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if code, err := p.Wait(waitCtx); err != nil || code != 0 {
		t.Fatalf("%d %v", code, err)
	}
	if err = m.Uninstall(context.Background(), ComponentEasyRPG); err != nil {
		t.Fatal(err)
	}
}
func TestEngineContractNWNonRPGPatchWarningDoesNotPreventLaunch(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, NWJS)
	engineContractRuntime(t, m, NWJS)
	if err := os.RemoveAll(filepath.Join(g.Path, "js")); err != nil {
		t.Fatal(err)
	}
	m.command = func(_ string, _ ...string) *exec.Cmd { return exec.Command("/usr/bin/true") }
	p, err := m.Launch(context.Background(), g, Settings{LastSelectedVersion: "v1.2.3", PatchGetPixel: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) == 0 {
		t.Fatal("patch failure hidden")
	}
}
func TestEngineContractNWAllowsLinkedAssetsButNotOutwardRepair(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, NWJS)
	engineContractRuntime(t, m, NWJS)
	outside := t.TempDir()
	engineContractWrite(t, filepath.Join(outside, "save"), []byte("keep"), 0600)
	if err := os.Symlink(outside, filepath.Join(g.Path, "linked-assets")); err != nil {
		t.Fatal(err)
	}
	m.command = func(_ string, _ ...string) *exec.Cmd { return exec.Command("/usr/bin/true") }
	p, err := m.Launch(context.Background(), g, Settings{LastSelectedVersion: "v1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = p.Wait(context.Background())
	if b, _ := os.ReadFile(filepath.Join(outside, "save")); string(b) != "keep" {
		t.Fatal("linked assets changed")
	}
	if err = os.RemoveAll(filepath.Join(g.Path, "js")); err != nil {
		t.Fatal(err)
	}
	engineContractWrite(t, filepath.Join(outside, "js", "rmmz_core.js"), []byte("Bitmap.prototype.getPixel = function(x, y) {\n return '#ffffff';\n};\n"), 0600)
	if err = os.Symlink(outside, filepath.Join(g.Path, "www")); err != nil {
		t.Fatal(err)
	}
	before := engineContractSnapshot(t, outside)
	if p, err = m.Launch(context.Background(), g, Settings{LastSelectedVersion: "v1.2.3", PatchGetPixel: true}); err == nil {
		_, _ = p.Wait(context.Background())
		t.Fatal("outward repair accepted")
	}
	if !reflect.DeepEqual(before, engineContractSnapshot(t, outside)) {
		t.Fatal("outside files repaired")
	}
}
func TestEngineContractRuntimeBeforeRepairAndStartFailure(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, NWJS)
	before, _ := os.ReadFile(filepath.Join(g.Path, "package.json"))
	called := false
	m.command = func(_ string, _ ...string) *exec.Cmd { called = true; return exec.Command("/does/not/exist") }
	if _, err := m.Launch(context.Background(), g, Settings{LastSelectedVersion: "v1.2.3"}); err == nil {
		t.Fatal("missing runtime accepted")
	}
	after, _ := os.ReadFile(filepath.Join(g.Path, "package.json"))
	if called || !bytes.Equal(before, after) {
		t.Fatal("repaired before runtime validation")
	}
	engineContractRuntime(t, m, NWJS)
	if _, err := m.Launch(context.Background(), g, Settings{LastSelectedVersion: "v1.2.3"}); err == nil {
		t.Fatal("failed start accepted")
	}
	if !called {
		t.Fatal("start not reached")
	}
	release, err := AcquireManagedLock(m.Root)
	if err != nil {
		t.Fatal("lock leaked", err)
	}
	release()
}
func TestEngineContractLaunchRejectsRuntimeOutwardConfig(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, MKXPZ)
	engineContractRuntime(t, m, MKXPZ)
	engineContractInstallSupport(t, m)
	outside := t.TempDir()
	engineContractWrite(t, filepath.Join(outside, "mkxp.json"), []byte(`{"sentinel":"keep"}`), 0600)
	before := engineContractSnapshot(t, outside)
	if err := os.Symlink(outside, filepath.Join(m.Root, "Z-universal.app", "Contents", "Game")); err != nil {
		t.Fatal(err)
	}
	called := false
	m.command = func(_ string, _ ...string) *exec.Cmd { called = true; return exec.Command("/usr/bin/true") }
	p, err := m.Launch(context.Background(), g, Settings{})
	if p != nil {
		_, _ = p.Wait(context.Background())
	}
	if err == nil || called || !reflect.DeepEqual(before, engineContractSnapshot(t, outside)) {
		t.Fatalf("unsafe runtime used: %v called=%v", err, called)
	}
}
func TestEngineContractLauncherExitKeepsGameAndLock(t *testing.T) {
	if root := os.Getenv("RPGM_ENGINE_WORKER_ROOT"); root != "" {
		m, err := NewManager(root, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		m.command = func(_ string, _ ...string) *exec.Cmd {
			return exec.Command("/bin/sh", "-c", "while [ ! -f \"$1\" ]; do sleep 0.02; done", "game", filepath.Join(root, "exit-gate"))
		}
		g, err := DetectGame(os.Getenv("RPGM_ENGINE_WORKER_GAME"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = m.Launch(context.Background(), g, Settings{}); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}
	m := engineContractManager(t)
	g := engineContractGame(t, EasyRPG)
	engineContractRuntime(t, m, EasyRPG)
	t.Cleanup(func() { _ = os.WriteFile(filepath.Join(m.Root, "exit-gate"), nil, 0600) })
	worker := exec.Command(os.Args[0], "-test.run=^TestEngineContractLauncherExitKeepsGameAndLock$")
	worker.Env = append(os.Environ(), "RPGM_ENGINE_WORKER_ROOT="+m.Root, "RPGM_ENGINE_WORKER_GAME="+g.Path)
	if b, err := worker.CombinedOutput(); err != nil {
		t.Fatalf("worker: %v %s", err, b)
	}
	if release, err := AcquireManagedLock(m.Root); err == nil {
		release()
		t.Fatal("launcher exit released live-game lock")
	}
	engineContractWrite(t, filepath.Join(m.Root, "exit-gate"), nil, 0600)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if release, err := AcquireManagedLock(m.Root); err == nil {
			release()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("game did not exit/release inherited lock")
}
func TestEngineContractLaunchRejectsPartialManagedResources(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, MKXPZ)
	engineContractRuntime(t, m, MKXPZ)
	engineContractWrite(t, filepath.Join(m.Root, "RTP", "Standard", "partial"), []byte("partial"), 0600)
	engineContractWrite(t, filepath.Join(m.Root, "GMGSx.SF2"), engineContractSF2(), 0600)
	engineContractWrite(t, filepath.Join(m.Root, "kawariki", "preload.rb"), []byte("partial"), 0600)
	called := false
	m.command = func(_ string, _ ...string) *exec.Cmd { called = true; return exec.Command("/usr/bin/true") }
	p, err := m.Launch(context.Background(), g, Settings{})
	if p != nil {
		_, _ = p.Wait(context.Background())
	}
	if err == nil || called {
		t.Fatal("partial resource accepted")
	}
}
func TestEngineContractMKXPCP932AndCustomPaths(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, MKXPZ)
	raw := append([]byte("[Game]\nTitle="), []byte{0x83, 0x65, 0x83, 0x58, 0x83, 0x67}...)
	raw = append(raw, []byte("\nLibrary=RGSS301.dll\n")...)
	engineContractWrite(t, filepath.Join(g.Path, "Game.ini"), raw, 0600)
	a := DefaultAdvancedSettings()
	a.MKXPZ["preloadScript"] = []any{filepath.Join(g.Path, "preload.rb")}
	a.MKXPZ["RTP"] = []any{g.Path}
	a.MKXPZ["midiSoundFont"] = filepath.Join(g.Path, "custom.sf2")
	engineContractWrite(t, filepath.Join(g.Path, "preload.rb"), []byte("# script"), 0600)
	engineContractWrite(t, filepath.Join(g.Path, "custom.sf2"), engineContractSF2(), 0600)
	c, err := m.mkxpConfig(g, a)
	if err != nil {
		t.Fatal(err)
	}
	if c["rgssVersion"] != 3 {
		t.Fatal(c)
	}
	if err = validateMKXPPaths(g, c); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(g.Path, "Game.ini"))
	if !bytes.Equal(raw, after) {
		t.Fatal("INI modified")
	}
}

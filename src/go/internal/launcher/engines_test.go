package launcher

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func engineContractWrite(t *testing.T, path string, b []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, mode); err != nil {
		t.Fatal(err)
	}
}
func engineContractARM() []byte {
	b := make([]byte, 32)
	binary.LittleEndian.PutUint32(b, 0xfeedfacf)
	binary.LittleEndian.PutUint32(b[4:], 0x0100000c)
	binary.LittleEndian.PutUint32(b[12:], 2)
	return b
}
func engineContractManager(t *testing.T) *Manager {
	t.Helper()
	base := t.TempDir()
	m, err := NewManager(filepath.Join(base, "managed"), filepath.Join(base, "resources"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"bg.js", "disable-net.js", "disable-child.js"} {
		engineContractWrite(t, filepath.Join(m.ResourcesDir, n), []byte("// "+n), 0600)
	}
	return m
}
func engineContractRuntime(t *testing.T, m *Manager, e GameEngine) {
	t.Helper()
	p := ""
	switch e {
	case NWJS:
		p = "v1.2.3/nwjs.app/Contents/MacOS/nwjs"
	case MKXPZ:
		p = "Z-universal.app/Contents/MacOS/Z-universal"
	case EasyRPG:
		p = "EasyRPG Player.app/Contents/MacOS/EasyRPG Player"
	}
	engineContractWrite(t, filepath.Join(m.Root, p), engineContractARM(), 0700)
}
func engineContractGame(t *testing.T, e GameEngine) GameInfo {
	t.Helper()
	p, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name, body := "package.json", `{"name":"","main":"index.html"}`
	if e == MKXPZ {
		name, body = "Game.ini", "[Game]\nRTP=Standard\n"
	}
	if e == EasyRPG {
		name, body = "RPG_RT.ldb", "game"
	}
	engineContractWrite(t, filepath.Join(p, name), []byte(body), 0600)
	engineContractWrite(t, filepath.Join(p, "save", "keep"), []byte("precious"), 0600)
	engineContractWrite(t, filepath.Join(p, "Game.exe"), []byte("windows"), 0600)
	for _, name := range []string{"nw.dll", "game_en.exe", "assets/English.json", "js/main.js", "js/plugins/Cheat_Menu.js"} {
		engineContractWrite(t, filepath.Join(p, name), []byte("original "+name), 0600)
	}
	if e == NWJS {
		engineContractWrite(t, filepath.Join(p, "js", "rmmz_core.js"), []byte("Bitmap.prototype.getPixel = function(x, y) {\n    return '#ffffff';\n};\n"), 0600)
	}
	g, err := DetectGame(p)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func TestEngineContractManagerRuntimeAndMissing(t *testing.T) {
	m := engineContractManager(t)
	s := Settings{LastSelectedVersion: "v1.2.3"}
	for _, e := range []GameEngine{NWJS, MKXPZ, EasyRPG} {
		g := engineContractGame(t, e)
		if _, err := m.CheckRuntime(g, s); err == nil {
			t.Fatal("missing accepted")
		}
		missing, err := m.MissingComponents(g, s)
		if err != nil || len(missing) == 0 {
			t.Fatalf("missing: %v %v", missing, err)
		}
		engineContractRuntime(t, m, e)
		if _, err := m.CheckRuntime(g, s); err != nil {
			t.Fatal(err)
		}
	}
}
func TestEngineContractInstalledNWLegacyDiscovery(t *testing.T) {
	m := engineContractManager(t)
	engineContractRuntime(t, m, NWJS)
	for _, version := range []string{"v2.0.0", "v3.0.0", "v4.0.0"} {
		body := engineContractARM()
		if version == "v3.0.0" {
			binary.LittleEndian.PutUint32(body[4:], 0x01000007)
		}
		engineContractWrite(t, filepath.Join(m.Root, version, "nwjs.app/Contents/MacOS/nwjs"), body, 0700)
	}
	engineContractWrite(t, filepath.Join(m.Root, "v4.0.0/run-with-rosetta"), nil, 0600)
	versions, err := m.InstalledNWVersions()
	if err != nil || len(versions) != 2 || versions[0] != "v2.0.0" || versions[1] != "v1.2.3" {
		t.Fatalf("%v %v", versions, err)
	}
	if _, err = m.InstallComponent(context.Background(), Component("v1.2.3"), nil); err == nil {
		t.Fatal("receiptless reinstall accepted")
	}
}
func TestEngineContractMKXPConfigTypes(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, MKXPZ)
	a := DefaultAdvancedSettings()
	a.MKXPZEnabled["fullscreen"] = true
	a.MKXPZ["fullscreen"] = false
	a.MKXPZEnabled["fixedFramerate"] = "True"
	a.MKXPZ["fixedFramerate"] = json.Number("0")
	a.MKXPZEnabled["windowTitle"] = "False"
	c, err := m.mkxpConfig(g, a)
	if err != nil {
		t.Fatal(err)
	}
	if c["gameFolder"] != g.Path || c["rgssVersion"] != 1 || c["fullscreen"] != false || c["fixedFramerate"] != json.Number("0") {
		t.Fatalf("config: %#v", c)
	}
	if _, ok := c["windowTitle"]; ok {
		t.Fatal("disabled title")
	}
	if _, ok := c["preloadScript"].([]string); !ok {
		t.Fatalf("preload type: %T", c["preloadScript"])
	}
	a.MKXPZ["RTP"] = []any{"/custom/rtp"}
	c, err = m.mkxpConfig(g, a)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(c["RTP"])
	if !bytes.Equal(b, []byte(`["/custom/rtp"]`)) {
		t.Fatal(string(b))
	}
}
func TestEngineContractMKXPAllAdvancedTypesRetained(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, MKXPZ)
	a := DefaultAdvancedSettings()
	for key := range a.MKXPZ {
		a.MKXPZEnabled[key] = true
	}
	c, err := m.mkxpConfig(g, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != len(a.MKXPZ) {
		t.Fatalf("lost settings: %d != %d", len(c), len(a.MKXPZ))
	}
	automatic := map[string]bool{"rgssVersion": true, "gameFolder": true, "RTP": true, "midiSoundFont": true, "preloadScript": true}
	for key, want := range a.MKXPZ {
		if !automatic[key] && !reflect.DeepEqual(c[key], want) {
			t.Errorf("%s: %#v != %#v", key, c[key], want)
		}
	}
	a.MKXPZ["rgssVersion"] = json.Number("0.0")
	c, err = m.mkxpConfig(g, a)
	if err != nil || c["rgssVersion"] != 1 {
		t.Fatal(c, err)
	}
}
func TestEngineContractCancelledBeforeRepair(t *testing.T) {
	m := engineContractManager(t)
	g := engineContractGame(t, NWJS)
	before, _ := os.ReadFile(filepath.Join(g.Path, "package.json"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Launch(ctx, g, Settings{LastSelectedVersion: "v1.2.3"}); err == nil {
		t.Fatal("accepted cancellation")
	}
	after, _ := os.ReadFile(filepath.Join(g.Path, "package.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("modified game")
	}
}

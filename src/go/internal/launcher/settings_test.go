package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func dataContractPut(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func dataContractRead(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestDataContractSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := DefaultRoot(home)
	s, e := LoadSettings(root)
	if e != nil {
		t.Fatal(e)
	}
	if s.OptimizeSpace || !s.PatchGetPixel || s.DisableProtection || s.LastSelectedFolder != filepath.Join(home, "Downloads") {
		t.Fatalf("defaults: %+v", s)
	}
	if _, e = os.Stat(root); !os.IsNotExist(e) {
		t.Fatalf("load wrote root: %v", e)
	}
	raw := `{"optimize_space":true,"extract_game_en":true,"add_cheat_menu":true,"last_selected_version":"v1.2.3","last_build_number":42,"future":{"keep":9007199254740993}}`
	path := filepath.Join(root, "settings.json")
	dataContractPut(t, path, []byte(raw))
	s, e = LoadSettings(root)
	if e != nil {
		t.Fatal(e)
	}
	if s.OptimizeSpace || s.LastBuildNumber != 42 {
		t.Fatal(s)
	}
	if string(dataContractRead(t, path)) != raw {
		t.Fatal("load changed settings")
	}
	s.OptimizeSpace = true
	if e = SaveSettings(root, s); e != nil {
		t.Fatal(e)
	}
	out := string(dataContractRead(t, path))
	for _, k := range []string{"extract_game_en", "add_cheat_menu"} {
		if strings.Contains(out, k) {
			t.Fatal(out)
		}
	}
	if !strings.Contains(out, "9007199254740993") {
		t.Fatal(out)
	}
	got, e := LoadSettings(root)
	if e != nil || got.OptimizeSpace || got.LastSelectedVersion != "v1.2.3" {
		t.Fatalf("%+v %v", got, e)
	}
	for _, bad := range []string{`{bad`, `[]`, `null`, `{"patch_getpixel":"true"}`, `{"last_build_number":null}`} {
		dataContractPut(t, path, []byte(bad))
		if _, e = LoadSettings(root); e == nil {
			t.Fatalf("accepted %s", bad)
		}
		if e = SaveSettings(root, s); e == nil {
			t.Fatalf("overwrote invalid %s", bad)
		}
		if string(dataContractRead(t, path)) != bad {
			t.Fatal("destroyed invalid JSON")
		}
	}
}
func TestDataContractAdvancedRoundTrip(t *testing.T) {
	root := t.TempDir()
	a, e := LoadAdvancedSettings(root)
	if e != nil {
		t.Fatal(e)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("advanced load wrote files", err)
	}
	if len(a.MKXPZ) != 61 || len(a.MKXPZEnabled) != 61 || len(a.EasyRPG) != 56 || len(a.EasyRPGEnabled) != 55 {
		t.Fatalf("default counts %d %d %d %d", len(a.MKXPZ), len(a.MKXPZEnabled), len(a.EasyRPG), len(a.EasyRPGEnabled))
	}
	if !SettingEnabled(a.MKXPZEnabled["rgssVersion"]) || SettingEnabled(a.MKXPZEnabled["displayFPS"]) {
		t.Fatal("enabled defaults")
	}
	for _, v := range []any{true, "True", "Force"} {
		if !SettingEnabled(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []any{false, "False", "true", 1, nil} {
		if SettingEnabled(v) {
			t.Fatal(v)
		}
	}
	dataContractPut(t, filepath.Join(root, "mkxpz-advanced.json"), []byte(`{"windowTitle":"0012","customScript":"false","dataPathOrg":"[1]","future":{"big":9007199254740993},"textureScalingFactor":4.0,"solidFonts":["12","false"]}`))
	dataContractPut(t, filepath.Join(root, "enabled-mkxpz-settings.json"), []byte(`{"displayFPS":true,"fullscreen":"False","future":"Force"}`))
	a, e = LoadAdvancedSettings(root)
	if e != nil {
		t.Fatal(e)
	}
	if a.MKXPZ["windowTitle"] != "0012" || a.MKXPZ["textureScalingFactor"] != json.Number("4.0") {
		t.Fatal(a.MKXPZ)
	}
	if e = SaveAdvancedSettings(root, a); e != nil {
		t.Fatal(e)
	}
	b, e := LoadAdvancedSettings(root)
	if e != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("roundtrip %v", e)
	}
	fresh := DefaultAdvancedSettings()
	fresh.MKXPZ["windowTitle"] = "changed"
	if DefaultAdvancedSettings().MKXPZ["windowTitle"] != "Custom Title" {
		t.Fatal("shared mutable defaults")
	}
	badPath := filepath.Join(root, "easyrpg-advanced.json")
	dataContractPut(t, badPath, []byte("{bad"))
	before := dataContractRead(t, filepath.Join(root, "mkxpz-advanced.json"))
	if _, e = LoadAdvancedSettings(root); e == nil {
		t.Fatal("accepted corrupt JSON")
	}
	a.MKXPZ["windowTitle"] = "new"
	if e = SaveAdvancedSettings(root, a); e == nil {
		t.Fatal("saved over corruption")
	}
	if string(before) != string(dataContractRead(t, filepath.Join(root, "mkxpz-advanced.json"))) {
		t.Fatal("partial validation writes")
	}
}
func TestDataContractAdvancedDefaultTypes(t *testing.T) {
	a := DefaultAdvancedSettings()
	groups := []struct {
		values map[string]any
		keys   string
		kind   string
	}{
		{a.MKXPZ, "rgssVersion smoothScaling smoothScalingDown bitmapSmoothScaling bitmapSmoothScalingDown bicubicSharpness defScreenW defScreenH fixedFramerate maxTextureSize JITVerboseLevel JITMaxCache JITMinCalls SESourceCount BGMTrackCount", "number"},
		{a.MKXPZ, "xbrzScalingFactor textureScalingFactor framebufferScalingFactor atlasScalingFactor", "float"},
		{a.MKXPZ, "windowTitle gameFolder dataPathOrg dataPathApp iconPath customScript midiSoundFont execName", "string"},
		{a.MKXPZ, "solidFonts preloadScript postloadScript RTP patches fontSub rubyLoadpath", "array"},
		{a.EasyRPG, "--font1-size --font2-size --patch-maniac --fps-limit --music-volume --sound-volume", "number"},
	}
	for _, group := range groups {
		for _, key := range strings.Fields(group.keys) {
			v, ok := group.values[key]
			if !ok {
				t.Fatal("missing default", key)
			}
			switch group.kind {
			case "number", "float":
				n, ok := v.(json.Number)
				if !ok {
					t.Fatalf("%s: %T", key, v)
				}
				if group.kind == "float" && !strings.Contains(string(n), ".") {
					t.Fatal("lost float default", key, n)
				}
			case "string":
				if _, ok := v.(string); !ok {
					t.Fatalf("%s: %T", key, v)
				}
			case "array":
				if _, ok := v.([]any); !ok {
					t.Fatalf("%s: %T", key, v)
				}
			}
		}
	}
	if binding, ok := a.MKXPZ["bindingNames"].(map[string]any); !ok || binding["c"] != "Confirm" || binding["b"] != "Cancel" {
		t.Fatal("bindingNames default")
	}
	if a.MKXPZ["rgssVersion"] != json.Number("0") || a.MKXPZ["defScreenW"] != json.Number("640") || a.MKXPZ["defScreenH"] != json.Number("480") || a.EasyRPG["--fps-limit"] != json.Number("60") || a.EasyRPG["--vsync"] != true {
		t.Fatal("engine defaults")
	}
}

func TestDataContractAtomicAndSymlink(t *testing.T) {
	root := t.TempDir()
	s := DefaultSettings(root)
	if e := SaveSettings(root, s); e != nil {
		t.Fatal(e)
	}
	old, e := os.Open(filepath.Join(root, "settings.json"))
	if e != nil {
		t.Fatal(e)
	}
	defer old.Close()
	info, _ := old.Stat()
	s.LastCommitSHA = "new"
	if e = SaveSettings(root, s); e != nil {
		t.Fatal(e)
	}
	now, _ := os.Stat(filepath.Join(root, "settings.json"))
	if os.SameFile(info, now) {
		t.Fatal("not atomic rename")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("temporary file leak")
	}
	outside := filepath.Join(t.TempDir(), "valuable")
	dataContractPut(t, outside, []byte(`{}`))
	os.Remove(filepath.Join(root, "settings.json"))
	if e = os.Symlink(outside, filepath.Join(root, "settings.json")); e != nil {
		t.Fatal(e)
	}
	if e = SaveSettings(root, s); e == nil {
		t.Fatal("followed symlink")
	}
	if string(dataContractRead(t, outside)) != "{}" {
		t.Fatal("modified target")
	}
}

package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dataContractAssets(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	for _, n := range []string{"bg.js", "disable-child.js", "disable-net.js"} {
		dataContractPut(t, filepath.Join(p, n), []byte("// launcher "+n))
	}
	return p
}
func TestDataContractDetectReadOnlyAndRTP(t *testing.T) {
	p := t.TempDir()
	dataContractPut(t, filepath.Join(p, "RPG_RT.exe"), []byte("MZ"))
	g, e := DetectGame(p)
	if e != nil || g.Engine != EasyRPG {
		t.Fatalf("%+v %v", g, e)
	}
	ini := filepath.Join(p, "Game.ini")
	dataContractPut(t, ini, []byte("[Game]\nRTP=RPGVXace\nLibrary=RGSS102E.dll\n"))
	g, e = DetectGame(p)
	if e != nil || g.Engine != MKXPZ || g.RGSSVersion != 3 || g.RTP != "RPGVXace" {
		t.Fatalf("%+v %v", g, e)
	}
	pkg := filepath.Join(p, "package.json")
	dataContractPut(t, pkg, []byte(`{"name":"valuable","bg-script":"custom.js"}`))
	before := dataContractRead(t, pkg)
	g, e = DetectGame(p)
	if e != nil || g.Engine != NWJS {
		t.Fatalf("%+v %v", g, e)
	}
	if string(before) != string(dataContractRead(t, pkg)) {
		t.Fatal("detection wrote package")
	}
	for _, tc := range []struct {
		ini  string
		want int
	}{{"rtp = Standard\n", 1}, {"RTP = rPgVx\nLibrary=RGSS301.dll", 2}, {"RTP=unknown\nlibrary=RGSS301.dll", 3}, {"Library=RGSS999.dll", 1}, {"", 1}} {
		dataContractPut(t, ini, []byte(tc.ini))
		_, v, e := GetRTPValue(p)
		if e != nil || v != tc.want {
			t.Fatalf("%q %d %v", tc.ini, v, e)
		}
	}
	dataContractPut(t, filepath.Join(p, "System", "RGSS202E.dll"), []byte("never execute"))
	_, v, e := GetRTPValue(p)
	if e != nil || v != 2 {
		t.Fatalf("System DLL %d %v", v, e)
	}
	dataContractPut(t, filepath.Join(p, "RGSS301.dll"), nil)
	_, v, e = GetRTPValue(p)
	if e != nil || v != 3 {
		t.Fatalf("root precedence %d %v", v, e)
	}
	// CP932 日本語; invalid UTF-8 must be decoded, not replaced.
	cp := append([]byte("[Game]\nTitle="), 0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea)
	cp = append(cp, []byte("\nRTP=RPGVX\n")...)
	dataContractPut(t, ini, cp)
	_, v, e = GetRTPValue(p)
	if e != nil || v != 2 {
		t.Fatalf("CP932 %d %v", v, e)
	}
	if string(dataContractRead(t, ini)) != string(cp) {
		t.Fatal("INI changed")
	}
	dataContractPut(t, ini, []byte{0x81})
	if _, _, e = GetRTPValue(p); e == nil {
		t.Fatal("invalid encoding silently accepted")
	}
	if _, e = DetectGame(t.TempDir()); e == nil {
		t.Fatal("unknown game accepted")
	}
}
func TestDataContractPrepareSafety(t *testing.T) {
	assets := dataContractAssets(t)
	for _, name := range []string{"", " ", "valuable", "../../valuable", "/absolute"} {
		t.Run(name, func(t *testing.T) {
			p := t.TempDir()
			path := filepath.Join(p, "package.json")
			raw, _ := json.Marshal(map[string]any{"name": name})
			dataContractPut(t, path, raw)
			sentinels := map[string][]byte{"game_en.exe": {0, 255}, "save/file1.rmmzsave": []byte("valuable"), "js/main.js": []byte("Cheat_Menu.js"), "js/plugins/Cheat_Menu.js": []byte("existing"), "locales/user-data": []byte("keep")}
			for n, b := range sentinels {
				dataContractPut(t, filepath.Join(p, n), b)
			}
			r, e := PreparePackage(p, assets, false)
			if e != nil || !r.OK {
				t.Fatalf("%+v %v", r, e)
			}
			var obj map[string]any
			json.Unmarshal(dataContractRead(t, path), &obj)
			want := name
			if strings.TrimSpace(want) == "" {
				want = "tempname"
			}
			if obj["name"] != want || obj["bg-script"] != "bg.js" {
				t.Fatal(obj)
			}
			r, e = PreparePackage(p, assets, true)
			if e != nil || !r.OK {
				t.Fatal(e)
			}
			json.Unmarshal(dataContractRead(t, path), &obj)
			var removed map[string]any
			json.Unmarshal(dataContractRead(t, path), &removed)
			if _, ok := removed["bg-script"]; ok {
				t.Fatal("known protection not removed")
			}
			for n, b := range sentinels {
				if string(dataContractRead(t, filepath.Join(p, n))) != string(b) {
					t.Fatal(n)
				}
			}
		})
	}
	for _, bg := range []string{"custom.js", "bg.js"} {
		for _, disable := range []bool{false, true} {
			p := t.TempDir()
			path := filepath.Join(p, "package.json")
			dataContractPut(t, path, []byte(`{"name":"game","bg-script":"`+bg+`"}`))
			dataContractPut(t, filepath.Join(p, bg), []byte("// user"))
			before := dataContractRead(t, path)
			r, e := PreparePackage(p, assets, disable)
			if e != nil || !r.OK || (!disable && len(r.Warnings) == 0) {
				t.Fatalf("%+v %v", r, e)
			}
			if string(before) != string(dataContractRead(t, path)) {
				t.Fatal("custom bg overwritten")
			}
		}
	}
	for _, bad := range []string{"{bad", "[]", `{"name":4}`} {
		p := t.TempDir()
		path := filepath.Join(p, "package.json")
		dataContractPut(t, path, []byte(bad))
		if _, e := PreparePackage(p, assets, false); e == nil {
			t.Fatal(bad)
		}
		if string(dataContractRead(t, path)) != bad {
			t.Fatal("rewrote invalid package")
		}
	}
	p := t.TempDir()
	path := filepath.Join(p, "package.json")
	dataContractPut(t, path, []byte(`{"name":""}`))
	dataContractPut(t, filepath.Join(p, "disable-net.js"), []byte("// user"))
	before := dataContractRead(t, path)
	if _, e := PreparePackage(p, assets, false); e == nil {
		t.Fatal("collision accepted")
	}
	if string(before) != string(dataContractRead(t, path)) {
		t.Fatal("preflight modified package")
	}
	if _, e := os.Stat(filepath.Join(p, "bg.js")); !os.IsNotExist(e) {
		t.Fatal("preflight created script")
	}
	os.Remove(filepath.Join(p, "disable-net.js"))
	os.Symlink(filepath.Join(assets, "bg.js"), filepath.Join(p, "bg.js"))
	if _, e := PreparePackage(p, assets, false); e == nil {
		t.Fatal("symlink collision accepted")
	}
}

// Ports the MV/MZ x injected-assets x legacy-toggle fixtures from test_safety.py.
func TestDataContractCopyPreparationPreservesAssets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	assets := dataContractAssets(t)
	profiles := []string{filepath.Join(home, "Library/Application Support/nwjs/save"), filepath.Join(home, "Library/Application Support/valuable/save.dat")}
	for _, p := range profiles {
		dataContractPut(t, p, []byte("valuable profile"))
	}
	for _, engine := range []string{"MV", "MZ"} {
		for _, injected := range []bool{false, true} {
			for _, legacy := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/injected=%t/legacy=%t", engine, injected, legacy), func(t *testing.T) {
					p := t.TempDir()
					prefix := ""
					core := "rmmz_core.js"
					main := []byte("const scriptUrls = [\"js/rmmz_managers.js\"];\n")
					if engine == "MV" {
						prefix = "www/"
						core = "rpg_core.js"
						main = []byte("PluginManager.setup($plugins);\n")
					}
					if injected {
						main = append(main, []byte("PluginManager.loadScript('Cheat_Menu.js');\n")...)
					}
					files := map[string][]byte{"package.json": []byte(`{"name":"","main":"index.html"}`), prefix + "js/main.js": main, "game_en.exe": {0, 255}, "nested/GAME_EN.EXE": []byte("original"), prefix + "data/English.json": []byte(`{"text":"Keep English assets"}`), prefix + "save/file1.rmmzsave": []byte("original save"), prefix + "js/" + core: []byte("Bitmap.prototype.getPixel = function(x, y) {\n    return '#ffffff';\n};\n")}
					if injected {
						files[prefix+"js/plugins/Cheat_Menu.js"] = []byte("existing cheat")
						files[prefix+"js/plugins/Cheat_Menu.css"] = []byte("existing css")
					}
					for n, b := range files {
						dataContractPut(t, filepath.Join(p, n), b)
					}
					root := t.TempDir()
					dataContractPut(t, filepath.Join(root, "settings.json"), []byte(fmt.Sprintf(`{"extract_game_en":%t,"add_cheat_menu":%t,"optimize_space":true,"patch_getpixel":true}`, legacy, legacy)))
					settings, e := LoadSettings(root)
					if e != nil || settings.OptimizeSpace {
						t.Fatal(settings, e)
					}
					if _, e = DetectGame(p); e != nil {
						t.Fatal(e)
					}
					for n, b := range files {
						if string(dataContractRead(t, filepath.Join(p, n))) != string(b) {
							t.Fatal("selection changed", n)
						}
					}
					// The export caller supplies a copy; preparing it must not mutate the original.
					copyDir := filepath.Join(t.TempDir(), "app.nw")
					if e = os.CopyFS(copyDir, os.DirFS(p)); e != nil {
						t.Fatal(e)
					}
					if _, e = PreparePackage(copyDir, assets, false); e != nil {
						t.Fatal(e)
					}
					for n, b := range files {
						if string(dataContractRead(t, filepath.Join(p, n))) != string(b) {
							t.Fatal("copy prep changed original", n)
						}
						if n != "package.json" && string(dataContractRead(t, filepath.Join(copyDir, n))) != string(b) {
							t.Fatal("copy asset changed", n)
						}
					}
					if _, e = PreparePackage(p, assets, false); e != nil {
						t.Fatal(e)
					}
					if _, e = PatchGetPixel(p, settings.PatchGetPixel); e != nil {
						t.Fatal(e)
					}
					for n, b := range files {
						if n == "package.json" || n == prefix+"js/"+core {
							continue
						}
						if string(dataContractRead(t, filepath.Join(p, n))) != string(b) {
							t.Fatal("asset changed", n)
						}
					}
					for _, p := range profiles {
						if string(dataContractRead(t, p)) != "valuable profile" {
							t.Fatal("shared profile changed")
						}
					}
				})
			}
		}
	}
}

func TestDataContractDecodeAmbiguousBytes(t *testing.T) {
	for _, tc := range []struct {
		raw      []byte
		encoding string
		want     string
	}{
		{[]byte{0xa9}, "CP932", "ｩ"},
		{[]byte{0xc2, 0xa9}, "UTF-8", "©"},
	} {
		text, encoding, err := dataContractDecode(tc.raw)
		if err != nil || encoding != tc.encoding || text != tc.want {
			t.Fatalf("%x: %q %q %v", tc.raw, text, encoding, err)
		}
	}
}

func TestDataContractPackageUTF8BOM(t *testing.T) {
	p := t.TempDir()
	path := filepath.Join(p, "package.json")
	dataContractPut(t, path, []byte("\ufeff{\"title\":\"日本語\"}"))
	if _, err := PreparePackage(p, dataContractAssets(t), true); err != nil {
		t.Fatal(err)
	}
	out := dataContractRead(t, path)
	if !strings.HasPrefix(string(out), "\ufeff") {
		t.Fatal("UTF-8 BOM lost")
	}
	obj, err := dataContractObject([]byte(strings.TrimPrefix(string(out), "\ufeff")))
	if err != nil || obj["title"] != "日本語" || obj["name"] != "tempname" {
		t.Fatalf("%#v %v", obj, err)
	}
}

func TestDataContractPackageCP932AndMissingName(t *testing.T) {
	p := t.TempDir()
	assets := dataContractAssets(t)
	path := filepath.Join(p, "package.json")
	raw := append([]byte(`{"title":"`), 0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea)
	raw = append(raw, []byte(`"}`)...)
	dataContractPut(t, path, raw)
	if _, e := PreparePackage(p, assets, false); e != nil {
		t.Fatal(e)
	}
	text, _, e := dataContractDecode(dataContractRead(t, path))
	if e != nil || !strings.Contains(text, "日本語") || !strings.Contains(text, "tempname") {
		t.Fatalf("%s %v", text, e)
	}
}
func TestDataContractGetPixel(t *testing.T) {
	for _, prefix := range []string{"js", "www/js"} {
		for _, core := range []string{"rpg_core.js", "rmmz_core.js"} {
			for _, nl := range []string{"\n", "\r\n"} {
				t.Run(prefix+core+nl, func(t *testing.T) {
					p := t.TempDir()
					path := filepath.Join(p, prefix, core)
					original := strings.ReplaceAll("Bitmap.prototype.getPixel = function(x, y) {\n    const text = '}'; // }\n    /* } { */\n    return '#ffffff';\n};\n", "\n", nl)
					dataContractPut(t, path, []byte(original))
					r, e := PatchGetPixel(p, true)
					if e != nil || !r.OK || !r.Changed || !r.Patched {
						t.Fatalf("%+v %v", r, e)
					}
					patched := dataContractRead(t, path)
					r, e = PatchGetPixel(p, true)
					if e != nil || r.Changed || !r.Patched {
						t.Fatal(r, e)
					}
					if string(patched) != string(dataContractRead(t, path)) {
						t.Fatal("reapply changed")
					}
					r, e = PatchGetPixel(p, false)
					if e != nil || !r.Changed || r.Patched {
						t.Fatal(r, e)
					}
					if string(dataContractRead(t, path)) != original {
						t.Fatal("unpatch must restore exact bytes")
					}
					r, e = PatchGetPixel(p, false)
					if e != nil || r.Changed {
						t.Fatal(r, e)
					}
				})
			}
		}
	}
}
func TestDataContractGetPixelQuotedMarkers(t *testing.T) {
	p := t.TempDir()
	path := filepath.Join(p, "js/rpg_core.js")
	original := "Bitmap.prototype.getPixel = function(x, y) {\n    const example = '/* RPGM-Launcher getPixel finite patch begin */';\n    return '#ffffff';\n};\n"
	dataContractPut(t, path, []byte(original))
	if r, e := PatchGetPixel(p, true); e != nil || !r.Changed {
		t.Fatal(r, e)
	}
	if _, e := PatchGetPixel(p, false); e != nil {
		t.Fatal(e)
	}
	if string(dataContractRead(t, path)) != original {
		t.Fatal("quoted marker changed")
	}
}

func TestDataContractGetPixelLegacyAndLexing(t *testing.T) {
	p := t.TempDir()
	path := filepath.Join(p, "js/rmmz_core.js")
	sig := "Bitmap.prototype.getPixel = function (x, y) {\n"
	body := "    return '#ffffff';\n};\n"
	legacy := "    if (!Number.isFinite(x) || !Number.isFinite(y)) {\n        return '#000000';\n    }\n"
	dataContractPut(t, path, []byte(sig+legacy+body))
	r, e := PatchGetPixel(p, true)
	if e != nil || r.Changed || !r.Patched {
		t.Fatal(r, e)
	}
	r, e = PatchGetPixel(p, false)
	if e != nil || !r.Changed || string(dataContractRead(t, path)) != sig+body {
		t.Fatal(r, e)
	}
	// Strings/comments outside the real function must not count as signatures or patches.
	decoy := "/*\n" + sig + "*/\nconst example = `\n" + sig + "`;\n"
	original := decoy + sig + "    const braces = /[{}]/;\n" + body
	dataContractPut(t, path, []byte(original))
	r, e = PatchGetPixel(p, true)
	if e != nil || !r.Changed {
		t.Fatal(r, e)
	}
	if !strings.HasPrefix(string(dataContractRead(t, path)), decoy) {
		t.Fatal("patched comment/string")
	}
	_, e = PatchGetPixel(p, false)
	if e != nil || string(dataContractRead(t, path)) != original {
		t.Fatal("lexing roundtrip", e)
	}
	dataContractPut(t, filepath.Join(p, "js/rpg_core.js"), []byte(sig+body))
	r, e = PatchGetPixel(p, true)
	if e != nil || r.JSFile != path {
		t.Fatal("MZ precedence", r, e)
	}
	for _, bad := range []string{sig + " // missing brace\n", sig + "    /* RPGM-Launcher getPixel finite patch begin */\n" + body, string([]byte{0xff}), "/*\n" + sig + body + "*/"} {
		dataContractPut(t, path, []byte(bad))
		if _, e = PatchGetPixel(p, true); e == nil {
			t.Fatal("unsafe JS accepted", bad)
		}
		if string(dataContractRead(t, path)) != bad {
			t.Fatal("unsafe JS changed")
		}
	}
}

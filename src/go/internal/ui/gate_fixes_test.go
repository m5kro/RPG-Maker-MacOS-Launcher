package ui

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"rpgm-launcher/nwjs-helper/internal/launcher"
)

func TestResetLegacyTypesExplicitIntent(t *testing.T) {
	for _, engine := range []string{"MKXPZ", "EasyRPG"} {
		t.Run(engine, func(t *testing.T) {
			u := newTestUI(t)
			a := launcher.DefaultAdvancedSettings()
			values, enabled, _ := formMaps(a, engine)
			key, number := "windowTitle", "1984"
			if engine == "EasyRPG" {
				key, number = "--seed", "123"
			}
			values[key] = json.Number(number)
			values["future"] = map[string]any{"huge": json.Number("9007199254740993123456789"), "string": "00123"}
			enabled[key], enabled["future"] = "Force", "True"
			if engine == "EasyRPG" {
				values["--project-path"] = "/日本語/game"
			}
			if err := launcher.SaveAdvancedSettings(u.manager.Root, a); err != nil {
				t.Fatal(err)
			}
			original, err := u.Advanced(engine, false)
			if err != nil {
				t.Fatal(err)
			}
			if err = u.SaveAdvanced(engine, original); err != nil {
				t.Fatal(err)
			}
			again, _ := u.Advanced(engine, false)
			if !reflect.DeepEqual(original, again) {
				t.Fatal("ordinary roundtrip changed mixed JSON")
			}
			reset, err := u.Advanced(engine, true)
			if err != nil {
				t.Fatal(err)
			}
			// Reset is preview-only, not a disk write.
			again, _ = u.Advanced(engine, false)
			if !reflect.DeepEqual(original, again) {
				t.Fatal("preview wrote settings")
			}
			for _, f := range reset {
				if f.Key == "future" || f.ReadOnly {
					if f.Reset {
						t.Errorf("non-resettable field marked: %s", f.Key)
					}
					for _, old := range original {
						if old.Key == f.Key && old != f {
							t.Errorf("reset changed preserved field: %s", f.Key)
						}
					}
				} else if !f.Reset {
					t.Errorf("missing explicit Reset intent: %s", f.Key)
				}
			}
			noIntent := append([]Field(nil), reset...)
			for i := range noIntent {
				noIntent[i].Reset = false
			}
			if err = u.SaveAdvanced(engine, noIntent); err == nil {
				t.Fatal("ordinary Save changed legacy type")
			}
			if err = u.SaveAdvanced(engine, reset); err != nil {
				t.Fatalf("Reset -> Save: %v", err)
			}
			loaded, err := launcher.LoadAdvancedSettings(u.manager.Root)
			if err != nil {
				t.Fatal(err)
			}
			got, on, _ := formMaps(loaded, engine)
			defaults, _, _ := formMaps(launcher.DefaultAdvancedSettings(), engine)
			if got[key] != defaults[key] || on[key] != "Force" || raw(got["future"]) != raw(values["future"]) || on["future"] != "True" {
				t.Fatal("reset lost defaults/Force/unknown", got, on)
			}
			for _, f := range reset {
				if f.Key == "future" || f.ReadOnly {
					f.Reset = true
					if err = u.SaveAdvanced(engine, []Field{f}); err == nil {
						t.Errorf("accepted Reset on %s", f.Key)
					}
				}
				if f.Key == key {
					f.Type, f.ValueJSON = "number", "999"
					if err = u.SaveAdvanced(engine, []Field{f}); err == nil {
						t.Fatal("Reset accepted non-default type")
					}
					f.Type, f.ValueJSON, f.EnabledJSON = "string", `"changed"`, "false"
					if err = u.SaveAdvanced(engine, []Field{f}); err == nil {
						t.Fatal("Reset bypassed Force")
					}
				}
			}
		})
	}
}

func TestNWReceiptInstallDiscoveryController(t *testing.T) {
	for _, tc := range []struct {
		name, receipt, marker string
		discover, reuse       bool
	}{
		{"false", `{"version":"v0.94.1","arch":"arm64","rosetta":false}`, "", true, true},
		{"omitted-rosetta", `{"version":"v0.94.1","arch":"arm64"}`, "", true, true},
		{"receiptless", "", "", true, false},
		{"null", `{"version":"v0.94.1","arch":"arm64","rosetta":null}`, "", false, false},
		{"string", `{"version":"v0.94.1","arch":"arm64","rosetta":"false"}`, "", false, false},
		{"number", `{"version":"v0.94.1","arch":"arm64","rosetta":0}`, "", false, false},
		{"true", `{"version":"v0.94.1","arch":"arm64","rosetta":true}`, "", false, false},
		{"missing-version", `{"arch":"arm64","rosetta":false}`, "", false, false},
		{"wrong-version", `{"version":"v0.94.2","arch":"arm64","rosetta":false}`, "", false, false},
		{"missing-arch", `{"version":"v0.94.1","rosetta":false}`, "", false, false},
		{"x64", `{"version":"v0.94.1","arch":"x64","rosetta":false}`, "", false, false},
		{"marker", `{"version":"v0.94.1","arch":"arm64","rosetta":false}`, "file", false, false},
		{"dangling-marker", "", "symlink", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := newTestUI(t)
			version := filepath.Join(u.manager.Root, "v0.94.1")
			exe := filepath.Join(version, "nwjs.app/Contents/MacOS/nwjs")
			if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
				t.Fatal(err)
			}
			arm := make([]byte, 32)
			binary.LittleEndian.PutUint32(arm, 0xfeedfacf)
			binary.LittleEndian.PutUint32(arm[4:], 0x0100000c)
			binary.LittleEndian.PutUint32(arm[12:], 2)
			if err := os.WriteFile(exe, arm, 0700); err != nil {
				t.Fatal(err)
			}
			if tc.receipt != "" {
				if err := os.WriteFile(filepath.Join(version, ".nwjs-install.json"), []byte(tc.receipt), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.marker == "file" {
				if err := os.WriteFile(filepath.Join(version, "run-with-rosetta"), []byte("legacy"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.marker == "symlink" {
				if err := os.Symlink("missing", filepath.Join(version, "run-with-rosetta")); err != nil {
					t.Fatal(err)
				}
			}
			before := uiTree(t, version)
			if _, err := launcher.RequireNWArm(version); (err == nil) != tc.discover {
				t.Fatalf("runtime: %v", err)
			}
			snap, err := u.Snapshot()
			if err != nil || (len(snap.Installed) == 1) != tc.discover {
				t.Fatalf("discovery: %+v %v", snap, err)
			}
			progress := 0
			path, err := launcher.InstallNW(t.Context(), u.manager.Root, "v0.94.1", func(launcher.Progress) error { progress++; return nil })
			if (err == nil) != tc.reuse || (!tc.reuse && path != "") || progress != 0 {
				t.Errorf("InstallNW: path=%s progress=%d error=%v", path, progress, err)
			}
			result, err := u.Run(Request{Action: "install", Component: "v0.94.1", Approved: true})
			if (err == nil) != tc.reuse || (!tc.reuse && result.Path != "") {
				t.Errorf("controller false success: %+v %v", result, err)
			}
			logs, _ := u.Logs()
			if strings.Contains(logs, "導入完了:") != tc.reuse {
				t.Errorf("success notification: %s", logs)
			}
			after, err := u.Snapshot()
			if err != nil || !reflect.DeepEqual(snap.Installed, after.Installed) {
				t.Fatal("installed list changed", after, err)
			}
			if !reflect.DeepEqual(before, uiTree(t, version)) {
				t.Fatal("original runtime changed")
			}
			if !tc.reuse {
				if _, err := os.Lstat(filepath.Join(u.manager.Root, "settings.json")); !os.IsNotExist(err) {
					t.Fatal("failed install wrote settings", err)
				}
			}
		})
	}
}

func uiTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		tree[rel] = info.Mode().String()
		if d.Type().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			tree[rel] += string(b)
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			tree[rel] += link
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

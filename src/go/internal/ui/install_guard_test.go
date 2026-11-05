package ui

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rpgm-launcher/nwjs-helper/internal/launcher"
)

func TestInstallNWRejectsBackendFalseSuccess(t *testing.T) {
	for _, fault := range []string{"pathwrong", "missing", "Intel", "invalidreceipt", "receiptless", "not-discovered", "discovery-error", "valid"} {
		t.Run(fault, func(t *testing.T) {
			u := newTestUI(t)
			version := "v0.94.1"
			root := filepath.Join(u.manager.Root, version)
			bundle := filepath.Join(root, "nwjs.app")
			exe := filepath.Join(bundle, "Contents/MacOS/nwjs")
			if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
				t.Fatal(err)
			}
			arm := make([]byte, 32)
			binary.LittleEndian.PutUint32(arm, 0xfeedfacf)
			binary.LittleEndian.PutUint32(arm[4:], 0x0100000c)
			binary.LittleEndian.PutUint32(arm[12:], 2)
			if fault == "Intel" {
				binary.LittleEndian.PutUint32(arm[4:], 0x01000007)
			}
			if fault != "missing" {
				if err := os.WriteFile(exe, arm, 0700); err != nil {
					t.Fatal(err)
				}
			}
			receipt := `{"version":"v0.94.1","arch":"arm64","rosetta":false}`
			if fault == "invalidreceipt" {
				receipt = `{"version":"v0.94.1","arch":"arm64","rosetta":null}`
			}
			if fault != "receiptless" {
				if err := os.WriteFile(filepath.Join(root, ".nwjs-install.json"), []byte(receipt), 0600); err != nil {
					t.Fatal(err)
				}
			}
			old := launcher.DefaultSettings(u.manager.Root)
			old.LastSelectedVersion, old.LastSelectedFolder = "v0.93.0", t.TempDir()
			old.DisableProtection, old.PatchGetPixel = true, true
			old.LastCommitSHA, old.LastBuildNumber = "old-commit", 42
			if err := launcher.SaveSettings(u.manager.Root, old); err != nil {
				t.Fatal(err)
			}
			settings := filepath.Join(u.manager.Root, "settings.json")
			before, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			tree := uiTree(t, root)
			calls, successEvents := 0, 0
			u.installComponent = func(ctx context.Context, c launcher.Component, p launcher.ProgressFunc) (launcher.InstallResult, error) {
				calls++
				if c != launcher.Component(version) || ctx.Err() != nil {
					t.Fatal(c, ctx.Err())
				}
				path := bundle
				if fault == "pathwrong" {
					path = filepath.Join(u.manager.Root, "v0.95.0/nwjs.app")
				}
				return launcher.InstallResult{Path: path, CommitSHA: "new-commit", BuildNumber: 999}, nil
			}
			if fault == "not-discovered" {
				u.installedNWVersions = func() ([]string, error) { return []string{"v0.93.0"}, nil }
			}
			if fault == "discovery-error" {
				u.installedNWVersions = func() ([]string, error) { return nil, errors.New("discovery failed") }
			}
			u.emit = func(name string, _ any) {
				if name == "log" && strings.Contains(u.logs.String(), "導入完了:") {
					successEvents++
				}
			}
			result, err := u.Run(Request{Action: "install", Component: version, Approved: true})
			if calls != 1 {
				t.Fatalf("backend calls = %d", calls)
			}
			if fault == "valid" {
				saved, loadErr := launcher.LoadSettings(u.manager.Root)
				if err != nil || loadErr != nil || result.Path != bundle || successEvents != 1 || saved.LastSelectedVersion != version || saved.LastCommitSHA != "new-commit" || saved.LastBuildNumber != 999 || saved.LastSelectedFolder != old.LastSelectedFolder || !saved.DisableProtection || !saved.PatchGetPixel {
					t.Fatalf("valid install: %+v %v saved=%+v %v events=%d", result, err, saved, loadErr, successEvents)
				}
			} else {
				after, readErr := os.ReadFile(settings)
				saved, loadErr := launcher.LoadSettings(u.manager.Root)
				if err == nil || result.Path != "" || successEvents != 0 || strings.Contains(u.logs.String(), "導入完了:") || readErr != nil || string(before) != string(after) || loadErr != nil || saved.LastSelectedVersion != old.LastSelectedVersion {
					t.Errorf("false success: result=%+v err=%v events=%d settingsPreserved=%v selection=%s", result, err, successEvents, string(before) == string(after), saved.LastSelectedVersion)
				}
			}
			if u.State().Busy {
				t.Error("still busy")
			}
			afterTree := uiTree(t, root)
			for path, content := range tree {
				if afterTree[path] != content {
					t.Fatalf("runtime changed: %s", path)
				}
			}
		})
	}
}

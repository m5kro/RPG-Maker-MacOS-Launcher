package launcher

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type engineContractTransport func(*http.Request) (*http.Response, error)

func (f engineContractTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func engineContractSF2() []byte {
	b := []byte("RIFF\x00\x00\x00\x00sfbk")
	for _, k := range []string{"INFO", "sdta", "pdta"} {
		b = append(b, []byte("LIST\x04\x00\x00\x00"+k)...)
	}
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	return b
}
func engineContractArchive(t *testing.T, c Component, bad bool) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string][]byte{}
	n := componentName(c)
	switch c {
	case ComponentMKXPZ:
		files[n+"/Contents/MacOS/Z-universal"] = engineContractARM()
		files[n+"/Contents/Game/mkxp.json"] = []byte(`{}`)
	case ComponentEasyRPG:
		files[n+"/Contents/MacOS/EasyRPG Player"] = engineContractARM()
	case ComponentRTP:
		for _, d := range []string{"Standard", "RPGVX", "RPGVXace"} {
			files[n+"/"+d+"/Graphics/tile"] = []byte("tiles")
		}
	case ComponentKawariki:
		files[n+"/preload.rb"] = []byte("require_relative 'patch'")
		files[n+"/patch.rb"] = []byte("# patch")
	}
	for n, data := range files {
		if bad {
			data = []byte("invalid")
		}
		h := &zip.FileHeader{Name: n, Method: zip.Deflate}
		h.SetMode(0755)
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func engineContractHTTP(t *testing.T, m *Manager, payload []byte, mode string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "commits/dev") {
			fmt.Fprint(w, `{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
			return
		}
		if strings.Contains(r.URL.Path, "lastSuccessfulBuild/api/json") {
			fmt.Fprint(w, `{"number":42,"result":"SUCCESS","building":false,"artifacts":[{"relativePath":"build/EasyRPG-Player-macos.app.zip"}]}`)
			return
		}
		switch mode {
		case "status":
			w.WriteHeader(503)
			return
		case "truncated":
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)+100))
		case "oversize":
			w.Header().Set("Content-Length", fmt.Sprint(int64(maxArchive)+1))
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	m.client = &http.Client{Transport: engineContractTransport(func(r *http.Request) (*http.Response, error) {
		q := r.Clone(r.Context())
		q.URL.Scheme = "http"
		q.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return server.Client().Transport.RoundTrip(q)
	}), Timeout: server.Client().Timeout}
}
func engineContractInstallSupport(t *testing.T, m *Manager) {
	t.Helper()
	for _, c := range []Component{ComponentRTP, ComponentSoundFont, ComponentKawariki} {
		b := engineContractSF2()
		if c != ComponentSoundFont {
			b = engineContractArchive(t, c, false)
		}
		engineContractHTTP(t, m, b, "")
		if _, err := m.InstallComponent(context.Background(), c, nil); err != nil {
			t.Fatal(err)
		}
	}
}
func TestEngineContractComponentTransactions(t *testing.T) {
	for _, c := range []Component{ComponentMKXPZ, ComponentEasyRPG, ComponentRTP, ComponentSoundFont, ComponentKawariki} {
		t.Run(string(c), func(t *testing.T) {
			m := engineContractManager(t)
			body := engineContractSF2()
			if c != ComponentSoundFont {
				body = engineContractArchive(t, c, false)
			}
			engineContractHTTP(t, m, body, "")
			result, err := m.InstallComponent(context.Background(), c, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = checkComponent(result.Path, c); err != nil {
				t.Fatal(err)
			}
			if c == ComponentMKXPZ && result.CommitSHA != strings.Repeat("a", 40) {
				t.Fatal(result)
			}
			if c == ComponentEasyRPG && result.BuildNumber != 42 {
				t.Fatal(result)
			}
			s, err := LoadSettings(m.Root)
			if err != nil || s.LastBuildNumber != 0 || s.LastCommitSHA != "" {
				t.Fatal("installer silently saved settings", s, err)
			}
			sentinel := filepath.Join(result.Path, "sentinel")
			if c == ComponentSoundFont {
				sentinel = result.Path
			} else {
				engineContractWrite(t, sentinel, []byte("old"), 0600)
			}
			before, _ := os.ReadFile(sentinel)
			for _, mode := range []string{"status", "truncated", "oversize", "invalid", "cancel", "progress", "verify"} {
				t.Run(mode, func(t *testing.T) {
					data := body
					if mode == "invalid" {
						data = []byte("<html>error</html>")
					}
					engineContractHTTP(t, m, data, mode)
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					var progress ProgressFunc
					want := errors.New("stop callback")
					if mode == "cancel" {
						progress = func(p Progress) error { cancel(); return nil }
					}
					if mode == "progress" {
						progress = func(p Progress) error { return want }
					}
					if mode == "verify" {
						progress = func(p Progress) error {
							if p.Phase == "verify" {
								return want
							}
							return nil
						}
					}
					failed, err := m.InstallComponent(ctx, c, progress)
					if err == nil {
						t.Fatal("failure accepted")
					}
					if (mode == "progress" || mode == "verify") && !errors.Is(err, want) {
						t.Fatal(err)
					}
					if failed.BuildNumber != 0 || failed.CommitSHA != "" {
						t.Fatal("unverified update identifier")
					}
					after, _ := os.ReadFile(sentinel)
					if !bytes.Equal(before, after) {
						t.Fatal("old resource changed")
					}
					stages, _ := filepath.Glob(filepath.Join(m.Root, ".runtime-stage-*"))
					if len(stages) > 0 {
						t.Fatal("stage leak", stages)
					}
				})
			}
		})
	}
}
func TestMKXPZInstallWithoutUpdateAPI(t *testing.T) {
	for _, tc := range []struct {
		name      string
		component Component
		badZIP    bool
	}{
		{"valid", ComponentMKXPZ, false},
		{"invalid-zip", ComponentMKXPZ, true},
		{"easyrpg-unknown-build", ComponentEasyRPG, false},
		{"easyrpg-invalid-zip", ComponentEasyRPG, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := engineContractManager(t)
			marker := filepath.Join(m.Root, componentName(tc.component), "keep")
			engineContractWrite(t, marker, []byte("old"), 0600)
			payload := engineContractArchive(t, tc.component, false)
			if tc.badZIP {
				payload = []byte("invalid zip")
			}
			downloaded := false
			m.client = &http.Client{Transport: engineContractTransport(func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "commits/dev") || strings.Contains(r.URL.Path, "lastSuccessfulBuild/api/json") {
					return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable")), Header: http.Header{}}, nil
				}
				expected := componentRelease + "Z-universal.zip"
				if tc.component == ComponentEasyRPG {
					expected = easyJob + "lastSuccessfulBuild/artifact/osxbuild/EasyRPG-Player-macos.app.zip"
				}
				if r.URL.String() != expected {
					t.Fatalf("unexpected download: %s", r.URL)
				}
				downloaded = true
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload)), Header: http.Header{}}, nil
			})}
			result, err := m.InstallComponent(context.Background(), tc.component, nil)
			if !tc.badZIP {
				if err != nil || result.CommitSHA != "" || result.BuildNumber != 0 || !downloaded || checkComponent(result.Path, tc.component) != nil {
					t.Fatalf("result=%+v err=%v downloaded=%v", result, err, downloaded)
				}
			} else {
				if err == nil || result.Path != "" {
					t.Fatalf("result=%+v err=%v", result, err)
				}
				if b, e := os.ReadFile(marker); e != nil || string(b) != "old" {
					t.Fatal("old install changed", e)
				}
			}
			noStaging(t, m.Root)
		})
	}
}

func TestSoundFontReleaseSource(t *testing.T) {
	m := engineContractManager(t)
	payload := engineContractSF2()
	requested := false
	m.client = &http.Client{Transport: engineContractTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://github.com/m5kro/mkxp-z/releases/download/launcher/GMGSx.SF2" {
			t.Fatalf("unexpected SoundFont source: %s", r.URL)
		}
		requested = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload)), Header: http.Header{}}, nil
	})}
	result, err := m.InstallComponent(context.Background(), ComponentSoundFont, nil)
	if err != nil || !requested {
		t.Fatalf("SoundFont download: requested=%v err=%v", requested, err)
	}
	if b, err := os.ReadFile(result.Path); err != nil || !bytes.Equal(b, payload) {
		t.Fatal("SoundFont payload changed", err)
	}
}

func TestEngineContractCancelBlockedDownload(t *testing.T) {
	m := engineContractManager(t)
	old := engineContractSF2()
	engineContractWrite(t, filepath.Join(m.Root, "GMGSx.SF2"), old, 0600)
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000")
		_, _ = w.Write([]byte("RIFF"))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	m.client = &http.Client{Transport: engineContractTransport(func(r *http.Request) (*http.Response, error) {
		q := r.Clone(r.Context())
		q.URL.Scheme = "http"
		q.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return server.Client().Transport.RoundTrip(q)
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.InstallComponent(ctx, ComponentSoundFont, nil); done <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not interrupt download")
	}
	if b, _ := os.ReadFile(filepath.Join(m.Root, "GMGSx.SF2")); !bytes.Equal(b, old) {
		t.Fatal("cancelled download replaced font")
	}
	stages, _ := filepath.Glob(filepath.Join(m.Root, ".runtime-stage-*"))
	if len(stages) > 0 {
		t.Fatal(stages)
	}
}
func TestEngineContractRuntimeRestoresExecutablePermission(t *testing.T) {
	for _, c := range []Component{ComponentMKXPZ, ComponentEasyRPG} {
		t.Run(string(c), func(t *testing.T) {
			m := engineContractManager(t)
			exe := "Z-universal"
			if c == ComponentEasyRPG {
				exe = "EasyRPG Player"
			}
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			h := &zip.FileHeader{Name: componentName(c) + "/Contents/MacOS/" + exe}
			h.SetMode(0644)
			w, _ := z.CreateHeader(h)
			_, _ = w.Write(engineContractARM())
			_ = z.Close()
			engineContractHTTP(t, m, b.Bytes(), "")
			if _, err := m.InstallComponent(context.Background(), c, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestEngineContractRuntimeBadARMPreservesOld(t *testing.T) {
	for _, c := range []Component{ComponentMKXPZ, ComponentEasyRPG} {
		t.Run(string(c), func(t *testing.T) {
			m := engineContractManager(t)
			engineContractRuntime(t, m, GameEngine(c))
			old := filepath.Join(m.Root, componentName(c), "keep")
			engineContractWrite(t, old, []byte("old"), 0600)
			engineContractHTTP(t, m, engineContractArchive(t, c, true), "")
			if _, err := m.InstallComponent(context.Background(), c, nil); err == nil {
				t.Fatal("invalid ARM accepted")
			}
			if b, _ := os.ReadFile(old); string(b) != "old" {
				t.Fatal("old runtime changed")
			}
		})
	}
}
func TestEngineContractZIPBoundary(t *testing.T) {
	for _, name := range []string{"RTP/../escape", "/absolute", "RTP/link"} {
		t.Run(name, func(t *testing.T) {
			m := engineContractManager(t)
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			h := &zip.FileHeader{Name: name}
			h.SetMode(0600)
			value := "bad"
			if name == "RTP/link" {
				h.SetMode(os.ModeSymlink | 0777)
				value = "../../outside"
			}
			w, _ := z.CreateHeader(h)
			_, _ = io.WriteString(w, value)
			_ = z.Close()
			engineContractHTTP(t, m, b.Bytes(), "")
			if _, err := m.InstallComponent(context.Background(), ComponentRTP, nil); err == nil {
				t.Fatal("unsafe ZIP accepted")
			}
			if _, err := os.Stat(filepath.Join(m.Root, "RTP")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("partial install exists")
			}
		})
	}
}
func TestEngineContractUpdateArtifactBindingAndStatus(t *testing.T) {
	m := engineContractManager(t)
	requested := []string{}
	m.client = &http.Client{Transport: engineContractTransport(func(r *http.Request) (*http.Response, error) {
		requested = append(requested, r.URL.String())
		body := `{"number":12,"result":"SUCCESS","artifacts":[{"relativePath":"out/EasyRPG-Player-macos.app.zip"}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Header: http.Header{}}, nil
	})}
	update, err := m.CheckUpdate(context.Background(), ComponentEasyRPG)
	if err != nil {
		t.Fatal(err)
	}
	if update.DownloadURL != easyJob+"12/artifact/out/EasyRPG-Player-macos.app.zip" || !update.Available {
		t.Fatal(update)
	}
	for _, body := range []string{`{"number":12,"result":"FAILURE"}`, `{"number":12,"result":"SUCCESS","artifacts":[]}`, `{"number":12,"result":"SUCCESS","artifacts":[{"relativePath":"../EasyRPG-Player-macos.app.zip"}]}`} {
		m.client.Transport = engineContractTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})
		if _, err := m.CheckUpdate(context.Background(), ComponentEasyRPG); err == nil {
			t.Fatal("bad metadata accepted", body)
		}
	}
}
func TestEngineContractPublishRollbackAndRecovery(t *testing.T) {
	m := engineContractManager(t)
	root := m.Root
	target := filepath.Join(root, "runtime.app")
	engineContractWrite(t, filepath.Join(target, "keep"), []byte("old"), 0600)
	if err := m.publishComponent(filepath.Join(root, "missing-payload"), target); err == nil {
		t.Fatal("missing publish accepted")
	}
	if b, _ := os.ReadFile(filepath.Join(target, "keep")); string(b) != "old" {
		t.Fatal("rollback lost previous runtime")
	}
	backup := filepath.Join(root, ".runtime.app.previous")
	engineContractWrite(t, filepath.Join(backup, "recovery"), []byte("recover"), 0600)
	payload := filepath.Join(root, "payload")
	engineContractWrite(t, filepath.Join(payload, "new"), []byte("new"), 0600)
	if err := m.publishComponent(payload, target); err == nil {
		t.Fatal("existing recovery copy replaced")
	}
	for _, p := range []string{filepath.Join(target, "keep"), filepath.Join(backup, "recovery"), filepath.Join(payload, "new")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
}
func TestEngineContractFailedRestoreRetainsRecovery(t *testing.T) {
	m := engineContractManager(t)
	engineContractRuntime(t, m, EasyRPG)
	target := filepath.Join(m.Root, componentName(ComponentEasyRPG))
	engineContractWrite(t, filepath.Join(target, "keep"), []byte("old\x00bundle"), 0600)
	before := engineContractSnapshot(t, target)
	backup := filepath.Join(m.Root, "."+componentName(ComponentEasyRPG)+".previous")
	publishErr, restoreErr := errors.New("publish failed"), errors.New("restore failed")
	calls := 0
	m.renameComponent = func(from, to string) error {
		calls++
		if to == target {
			if from == backup {
				return restoreErr
			}
			return publishErr
		}
		return os.Rename(from, to)
	}
	engineContractHTTP(t, m, engineContractArchive(t, ComponentEasyRPG, false), "")
	result, err := m.InstallComponent(context.Background(), ComponentEasyRPG, nil)
	if !errors.Is(err, publishErr) || !errors.Is(err, restoreErr) || calls != 3 || result.Path != "" || result.BuildNumber != 0 {
		t.Fatalf("both failures must propagate: calls=%d result=%+v err=%v", calls, result, err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed target exists", err)
	}
	if after := engineContractSnapshot(t, backup); !reflect.DeepEqual(before, after) {
		t.Fatal("recovery bytes changed", after)
	}
	stages, err := filepath.Glob(filepath.Join(m.Root, ".runtime-stage-*"))
	if err != nil || len(stages) != 0 {
		t.Fatal("cleanup leaked staging", stages, err)
	}
	m.renameComponent = nil
	result, err = m.InstallComponent(context.Background(), ComponentEasyRPG, nil)
	if err == nil || !strings.Contains(err.Error(), "existing recovery copy") || result.Path != "" {
		t.Fatal("next install did not refuse recovery copy", result, err)
	}
	if after := engineContractSnapshot(t, backup); !reflect.DeepEqual(before, after) {
		t.Fatal("retry/cleanup removed recovery bytes")
	}
	stages, err = filepath.Glob(filepath.Join(m.Root, ".runtime-stage-*"))
	if err != nil || len(stages) != 0 {
		t.Fatal("retry cleanup leaked staging", stages, err)
	}
}

func TestEngineContractComponentRedirectPolicy(t *testing.T) {
	initial, _ := http.NewRequest("GET", componentRelease+"RTP.zip", nil)
	for _, u := range []string{"http://github.com/a", "https://evil.test/a", "https://github.com.evil.test/a", "https://github.com:444/a", "https://user@github.com/a"} {
		r, _ := http.NewRequest("GET", u, nil)
		if err := componentRedirect(r, []*http.Request{initial}); err == nil {
			t.Fatal("unsafe redirect", u)
		}
	}
	for _, u := range []string{"https://github.com/a", "https://release-assets.githubusercontent.com/a", "https://objects.githubusercontent.com/a"} {
		r, _ := http.NewRequest("GET", u, nil)
		if err := componentRedirect(r, []*http.Request{initial}); err != nil {
			t.Fatal(err)
		}
	}
	r, _ := http.NewRequest("GET", "https://release-assets.githubusercontent.com/a", nil)
	if err := officialRedirect(r, nil); err == nil {
		t.Fatal("NW allowlist widened")
	}
}
func TestEngineContractUninstallComponentsKeepsSettings(t *testing.T) {
	m := engineContractManager(t)
	engineContractRuntime(t, m, MKXPZ)
	engineContractRuntime(t, m, EasyRPG)
	engineContractRuntime(t, m, NWJS)
	engineContractInstallSupport(t, m)
	if err := SaveSettings(m.Root, Settings{LastSelectedVersion: "v1.2.3"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(context.Background(), ComponentMKXPZ); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Z-universal.app", "RTP", "GMGSx.SF2", "kawariki"} {
		if _, err := os.Lstat(filepath.Join(m.Root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("not removed", name, err)
		}
	}
	if _, err := m.CheckRuntime(GameInfo{Engine: EasyRPG}, Settings{}); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(context.Background(), Component("v1.2.3")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(m.Root, "settings.json")); err != nil {
		t.Fatal("settings removed", err)
	}
	if _, err := os.Stat(filepath.Join(m.Root, "v1.2.3")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("NW version remains")
	}
}
func TestEngineContractUninstallLockAndScope(t *testing.T) {
	m := engineContractManager(t)
	engineContractRuntime(t, m, MKXPZ)
	outside := t.TempDir()
	engineContractWrite(t, filepath.Join(outside, "save"), []byte("save"), 0600)
	if err := os.Symlink(outside, filepath.Join(m.Root, "RTP")); err != nil {
		t.Fatal(err)
	}
	engineContractWrite(t, filepath.Join(m.Root, "unknown-game", "save"), []byte("keep"), 0600)
	engineContractWrite(t, filepath.Join(m.Root, "v99.0.0", "save"), []byte("also user data"), 0600)
	if err := m.Uninstall(context.Background(), Component("v99.0.0")); err == nil {
		t.Fatal("unrecognized version-like user directory removed")
	}
	if err := SaveSettings(m.Root, Settings{}); err != nil {
		t.Fatal(err)
	}
	release, err := AcquireManagedLock(m.Root)
	if err != nil {
		t.Fatal(err)
	}
	lockBefore, err := os.Stat(filepath.Join(m.Root, ".nwjs-install.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.UninstallAll(context.Background()); err == nil {
		t.Fatal("bypassed lock")
	}
	release()
	if err = m.UninstallAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	lockAfter, _ := os.Stat(filepath.Join(m.Root, ".nwjs-install.lock"))
	if !os.SameFile(lockBefore, lockAfter) {
		t.Fatal("lock inode changed")
	}
	for _, p := range []string{filepath.Join(outside, "save"), filepath.Join(m.Root, "unknown-game", "save"), filepath.Join(m.Root, "v99.0.0", "save")} {
		if _, err = os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = os.Stat(filepath.Join(m.Root, "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("settings not removed")
	}
	if err = m.Uninstall(context.Background(), Component("../outside")); err == nil {
		t.Fatal("unsafe component accepted")
	}
}

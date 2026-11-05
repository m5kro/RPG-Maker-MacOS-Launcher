package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testVersion = "v0.100.0"
const testPrefix = "nwjs-sdk-v0.100.0-osx-arm64"
const testArchive = testPrefix + ".zip"
const appPrefix = testPrefix + "/nwjs.app/"

type zipEntry struct {
	name string
	mode os.FileMode
	data []byte
}

func executable(cpu macho.Cpu) []byte {
	var b bytes.Buffer
	for _, v := range []uint32{macho.Magic64, uint32(cpu), 0, uint32(macho.TypeExec), 0, 0, 0, 0} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	return b.Bytes()
}

func goodEntries() []zipEntry {
	return []zipEntry{
		{appPrefix + "Contents/MacOS/nwjs", 0755, executable(macho.CpuArm64)},
		{appPrefix + "Contents/Frameworks/Test.framework/Versions/A/Test", 0755, []byte("framework")},
		{appPrefix + "Contents/Frameworks/Test.framework/Versions/Current", os.ModeSymlink | 0777, []byte("A")},
		{appPrefix + "Contents/Frameworks/Test.framework/Test", os.ModeSymlink | 0777, []byte("Versions/Current/Test")},
	}
}

func makeZIP(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(e.mode)
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func fixture(t *testing.T, archive []byte, override http.HandlerFunc) config {
	t.Helper()
	sum := sha256.Sum256(archive)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if override != nil {
			override(w, r)
			return
		}
		switch r.URL.Path {
		case "/versions.json":
			io.WriteString(w, `{"versions":[{"version":"v0.100.0","files":["osx-x64","linux-x64","osx-arm64"]},{"version":"v0.14.7","files":["osx-x64"]},{"version":"v0.1.0","files":["linux-x64"]}]}`)
		case "/" + testVersion + "/SHASUMS256.txt":
			fmt.Fprintf(w, "%x  %s\n", sum, testArchive)
		case "/" + testVersion + "/" + testArchive:
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return config{client: s.Client(), versionsURL: s.URL + "/versions.json", downloadBase: s.URL}
}

func args(root string) []string {
	return []string{"install", "--root", root, "--version", testVersion, "--arch", "arm64"}
}

func invoke(ctx context.Context, c config, arguments []string) (int, string, string) {
	var out, diag bytes.Buffer
	code := run(ctx, arguments, &out, &diag, c)
	return code, out.String(), diag.String()
}

func events(t *testing.T, out string) []map[string]any {
	t.Helper()
	var result []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("non-JSON stdout %q: %v", line, err)
		}
		result = append(result, e)
	}
	return result
}

func noStaging(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".nwjs-stage-") {
			t.Errorf("staging remains: %s", e.Name())
		}
	}
}

func failedInstall(t *testing.T, c config) {
	t.Helper()
	root := t.TempDir()
	old := filepath.Join(root, "v0.99.0")
	if err := os.Mkdir(old, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(old, "keep")
	if err := os.WriteFile(sentinel, []byte("prior version"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, diag := invoke(context.Background(), c, args(root))
	if code == 0 || diag == "" {
		t.Fatalf("expected diagnostic failure, got %d %s %s", code, out, diag)
	}
	for _, e := range events(t, out) {
		if e["event"] == "complete" {
			t.Fatal("false completion")
		}
	}
	if _, err := os.Lstat(filepath.Join(root, testVersion)); !os.IsNotExist(err) {
		t.Fatalf("failed target exists: %v", err)
	}
	b, err := os.ReadFile(sentinel)
	if err != nil || string(b) != "prior version" {
		t.Fatal("prior version mutated")
	}
	noStaging(t, root)
}

func TestVersions(t *testing.T) {
	c := fixture(t, nil, nil)
	code, out, diag := invoke(context.Background(), c, []string{"versions"})
	want := "{\"event\":\"versions\",\"versions\":[{\"version\":\"v0.100.0\",\"arches\":[\"arm64\",\"x64\"]},{\"version\":\"v0.14.7\",\"arches\":[\"x64\"]}]}\n"
	if code != 0 || out != want || diag != "" {
		t.Fatalf("got %d %q %q", code, out, diag)
	}
}

func TestInstallAndNoop(t *testing.T) {
	c := fixture(t, makeZIP(t, goodEntries()), nil)
	root := t.TempDir()
	old := filepath.Join(root, "v0.99.0")
	if err := os.WriteFile(old, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		code, out, diag := invoke(context.Background(), c, args(root))
		if code != 0 || diag != "" {
			t.Fatalf("%d: %d %s %s", i, code, out, diag)
		}
		es := events(t, out)
		last := es[len(es)-1]
		canonicalRoot, _ := filepath.EvalSymlinks(root)
		if last["event"] != "complete" || last["version"] != testVersion || last["path"] != filepath.Join(canonicalRoot, testVersion, "nwjs.app") {
			t.Fatalf("completion: %v", last)
		}
		if i == 0 {
			phases := map[string]bool{}
			for _, e := range es[:len(es)-1] {
				if e["event"] != "progress" {
					t.Fatalf("bad event %v", e)
				}
				phases[e["phase"].(string)] = true
				if _, ok := e["completed"].(float64); !ok {
					t.Fatal("missing completed")
				}
				if _, ok := e["total"].(float64); !ok {
					t.Fatal("missing total")
				}
			}
			for _, p := range []string{"download", "verify", "extract"} {
				if !phases[p] {
					t.Fatalf("missing phase %s", p)
				}
			}
		} else if len(es) != 1 {
			t.Fatalf("no-op downloaded again: %s", out)
		}
	}
	app := filepath.Join(root, testVersion, "nwjs.app")
	info, err := os.Stat(filepath.Join(app, "Contents/MacOS/nwjs"))
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("executable mode: %v %v", info, err)
	}
	link := filepath.Join(app, "Contents/Frameworks/Test.framework/Test")
	if target, err := os.Readlink(link); err != nil || target != "Versions/Current/Test" {
		t.Fatalf("symlink: %q %v", target, err)
	}
	if b, err := os.ReadFile(link); err != nil || string(b) != "framework" {
		t.Fatalf("framework: %q %v", b, err)
	}
	if b, _ := os.ReadFile(old); string(b) != "untouched" {
		t.Fatal("old version changed")
	}
	noStaging(t, root)
}

func TestDownloadFailures(t *testing.T) {
	archive := makeZIP(t, goodEntries())
	sum := sha256.Sum256(archive)
	for _, kind := range []string{"manifest404", "archive503", "interrupted", "mismatch", "missing-checksum", "duplicate-checksum"} {
		t.Run(kind, func(t *testing.T) {
			c := fixture(t, archive, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "SHASUMS256.txt") {
					switch kind {
					case "manifest404":
						http.NotFound(w, r)
					case "mismatch":
						fmt.Fprintf(w, "%064x  %s\n", 0, testArchive)
					case "missing-checksum":
						fmt.Fprintf(w, "%x  another.zip\n", sum)
					case "duplicate-checksum":
						fmt.Fprintf(w, "%x  %s\n%x  %s\n", sum, testArchive, sum, testArchive)
					default:
						fmt.Fprintf(w, "%x  %s\n", sum, testArchive)
					}
					return
				}
				if kind == "archive503" {
					w.WriteHeader(503)
					return
				}
				if kind == "interrupted" {
					w.Header().Set("Content-Length", fmt.Sprint(len(archive)+100))
					w.Write(archive[:len(archive)/2])
					return
				}
				w.Write(archive)
			})
			failedInstall(t, c)
		})
	}
}

func TestRejectArchives(t *testing.T) {
	cases := map[string][]zipEntry{
		"traversal":                {{testPrefix + "/../escape", 0644, []byte("x")}},
		"absolute":                 {{"/tmp/nwjs-escape", 0644, []byte("x")}},
		"backslash":                {{appPrefix + `Contents\escape`, 0644, []byte("x")}},
		"absolute-link":            {{appPrefix + "Contents/link", os.ModeSymlink | 0777, []byte("/tmp")}},
		"escaping-link":            {{appPrefix + "Contents/link", os.ModeSymlink | 0777, []byte("../../../outside")}},
		"dangling-link":            {{appPrefix + "Contents/link", os.ModeSymlink | 0777, []byte("missing")}},
		"loop":                     {{appPrefix + "Contents/a", os.ModeSymlink | 0777, []byte("b")}, {appPrefix + "Contents/b", os.ModeSymlink | 0777, []byte("a")}},
		"symlink-ancestor":         {{appPrefix + "Contents/alias", os.ModeSymlink | 0777, []byte("MacOS")}, {appPrefix + "Contents/alias/write", 0644, []byte("x")}},
		"reverse-symlink-ancestor": {{appPrefix + "Contents/alias/write", 0644, []byte("x")}, {appPrefix + "Contents/alias", os.ModeSymlink | 0777, []byte("MacOS")}},
		"duplicate":                {{appPrefix + "Contents/MacOS/nwjs", 0755, executable(macho.CpuArm64)}},
		"fifo":                     {{appPrefix + "Contents/pipe", os.ModeNamedPipe | 0600, nil}},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) { failedInstall(t, fixture(t, makeZIP(t, append(goodEntries(), extra...)), nil)) })
	}
	t.Run("not-zip", func(t *testing.T) { failedInstall(t, fixture(t, []byte("not zip"), nil)) })
	for _, kind := range []string{"missing-executable", "not-executable", "wrong-arch", "not-mach-o"} {
		t.Run(kind, func(t *testing.T) {
			es := goodEntries()
			switch kind {
			case "missing-executable":
				es = es[1:]
			case "not-executable":
				es[0].mode = 0644
			case "wrong-arch":
				es[0].data = executable(macho.CpuAmd64)
			case "not-mach-o":
				es[0].data = []byte("not an executable")
			}
			failedInstall(t, fixture(t, makeZIP(t, es), nil))
		})
	}
}

func TestExistingTargetsPreserved(t *testing.T) {
	for _, kind := range []string{"empty", "file", "symlink", "legacy", "arch-mismatch", "rosetta-mismatch", "tampered-executable"} {
		t.Run(kind, func(t *testing.T) {
			c := fixture(t, makeZIP(t, goodEntries()), nil)
			root := t.TempDir()
			target := filepath.Join(root, testVersion)
			switch kind {
			case "file":
				if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
			case "empty":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			default:
				code, _, diag := invoke(context.Background(), c, args(root))
				if code != 0 {
					t.Fatal(diag)
				}
				switch kind {
				case "legacy":
					if err := os.Remove(filepath.Join(target, ".nwjs-install.json")); err != nil {
						t.Fatal(err)
					}
				case "tampered-executable":
					if err := os.Chmod(filepath.Join(target, "nwjs.app/Contents/MacOS/nwjs"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			a := args(root)
			if kind == "arch-mismatch" {
				a[len(a)-1] = "x64"
			}
			if kind == "rosetta-mismatch" {
				a[len(a)-1] = "x64"
				a = append(a, "--rosetta")
			}
			code, out, diag := invoke(context.Background(), c, a)
			if code == 0 || diag == "" {
				t.Fatalf("overwrote %s: %s", kind, out)
			}
			after, err := os.Lstat(target)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("target replaced: %v", err)
			}
			noStaging(t, root)
		})
	}
}

func TestCancelAndCompetingInstall(t *testing.T) {
	archive := makeZIP(t, goodEntries())
	sum := sha256.Sum256(archive)
	started := make(chan struct{})
	c := fixture(t, archive, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHASUMS256.txt") {
			fmt.Fprintf(w, "%x  %s\n", sum, testArchive)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(archive)))
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	root := t.TempDir()
	stale := filepath.Join(root, ".nwjs-stale-do-not-touch")
	if err := os.Mkdir(stale, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { code, _, _ := invoke(ctx, c, args(root)); done <- code }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("download never started")
	}
	code, out, diag := invoke(context.Background(), c, args(root))
	if code == 0 || !strings.Contains(diag, "lock") {
		t.Fatalf("competing install: %d %s %s", code, out, diag)
	}
	cancel()
	select {
	case code = <-done:
		if code == 0 {
			t.Fatal("cancel succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel stuck")
	}
	noStaging(t, root)
	if _, err := os.Stat(stale); err != nil {
		t.Fatal("deleted preexisting state")
	}
	// A persistent lock file must not prevent a later process acquiring its OS lock.
	c2 := fixture(t, archive, nil)
	if code, _, diag := invoke(context.Background(), c2, args(root)); code != 0 {
		t.Fatal(diag)
	}
}

func TestInvalidCLI(t *testing.T) {
	c := fixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid CLI made network request")
		w.WriteHeader(500)
	})
	root := t.TempDir()
	cases := [][]string{nil, {"unknown"}, {"versions", "extra"}, {"install"}, {"install", "--base-url", "http://example.com"}}
	for _, v := range []string{"../v0.1.0", "v1.2", "1.2.3", "v1.2.3/escape", "v1.2.3-beta", "v01.2.3"} {
		a := args(root)
		a[4] = v
		cases = append(cases, a)
	}
	for _, arch := range []string{"amd64", "ARM64", "../x64", ""} {
		a := args(root)
		a[6] = arch
		cases = append(cases, a)
	}
	for _, r := range []string{"", "relative", "/"} {
		a := args(root)
		a[2] = r
		cases = append(cases, a)
	}
	cases = append(cases, append(args(root), "--rosetta"), append(args(root), "extra"))
	for _, a := range cases {
		code, out, diag := invoke(context.Background(), c, a)
		if code == 0 || out != "" || diag == "" {
			t.Errorf("%v: %d %q %q", a, code, out, diag)
		}
	}
}

func TestSerializedJSON(t *testing.T) {
	var b bytes.Buffer
	e := emitter{out: &b}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if err := e.emit(map[string]any{"event": "test", "n": j}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if len(events(t, b.String())) != 400 {
		t.Fatal("lost events")
	}
}

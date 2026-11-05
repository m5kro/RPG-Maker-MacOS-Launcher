package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/macho"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type callbackWriter struct {
	bytes.Buffer
	callback func([]byte)
}

func (w *callbackWriter) Write(b []byte) (int, error) {
	n, err := w.Buffer.Write(b)
	w.callback(b)
	return n, err
}

func TestCancelDuringPhases(t *testing.T) {
	for _, phase := range []string{"download", "verify", "extract"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			c := fixture(t, makeZIP(t, goodEntries()), nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := &callbackWriter{callback: func(b []byte) {
				var e map[string]any
				if err := json.Unmarshal(b, &e); err != nil {
					t.Fatal(err)
				}
				if e["phase"] == phase {
					cancel()
				}
			}}
			var diag bytes.Buffer
			if code := run(ctx, args(root), out, &diag, c); code == 0 {
				t.Fatal("cancellation succeeded")
			}
			if strings.Contains(out.String(), `"event":"complete"`) {
				t.Fatal("false completion")
			}
			if _, err := os.Lstat(filepath.Join(root, testVersion)); !os.IsNotExist(err) {
				t.Fatal("canceled install published")
			}
			noStaging(t, root)
		})
	}
}

func TestPublishDoesNotReplaceNewTarget(t *testing.T) {
	c := fixture(t, makeZIP(t, goodEntries()), nil)
	root := t.TempDir()
	target := filepath.Join(root, testVersion)
	created := false
	out := &callbackWriter{callback: func(b []byte) {
		if !created && strings.Contains(string(b), `"phase":"extract"`) {
			created = true
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, "keep"), []byte("concurrent creator"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}}
	var diag bytes.Buffer
	if code := run(context.Background(), args(root), out, &diag, c); code == 0 {
		t.Fatal("replaced competing target")
	}
	if b, err := os.ReadFile(filepath.Join(target, "keep")); err != nil || string(b) != "concurrent creator" {
		t.Fatalf("target mutated: %s %v", b, err)
	}
	noStaging(t, root)
}

func TestRosettaAndArchitecture(t *testing.T) {
	es := goodEntries()
	for i := range es {
		es[i].name = strings.Replace(es[i].name, "osx-arm64", "osx-x64", 1)
	}
	es[0].data = executable(macho.CpuAmd64)
	archive := makeZIP(t, es)
	sum := sha256.Sum256(archive)
	c := fixture(t, archive, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHASUMS256.txt") {
			fmt.Fprintf(w, "%x  nwjs-sdk-%s-osx-x64.zip\n", sum, testVersion)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "osx-x64.zip") {
			t.Errorf("wrong request %s", r.URL.Path)
		}
		w.Write(archive)
	})
	root := t.TempDir()
	a := args(root)
	a[6] = "x64"
	a = append(a, "--rosetta")
	for i := 0; i < 2; i++ {
		if code, _, diag := invoke(context.Background(), c, a); code != 0 {
			t.Fatal(diag)
		}
	}
	if _, err := regularFile(filepath.Join(root, testVersion, "run-with-rosetta")); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := invoke(context.Background(), c, a[:len(a)-1]); code == 0 {
		t.Fatal("no-op with wrong Rosetta setting")
	}
}

func TestRelativeFrameworkSymlink(t *testing.T) {
	es := append(goodEntries(), zipEntry{appPrefix + "Contents/Frameworks/Test.framework/Versions/A/other", os.ModeSymlink | 0777, []byte("../Current/Test")})
	c := fixture(t, makeZIP(t, es), nil)
	if code, _, diag := invoke(context.Background(), c, args(t.TempDir())); code != 0 {
		t.Fatal(diag)
	}
}

func TestLimits(t *testing.T) {
	if _, err := readBounded(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("unbounded metadata")
	}
	if b, err := readBounded(strings.NewReader("1234"), 4); err != nil || string(b) != "1234" {
		t.Fatal("exact limit rejected")
	}
	for _, kind := range []string{"manifest", "archive"} {
		t.Run(kind, func(t *testing.T) {
			c := fixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "SHASUMS256.txt") && kind != "manifest" {
					fmt.Fprintf(w, "%064x  %s\n", 0, testArchive)
					return
				}
				limit := maxManifest
				if kind == "archive" {
					limit = maxArchive
				}
				w.Header().Set("Content-Length", fmt.Sprint(int64(limit)+1))
				w.WriteHeader(200)
			})
			failedInstall(t, c)
		})
	}
	t.Run("expanded-zip", func(t *testing.T) {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		h := &zip.FileHeader{Name: appPrefix + "big", Method: zip.Store, UncompressedSize64: maxExtracted + 1, CompressedSize64: 1}
		h.SetMode(0600)
		w, err := z.CreateRaw(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
		failedInstall(t, fixture(t, b.Bytes(), nil))
	})
}

func TestRootAndLockValidation(t *testing.T) {
	for _, kind := range []string{"root-symlink", "root-file", "writable-root", "lock-symlink", "lock-directory", "lock-fifo"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "managed")
			switch kind {
			case "root-symlink":
				if err := os.Symlink(t.TempDir(), root); err != nil {
					t.Fatal(err)
				}
			case "root-file":
				if err := os.WriteFile(root, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
			}
			lock := filepath.Join(root, ".nwjs-install.lock")
			switch kind {
			case "writable-root":
				if err := os.Chmod(root, 0777); err != nil {
					t.Fatal(err)
				}
			case "lock-symlink":
				if err := os.Symlink(filepath.Join(parent, "outside"), lock); err != nil {
					t.Fatal(err)
				}
			case "lock-directory":
				if err := os.Mkdir(lock, 0700); err != nil {
					t.Fatal(err)
				}
			case "lock-fifo":
				if err := syscall.Mkfifo(lock, 0600); err != nil {
					t.Fatal(err)
				}
			}
			c := fixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				t.Error("unsafe root/lock reached network")
				w.WriteHeader(500)
			})
			if code, _, diag := invoke(context.Background(), c, args(root)); code == 0 || diag == "" {
				t.Fatal("unsafe root accepted")
			}
		})
	}
}

func TestVersionsErrorsAndTimeout(t *testing.T) {
	for _, kind := range []string{"http", "json", "missing-array", "size", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			c := fixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "http":
					w.WriteHeader(503)
				case "json":
					io.WriteString(w, "{")
				case "missing-array":
					io.WriteString(w, `{}`)
				case "size":
					w.Header().Set("Content-Length", fmt.Sprint(maxMetadata+1))
				case "timeout":
					<-r.Context().Done()
				}
			})
			c.client.Timeout = 50 * time.Millisecond
			code, out, diag := invoke(context.Background(), c, []string{"versions"})
			if code == 0 || out != "" || diag == "" {
				t.Fatalf("%d %q %q", code, out, diag)
			}
		})
	}
}

func TestOfficialRedirectPolicy(t *testing.T) {
	for _, raw := range []string{"http://dl.nwjs.io/a", "https://evil.example/a", "https://dl.nwjs.io:8080/a", "https://user@dl.nwjs.io/a"} {
		u, _ := url.Parse(raw)
		if err := officialRedirect(&http.Request{URL: u}, nil); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	u, _ := url.Parse("https://dl.node-webkit.org/v0.100.0/SHASUMS256.txt")
	if err := officialRedirect(&http.Request{URL: u}, nil); err != nil {
		t.Fatal(err)
	}
	if err := officialRedirect(&http.Request{URL: u}, make([]*http.Request, 5)); err == nil {
		t.Fatal("redirect loop accepted")
	}
}

// These endpoints exist only in the test binary; production has no URL override flags or environment variables.
func TestProcessWorker(t *testing.T) {
	root := os.Getenv("NWJS_TEST_ROOT")
	if root == "" {
		return
	}
	c := config{client: &http.Client{Timeout: time.Minute}, downloadBase: os.Getenv("NWJS_TEST_SERVER")}
	os.Exit(process(c, args(root)))
}

func child(t *testing.T, root, server string) *exec.Cmd {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestProcessWorker$")
	cmd.Env = append(os.Environ(), "NWJS_TEST_ROOT="+root, "NWJS_TEST_SERVER="+server)
	return cmd
}

func TestSignalCleanupAndCrashLock(t *testing.T) {
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM, os.Kill} {
		t.Run(signal.String(), func(t *testing.T) {
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
			cmd := child(t, root, c.downloadBase)
			var out, diag bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &diag
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill() })
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("child never started download")
			}
			if err := cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("signal returned success")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("child did not exit")
			}
			if strings.Contains(out.String(), `"event":"complete"`) {
				t.Fatal("canceled completion")
			}
			if signal != os.Kill {
				noStaging(t, root)
				if diag.Len() == 0 {
					t.Fatal("missing cancellation diagnostic")
				}
			}
			// Even SIGKILL releases the kernel lock; unknown orphan staging must not be deleted.
			before, _ := filepath.Glob(filepath.Join(root, ".nwjs-stage-*"))
			c2 := fixture(t, archive, nil)
			if code, _, diag := invoke(context.Background(), c2, args(root)); code != 0 {
				t.Fatal(diag)
			}
			for _, path := range before {
				if _, err := os.Stat(path); err != nil {
					t.Fatal("blindly deleted orphan staging")
				}
			}
		})
	}
}

func TestInstalledValidationCancellation(t *testing.T) {
	c := fixture(t, makeZIP(t, goodEntries()), nil)
	root := t.TempDir()
	if code, _, diag := invoke(context.Background(), c, args(root)); code != 0 {
		t.Fatal(diag)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := validateInstalled(ctx, filepath.Join(root, testVersion), installOptions{root: root, version: testVersion, arch: "arm64"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("existing bundle validation ignored cancellation: %v", err)
	}
}

func TestBrokenStdoutCleansStaging(t *testing.T) {
	c := fixture(t, makeZIP(t, goodEntries()), nil)
	root := t.TempDir()
	cmd := child(t, root, c.downloadBase)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	defer w.Close()
	cmd.Stdout = w
	var diag bytes.Buffer
	cmd.Stderr = &diag
	err = cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 || !strings.Contains(diag.String(), "broken pipe") {
		t.Fatalf("expected handled stdout failure, got %v / %s", err, diag.String())
	}
	noStaging(t, root)
}

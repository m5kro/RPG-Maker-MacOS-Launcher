package launcher

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Route official HTTPS requests to a local fixture only in tests, without a production URL override.
// These tests must not run in parallel because http.DefaultTransport is process-global.
func apiFixture(t *testing.T, archive []byte, handler http.HandlerFunc) {
	t.Helper()
	c := fixture(t, archive, handler)
	u, err := url.Parse(c.downloadBase)
	if err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, u.Host)
	}
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = original
		transport.CloseIdleConnections()
	})
}

func TestAPIListNWVersions(t *testing.T) {
	apiFixture(t, nil, nil)
	got, err := ListNWVersions(context.Background())
	want := []NWVersion{{Version: testVersion, Arches: []string{"arm64"}}, {Version: "v0.99.0", Arches: []string{"arm64"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("versions: %#v / %v", got, err)
	}
}

func TestAPIInstallProgressAndReceiptRequired(t *testing.T) {
	apiFixture(t, makeZIP(t, goodEntries()), nil)
	root := t.TempDir()
	var progress []Progress
	var callback ProgressFunc = func(p Progress) error {
		progress = append(progress, p)
		return nil
	}
	app, err := InstallNW(context.Background(), root, testVersion, callback)
	canonical, _ := filepath.EvalSymlinks(root)
	if err != nil || app != filepath.Join(canonical, testVersion, "nwjs.app") {
		t.Fatalf("install: %q / %v", app, err)
	}
	for _, phase := range []string{"download", "verify", "extract"} {
		var matching []Progress
		for _, p := range progress {
			if p.Phase == phase {
				matching = append(matching, p)
				if p.Completed < 0 || p.Total < p.Completed {
					t.Fatalf("invalid progress: %#v", p)
				}
			}
		}
		if len(matching) < 2 || matching[0].Completed != 0 || matching[len(matching)-1].Completed != matching[len(matching)-1].Total {
			t.Fatalf("missing phase start/end for %s: %#v", phase, matching)
		}
	}
	progress = nil
	if again, err := InstallNW(context.Background(), root, testVersion, callback); err != nil || again != app || len(progress) != 0 {
		t.Fatalf("no-op: %q / %v / %#v", again, err, progress)
	}
	if err := os.Remove(filepath.Join(root, testVersion, ".nwjs-install.json")); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, filepath.Join(root, testVersion))
	if path, err := InstallNW(context.Background(), root, testVersion, nil); err == nil || path != "" || !strings.Contains(err.Error(), "cannot prove prior helper install") {
		t.Fatalf("receipt-less target accepted: %q / %v", path, err)
	}
	if !reflect.DeepEqual(before, snapshot(t, filepath.Join(root, testVersion))) {
		t.Fatal("receipt-less target changed")
	}
	noStaging(t, root)
}

func TestAPIProgressErrorAndCancellation(t *testing.T) {
	for _, phase := range []string{"download", "verify", "extract"} {
		for _, cancelInstead := range []bool{false, true} {
			name := phase + "/callback-error"
			if cancelInstead {
				name = phase + "/cancel"
			}
			t.Run(name, func(t *testing.T) {
				apiFixture(t, makeZIP(t, goodEntries()), nil)
				root := t.TempDir()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				want := error(syscall.EPIPE)
				if cancelInstead {
					want = context.Canceled
				}
				called := false
				path, err := InstallNW(ctx, root, testVersion, func(p Progress) error {
					if p.Phase == phase {
						called = true
						if cancelInstead {
							cancel()
							return nil
						}
						return want
					}
					return nil
				})
				if !called || path != "" || !errors.Is(err, want) {
					t.Fatalf("callback failure/cancel: called=%v path=%q err=%v", called, path, err)
				}
				if _, err := os.Lstat(filepath.Join(root, testVersion)); !os.IsNotExist(err) {
					t.Fatalf("failed install published: %v", err)
				}
				noStaging(t, root)
				release, err := AcquireManagedLock(root)
				if err != nil {
					t.Fatalf("lock leaked: %v", err)
				}
				release()
				if _, err := InstallNW(context.Background(), root, testVersion, nil); err != nil {
					t.Fatalf("retry with nil callback: %v", err)
				}
			})
		}
	}
}

func TestAPIPreCanceledHasNoSideEffects(t *testing.T) {
	apiFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		t.Error("pre-canceled operation reached network")
		w.WriteHeader(500)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListNWVersions(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("list: %v", err)
	}
	root := filepath.Join(t.TempDir(), "managed")
	if path, err := InstallNW(ctx, root, testVersion, nil); path != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("install: %q / %v", path, err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("pre-canceled install created root: %v", err)
	}
}

func TestAPIListCancellation(t *testing.T) {
	started := make(chan struct{})
	apiFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := ListNWVersions(ctx); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("list never started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("list ignored cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("list cancellation stuck")
	}
}

func TestAPISharedManagedLock(t *testing.T) {
	apiFixture(t, makeZIP(t, goodEntries()), nil)
	root := filepath.Join(t.TempDir(), "managed")
	release, err := AcquireManagedLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	lockPath := filepath.Join(root, ".nwjs-install.lock")
	before, err := os.Lstat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := AcquireManagedLock(root); err == nil {
		second()
		t.Fatal("shared lock is not exclusive")
	}
	if path, err := InstallNW(context.Background(), root, testVersion, nil); err == nil || path != "" || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("install ignored shared lock: %q / %v", path, err)
	}
	c := fixture(t, nil, nil)
	if code, _, diag := invoke(context.Background(), c, args(root)); code == 0 || !strings.Contains(diag, "lock") {
		t.Fatalf("CLI ignored shared lock: %d / %s", code, diag)
	}
	release()
	release() // Safe to defer and also release early.
	if _, err := InstallNW(context.Background(), root, testVersion, func(p Progress) error {
		if second, err := AcquireManagedLock(root); err == nil {
			second()
			t.Fatal("shared lock ignored active install")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(lockPath)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("persistent lock inode replaced: %v", err)
	}
}

func TestAPIManagedLockValidation(t *testing.T) {
	for _, root := range []string{"", "relative", "/", "/tmp/invalid\x00root"} {
		if release, err := AcquireManagedLock(root); err == nil {
			release()
			t.Fatalf("invalid root accepted: %q", root)
		}
	}
	for _, kind := range []string{"root-symlink", "writable-root", "lock-symlink", "lock-hardlink", "lock-mode", "lock-directory", "lock-fifo"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			lock := filepath.Join(root, ".nwjs-install.lock")
			var err error
			switch kind {
			case "root-symlink":
				link := filepath.Join(t.TempDir(), "alias")
				err = os.Symlink(root, link)
				root = link
			case "writable-root":
				err = os.Chmod(root, 0777)
			case "lock-symlink":
				err = os.Symlink(filepath.Join(t.TempDir(), "outside"), lock)
			case "lock-hardlink":
				outside := filepath.Join(t.TempDir(), "outside")
				if err = os.WriteFile(outside, nil, 0600); err == nil {
					err = os.Link(outside, lock)
				}
			case "lock-mode":
				err = os.WriteFile(lock, nil, 0644)
			case "lock-directory":
				err = os.Mkdir(lock, 0700)
			case "lock-fifo":
				err = syscall.Mkfifo(lock, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if release, err := AcquireManagedLock(root); err == nil {
				release()
				t.Fatal("unsafe root/lock accepted")
			}
		})
	}
}

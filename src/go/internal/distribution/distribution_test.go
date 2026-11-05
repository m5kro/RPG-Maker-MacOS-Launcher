//go:build darwin

// Packaging tests use the real repository resources and system signing tools.
// Only the expensive GUI compilation is replaced by a tiny real arm64 Go binary.
package distribution

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, p string) []byte { t.Helper(); b, e := os.ReadFile(p); must(t, e); return b }
func put(t *testing.T, p, s string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(p), 0700))
	must(t, os.WriteFile(p, []byte(s), 0700))
}
func command(t *testing.T, name string, args ...string) string {
	t.Helper()
	b, e := exec.Command(name, args...).CombinedOutput()
	if e != nil {
		t.Fatalf("%s: %v\n%s", name, e, b)
	}
	return strings.TrimSpace(string(b))
}
func repo(t *testing.T) string {
	t.Helper()
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(f), "../../../.."))
}

var assets = map[string]string{
	"LICENSE":                                  "Contents/Resources/licenses/Launcher.txt",
	"packaging/macos/arm64/Info.plist":         "Contents/Info.plist",
	"packaging/macos/arm64/EasyRPG-Standalone": "Contents/Resources/EasyRPG-Standalone",
	"assets/icons/icon.icns":                   "Contents/Resources/icon.icns",
	"src/javascript/bg.js":                     "Contents/Resources/bg.js",
	"src/javascript/disable-child.js":          "Contents/Resources/disable-child.js",
	"src/javascript/disable-net.js":            "Contents/Resources/disable-net.js",
	"src/webui/index.html":                     "Contents/Resources/webui/index.html",
	"src/webui/app.js":                         "Contents/Resources/webui/app.js",
	"src/webui/style.css":                      "Contents/Resources/webui/style.css",
}

func TestRepositoryDistributionResources(t *testing.T) {
	r := repo(t)
	for p := range assets {
		if len(read(t, filepath.Join(r, p))) == 0 {
			t.Fatal(p)
		}
	}
	for _, p := range []string{"assets/icons/icon.png", "assets/icons/colorless-icon.png", "assets/icons/colorless-icon.icns", "assets/icons/old-icon.icns", "src/go/go.mod", "src/go/cmd/launcher/main.go", "src/go/cmd/validate-mz/main.go", "src/go/cmd/nwjs-helper/main.go"} {
		read(t, filepath.Join(r, p))
	}
	standalone := string(read(t, filepath.Join(r, "packaging/macos/arm64/EasyRPG-Standalone")))
	if !strings.Contains(standalone, `/usr/bin/arch -arm64 "${0%/*}/EasyRPG Player.app/Contents/MacOS/EasyRPG Player"`) {
		t.Fatal("standalone architecture")
	}
	for _, p := range []string{"scripts/shell/build-launcher.sh", "scripts/shell/build-nwjs-helper.sh"} {
		b := string(read(t, filepath.Join(r, p)))
		if !strings.Contains(b, "GOARCH=arm64") {
			t.Fatal(p)
		}
	}
}

func TestGoToolingTasks(t *testing.T) {
	r := repo(t)
	mise := string(read(t, filepath.Join(r, "mise.toml")))
	for _, contract := range []string{`go = "1.27.3"`, "mod download", "mod verify", "test -count=1", "test -race -count=1", "src/go vet", "build-launcher.sh", "smoke-launcher.sh", "./cmd/validate-mz", "dist/RPG-Maker-Launcher.app/Contents/MacOS/launcher"} {
		if !strings.Contains(mise, contract) {
			t.Fatal("missing task contract", contract)
		}
	}
}

type fixture struct{ root, tools, log, binary, goroot, wails string }

func newFixture(t *testing.T) fixture {
	t.Helper()
	r := repo(t)
	f := fixture{root: t.TempDir(), tools: t.TempDir(), log: filepath.Join(t.TempDir(), "go.log")}
	// Resolve /var aliases: output paths must be physical, not symlink spellings.
	var e error
	f.root, e = filepath.EvalSymlinks(f.root)
	must(t, e)
	for p := range assets {
		put(t, filepath.Join(f.root, p), string(read(t, filepath.Join(r, p))))
	}
	put(t, filepath.Join(f.root, "scripts/shell/build-launcher.sh"), string(read(t, filepath.Join(r, "scripts/shell/build-launcher.sh"))))
	must(t, os.MkdirAll(filepath.Join(f.root, "src/go"), 0700))
	f.goroot = command(t, "go", "env", "GOROOT")
	f.wails = command(t, "go", "-C", filepath.Join(r, "src/go"), "list", "-m", "-f", "{{.Dir}}", "github.com/wailsapp/wails/v3")
	source := filepath.Join(t.TempDir(), "main.go")
	put(t, source, "package main\nfunc main() {}\n")
	f.binary = filepath.Join(t.TempDir(), "launcher")
	command(t, "go", "build", "-o", f.binary, source)
	put(t, filepath.Join(f.tools, "go"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$TEST_GO_LOG"
case "$*" in
 'env GOROOT') printf '%s\n' "$TEST_GOROOT"; exit;;
 *'list -m'*) printf '%s\n' "$TEST_WAILS"; exit;;
esac
[ "${TEST_BUILD_FAIL:-}" != 1 ] || exit 71
[ "$CGO_ENABLED/$GOOS/$GOARCH" = 1/darwin/arm64 ]
while [ "$#" -gt 0 ]; do
 if [ "$1" = -o ]; then cp "$TEST_BINARY" "$2"; exit; fi
 shift
done
exit 72
`)
	return f
}
func (f fixture) run(t *testing.T, extra []string, args ...string) ([]byte, error) {
	t.Helper()
	c := exec.Command("/bin/sh", append([]string{filepath.Join(f.root, "scripts/shell/build-launcher.sh")}, args...)...)
	c.Dir = "/"
	c.Env = append(os.Environ(), "PATH="+f.tools+":"+os.Getenv("PATH"), "TEST_GO_LOG="+f.log, "TEST_BINARY="+f.binary, "TEST_GOROOT="+f.goroot, "TEST_WAILS="+f.wails)
	c.Env = append(c.Env, extra...)
	return c.CombinedOutput()
}
func tree(t *testing.T, p string) string {
	t.Helper()
	h := sha256.New()
	must(t, filepath.Walk(p, func(path string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(p, path)
		if e != nil {
			return e
		}
		fmt.Fprintf(h, "%s %s\n", rel, i.Mode())
		if i.Mode().IsRegular() {
			h.Write(read(t, path))
		}
		if i.Mode()&os.ModeSymlink != 0 {
			s, e := os.Readlink(path)
			if e != nil {
				return e
			}
			h.Write([]byte(s))
		}
		return nil
	}))
	return fmt.Sprintf("%x", h.Sum(nil))
}
func (f fixture) bundle(t *testing.T, p string) {
	t.Helper()
	for src, dest := range assets {
		if !bytes.Equal(read(t, filepath.Join(f.root, src)), read(t, filepath.Join(p, dest))) {
			t.Fatal("bundle bytes", src)
		}
	}
	goLicense := filepath.Join(f.goroot, "LICENSE")
	if _, e := os.Stat(goLicense); e != nil {
		goLicense = filepath.Join(f.goroot, "../LICENSE")
	}
	for src, dest := range map[string]string{goLicense: "Go.txt", filepath.Join(f.wails, "LICENSE"): "Wails.txt"} {
		if !bytes.Equal(read(t, src), read(t, filepath.Join(p, "Contents/Resources/licenses", dest))) {
			t.Fatal(dest)
		}
	}
	if command(t, "/usr/bin/lipo", "-archs", filepath.Join(p, "Contents/MacOS/launcher")) != "arm64" {
		t.Fatal("not arm64")
	}
	command(t, "/usr/bin/plutil", "-lint", filepath.Join(p, "Contents/Info.plist"))
	command(t, "/usr/bin/codesign", "--verify", "--strict", p)
	for key, want := range map[string]string{"CFBundleExecutable": "launcher", "CFBundleIconFile": "icon.icns", "LSMinimumSystemVersion": "13.3", "LSArchitecturePriority.0": "arm64"} {
		if command(t, "/usr/bin/plutil", "-extract", key, "raw", "-o", "-", filepath.Join(p, "Contents/Info.plist")) != want {
			t.Fatal(key)
		}
	}
}
func TestBuildModesAndOutputContract(t *testing.T) {
	f := newFixture(t)
	for _, mode := range []string{"production", "smoke"} {
		t.Run(mode, func(t *testing.T) {
			out := filepath.Join(f.root, "outputs", mode)
			name := "A Launcher.app"
			b, e := f.run(t, nil, mode, name, out)
			if e != nil {
				t.Fatalf("build: %v\n%s", e, b)
			}
			if !strings.HasSuffix(strings.TrimSpace(string(b)), filepath.Join(out, name)) {
				t.Fatal(string(b))
			}
			f.bundle(t, filepath.Join(out, name))
			before := tree(t, out)
			if b, e = f.run(t, nil, mode, name, out); e == nil {
				t.Fatal("arbitrary output overwritten", string(b))
			}
			if tree(t, out) != before {
				t.Fatal("refusal changed output")
			}
		})
	}
	b, e := f.run(t, nil, "smoke", "Legacy.app")
	if e != nil {
		t.Fatalf("two arguments: %v %s", e, b)
	}
	f.bundle(t, filepath.Join(f.root, "build/validation/python-zero/gui/Legacy.app"))
	log := string(read(t, f.log))
	for _, flag := range []string{"-C " + filepath.Join(f.root, "src/go"), "-tags production -trimpath", "-tags production,smoke -trimpath", "-ldflags=-extldflags=-mmacosx-version-min=13.3", "./cmd/launcher"} {
		if !strings.Contains(log, flag) {
			t.Fatal("missing flag", flag, log)
		}
	}
}
func TestUnsafeOutputsLeaveTreeUnchanged(t *testing.T) {
	f := newFixture(t)
	out := filepath.Join(f.root, "dist")
	put(t, filepath.Join(out, "RPG-Maker-Launcher.app/sentinel"), "old")
	must(t, os.Symlink(out, filepath.Join(f.root, "link")))
	must(t, os.Symlink("/missing", filepath.Join(out, "dangling.app")))
	cases := [][]string{{"bad"}, {"production", "../bad.app", out}, {"production", "bad\n.app", out}, {"production", "a.app", "relative"}, {"production", "a.app", out + "/../dist"}, {"production", "a.app", filepath.Join(f.root, "link/sub")}, {"production", "dangling.app", out}, {"production", "a.app", out, "extra"}, {"smoke", "RPG-Maker-Launcher.app", out}}
	for _, args := range cases {
		before := tree(t, f.root)
		b, e := f.run(t, nil, args...)
		if e == nil {
			t.Fatalf("accepted %q: %s", args, b)
		}
		if tree(t, f.root) != before {
			t.Fatal("unsafe output changed tree", args)
		}
	}
	must(t, os.RemoveAll(filepath.Join(out, "RPG-Maker-Launcher.app")))
	must(t, os.Symlink(filepath.Join(f.root, "LICENSE"), filepath.Join(out, "RPG-Maker-Launcher.app")))
	before := tree(t, f.root)
	if b, e := f.run(t, nil, "production", "RPG-Maker-Launcher.app", out); e == nil {
		t.Fatal(string(b))
	}
	if tree(t, f.root) != before {
		t.Fatal("canonical symlink changed")
	}
}
func TestPublishCollisionNeverMergesIntoExistingApp(t *testing.T) {
	f := newFixture(t)
	out := filepath.Join(f.root, "outputs")
	put(t, filepath.Join(f.tools, "mv"), `#!/bin/sh
if [ "$1" = -n ]; then
  parent=$(dirname -- "$(dirname -- "$2")")
  dest="$parent/$(basename -- "$2")"
  mkdir "$dest"
  printf 'user app' > "$dest/sentinel"
fi
exec /bin/mv "$@"
`)
	b, err := f.run(t, nil, "production", "Existing.app", out)
	if err == nil {
		t.Fatalf("publish collision accepted: %s", b)
	}
	entries, err := os.ReadDir(filepath.Join(out, "Existing.app"))
	must(t, err)
	if len(entries) != 1 || entries[0].Name() != "sentinel" {
		t.Fatal("merged into user app", entries)
	}
	if string(read(t, filepath.Join(out, "Existing.app/sentinel"))) != "user app" {
		t.Fatal("user app changed")
	}
}

func TestSmokeCopyPreservesReviewedRuntime(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	put(t, filepath.Join(root, "scripts/shell/smoke-launcher.sh"), string(read(t, filepath.Join(repo(t), "scripts/shell/smoke-launcher.sh"))))
	put(t, filepath.Join(root, "scripts/shell/build-launcher.sh"), `#!/bin/sh
set -eu
mkdir -p "$3/$2/Contents/MacOS"
printf '#!/bin/sh\nexit 0\n' > "$3/$2/Contents/MacOS/launcher"
chmod +x "$3/$2/Contents/MacOS/launcher"
`)
	source := filepath.Join(root, "v0.94.1")
	put(t, filepath.Join(source, "nwjs.app/Contents/MacOS/nwjs"), "reviewed fixture; never execute")
	must(t, os.Chmod(filepath.Join(source, "nwjs.app"), 0755))
	must(t, os.Symlink("nwjs.app", filepath.Join(source, "link")))
	before := tree(t, source)
	work := filepath.Join(root, "fresh")
	c := exec.Command("/bin/sh", filepath.Join(root, "scripts/shell/smoke-launcher.sh"), work, source, "--reviewed-runtime")
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("smoke task: %v %s", err, b)
	}
	if tree(t, source) != before || tree(t, filepath.Join(work, "root/v0.94.1")) != before {
		t.Fatal("runtime copy changed bytes/modes/links")
	}
}

func TestSmokeRequiresConsentAndFreshPhysicalInputs(t *testing.T) {
	r := repo(t)
	work := filepath.Join(t.TempDir(), "fresh")
	for _, args := range [][]string{nil, {work, "/missing/v0.94.1"}, {work, "/missing/v0.94.1", "--reviewed-runtime"}, {"relative", "/missing/v0.94.1", "--reviewed-runtime"}} {
		c := exec.Command("/bin/sh", append([]string{filepath.Join(r, "scripts/shell/smoke-launcher.sh")}, args...)...)
		if b, err := c.CombinedOutput(); err == nil {
			t.Fatalf("accepted %q: %s", args, b)
		}
		if _, err := os.Lstat(work); !os.IsNotExist(err) {
			t.Fatal("invalid inputs created root", err)
		}
	}
}

func TestCanonicalReplacementAndRollback(t *testing.T) {
	f := newFixture(t)
	out := filepath.Join(f.root, "dist")
	app := filepath.Join(out, "RPG-Maker-Launcher.app")
	put(t, filepath.Join(app, "Contents/Frameworks/libpython.dylib"), "old python")
	put(t, filepath.Join(app, "Contents/Frameworks/QtWidgets/old"), "old Qt")
	args := []string{"production", "RPG-Maker-Launcher.app", out}
	old := tree(t, out)
	b, e := f.run(t, []string{"TEST_BUILD_FAIL=1"}, args...)
	if e == nil || tree(t, out) != old {
		t.Fatalf("build failure touched old bundle %v %s", e, b)
	}
	// Test-only PATH interception; no production arbitrary-command hooks.
	put(t, filepath.Join(f.tools, "mv"), `#!/bin/sh
SOURCE=$1
[ "$SOURCE" != -n ] || SOURCE=$2
case "$SOURCE" in
 *'.launcher-build.'*) exit 73;;
 *'.launcher-backup.'*) [ "${TEST_RESTORE_FAIL:-}" != 1 ] || exit 74;;
esac
exec /bin/mv "$@"
`)
	b, e = f.run(t, nil, args...)
	if e == nil || tree(t, out) != old {
		t.Fatalf("rollback failed: %v %s", e, b)
	}
	b, e = f.run(t, []string{"TEST_RESTORE_FAIL=1"}, args...)
	if e == nil {
		t.Fatal("restore failure accepted")
	}
	backups, e := filepath.Glob(filepath.Join(out, ".launcher-backup.*", "RPG-Maker-Launcher.app"))
	must(t, e)
	if len(backups) != 1 {
		t.Fatalf("lost recovery backup: %v %s", backups, b)
	}
	if string(read(t, filepath.Join(backups[0], "Contents/Frameworks/libpython.dylib"))) != "old python" {
		t.Fatal("old bytes lost")
	}
	must(t, os.Rename(backups[0], app))
	must(t, os.Remove(filepath.Dir(backups[0])))
	must(t, os.Remove(filepath.Join(f.tools, "mv")))
	for i := 0; i < 2; i++ {
		b, e = f.run(t, nil, args...)
		if e != nil {
			t.Fatalf("repeat %d: %v %s", i, e, b)
		}
		f.bundle(t, app)
	}
	entries, e := os.ReadDir(out)
	must(t, e)
	if len(entries) != 1 {
		t.Fatal("stage/backup/lock leaked", entries)
	}
}

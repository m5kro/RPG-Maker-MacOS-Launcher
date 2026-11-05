//go:build nativeacceptance

package launcher

// Opt-in real-artifact acceptance. No downloads; missing inputs fail, never skip.
// Run from src/go with RPGM_NATIVE_REPO set to the absolute reviewed checkout.
import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const nativeFixtureHash = "ed4ee4c57d1c7b4ababf8900c8e4eb2fc4df1a3094911a6cefa66209810d2b7c"
const nativeFixtureTop = "rpgmaker-mz-demo-81abd876e3c339fbd7d74323ad8817ad7011cade"

func nativeRoot(t *testing.T) (string, string) {
	t.Helper()
	repo := os.Getenv("RPGM_NATIVE_REPO")
	if !filepath.IsAbs(repo) || filepath.Clean(repo) != repo {
		t.Fatal("RPGM_NATIVE_REPO must be the absolute reviewed checkout; missing fixtures are failures")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("native acceptance requires darwin/arm64")
	}
	parent := filepath.Join(repo, "build/validation/python-zero/native-backend")
	if evidence := os.Getenv("RPGM_NATIVE_EVIDENCE"); evidence != "" {
		if !filepath.IsAbs(evidence) || filepath.Clean(evidence) != evidence || !within(filepath.Join(repo, "build/validation"), evidence) {
			t.Fatal("RPGM_NATIVE_EVIDENCE must be an absolute directory under build/validation")
		}
		parent = evidence
	}
	nativeMust(t, os.MkdirAll(parent, 0700))
	resolved, err := filepath.EvalSymlinks(parent)
	nativeMust(t, err)
	if resolved != parent {
		t.Fatal("evidence directory must not contain symlinks")
	}
	root, err := os.MkdirTemp(parent, t.Name()+"-")
	nativeMust(t, err)
	t.Logf("evidence=%s", root)
	return repo, root
}
func nativeMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func nativeJSON(t *testing.T, p string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	nativeMust(t, err)
	nativeMust(t, os.WriteFile(p, append(b, '\n'), 0600))
}
func nativeHash(t *testing.T, p string) string {
	t.Helper()
	f, err := os.Open(p)
	nativeMust(t, err)
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	nativeMust(t, err)
	return fmt.Sprintf("%x", h.Sum(nil))
}
func nativeTree(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		fmt.Fprintf(h, "%s\x00%s\x00", rel, info.Mode())
		if info.Mode().IsRegular() {
			fmt.Fprint(h, nativeHash(t, p))
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(p)
			if e != nil {
				return e
			}
			fmt.Fprint(h, target)
		}
		fmt.Fprintln(h)
		return nil
	})
	nativeMust(t, err)
	return fmt.Sprintf("%x", h.Sum(nil))
}
func nativeCommand(t *testing.T, program string, args ...string) []byte {
	t.Helper()
	b, err := exec.Command(program, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %q: %v\n%s", program, args, err, b)
	}
	return b
}

type nativeTransport func(*http.Request) (*http.Response, error)

func (f nativeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNativeComponentArtifacts(t *testing.T) {
	repo, root := nativeRoot(t)
	for _, v := range []struct {
		component             Component
		archive, hash, source string
	}{
		{ComponentMKXPZ, "Z-universal.zip", "7bc1ef8ea8e4d5f2a75e3147289538677a4fc7600895494f6d0e2f66819079f9", componentRelease + "Z-universal.zip"},
		{ComponentEasyRPG, "EasyRPG-1969.zip", "8896d541dfb7f3a5176fb38d03ce4e56e43679cf1a545f0172a1ba2f4513ab4f", easyJob + "1969/artifact/builds/desktop/Player/EasyRPG-Player-macos.app.zip"},
	} {
		t.Run(string(v.component), func(t *testing.T) {
			source := filepath.Join(repo, "build/validation/arm-only-independent", v.archive)
			before := nativeHash(t, source)
			if before != v.hash {
				t.Fatal("cached official ZIP hash mismatch", before)
			}
			dest := filepath.Join(root, string(v.component))
			nativeMust(t, os.Mkdir(dest, 0700))
			upstream := v.source
			if v.component == ComponentEasyRPG {
				upstream = easyJob + "1969/"
			}
			priorSource := filepath.Join(repo, "build/validation/arm-only-review", v.archive)
			if nativeHash(t, priorSource) != before {
				t.Fatal("prior ARM review artifact differs")
			}
			report := map[string]any{"source": source, "upstream_reference": upstream, "replay_download_url": v.source, "sha256": before, "prior_review_source": priorSource, "prior_review_hash_matches": true, "mode": "local HTTP transport replay of prior downloaded bytes; no live download", "metadata_note": "MKXP SHA and EasyRPG artifact relativePath are synthetic replay metadata, not historical provenance; build 1969 and ZIP hashes are pinned", "passed": false}
			defer func() {
				after := nativeHash(t, source)
				report["source_unchanged"] = after == before
				if after != before {
					t.Error("original ZIP changed")
				}
				report["passed"] = !t.Failed()
				nativeJSON(t, filepath.Join(dest, "result.json"), report)
			}()
			z, err := zip.OpenReader(source)
			nativeMust(t, err)
			defer z.Close()
			dirs, links, files := 0, 0, 0
			var structure []string
			for _, f := range z.File {
				if f.Mode().IsDir() {
					dirs++
				} else if f.Mode()&os.ModeSymlink != 0 {
					links++
				} else {
					files++
				}
				structure = append(structure, fmt.Sprintf("%s %d %s", f.Mode(), f.UncompressedSize64, f.Name))
			}
			report["zip_directories"] = dirs
			report["zip_symlinks"] = links
			report["zip_regular_files"] = files
			nativeMust(t, os.WriteFile(filepath.Join(dest, "zip-structure.txt"), []byte(strings.Join(structure, "\n")), 0600))
			m, err := NewManager(filepath.Join(dest, "managed"), "", nil)
			nativeMust(t, err)
			var requests []string
			m.client = &http.Client{Transport: nativeTransport(func(r *http.Request) (*http.Response, error) {
				if err := r.Context().Err(); err != nil {
					return nil, err
				}
				requests = append(requests, r.URL.String())
				var text string
				switch r.URL.String() {
				case "https://api.github.com/repos/m5kro/mkxp-z/commits/dev":
					text = `{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
				case easyJob + "lastSuccessfulBuild/api/json":
					text = `{"number":1969,"result":"SUCCESS","building":false,"artifacts":[{"relativePath":"builds/desktop/Player/EasyRPG-Player-macos.app.zip"}]}`
				case v.source:
					f, e := os.Open(source)
					if e != nil {
						return nil, e
					}
					info, e := f.Stat()
					if e != nil {
						f.Close()
						return nil, e
					}
					return &http.Response{StatusCode: 200, Body: f, ContentLength: info.Size(), Header: make(http.Header), Request: r}, nil
				default:
					return nil, fmt.Errorf("unapproved replay request: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(text)), ContentLength: int64(len(text)), Header: make(http.Header), Request: r}, nil
			})}
			defer func() { report["requests"] = requests }()
			// Existing official bundle plus a marker models prior-install retention (not an older release).
			nativeCommand(t, "/usr/bin/ditto", "-x", "-k", source, m.Root)
			old := filepath.Join(m.Root, componentName(v.component))
			marker := filepath.Join(old, "previous-install-marker")
			nativeMust(t, os.WriteFile(marker, []byte("preserve on failure/cancel"), 0600))
			oldHash := nativeTree(t, old)
			report["prior_install_tree_sha256"] = oldHash
			for _, phase := range []string{"download", "extract", "extract-written"} {
				ctx, cancel := context.WithCancel(context.Background())
				seen := false
				var at Progress
				_, e := m.InstallComponent(ctx, v.component, func(p Progress) error {
					if p.Phase == phase || (phase == "extract-written" && p.Phase == "extract" && p.Completed > 0) {
						seen = true
						at = p
						cancel()
					}
					return nil
				})
				cancel()
				preserved := nativeTree(t, old) == oldHash
				stages, stageErr := filepath.Glob(filepath.Join(m.Root, ".runtime-stage-*"))
				nativeMust(t, stageErr)
				if len(stages) != 0 {
					t.Error("cancel left owned staging", stages)
				}
				report["cancel_"+phase] = map[string]any{"reached": seen, "error": fmt.Sprint(e), "old_preserved": preserved, "remaining_stages": stages, "progress_at_cancel": at}
				if e == nil || !preserved {
					t.Errorf("cancel %s: err=%v preserved=%v", phase, e, preserved)
				}
				if !seen {
					t.Errorf("cancel phase %s unreachable with real ZIP: %v", phase, e)
				}
				if seen && !errors.Is(e, context.Canceled) {
					t.Errorf("cancel not propagated: %v", e)
				}
			}
			result, err := m.InstallComponent(context.Background(), v.component, nil)
			report["install_result"] = result
			report["install_error"] = fmt.Sprint(err)
			if err != nil {
				report["old_preserved_on_failure"] = nativeTree(t, old) == oldHash
				if report["old_preserved_on_failure"] != true {
					t.Error("failed install changed existing bundle")
				}
			} else {
				_, markerErr := os.Lstat(marker)
				report["old_marker_replaced_on_success"] = errors.Is(markerErr, os.ErrNotExist)
				if !errors.Is(markerErr, os.ErrNotExist) {
					t.Error("new bundle was not published")
				}
			}
			stages, e := filepath.Glob(filepath.Join(m.Root, ".runtime-stage-*"))
			nativeMust(t, e)
			report["remaining_stages"] = stages
			if len(stages) != 0 {
				t.Error("installer leaked staging", stages)
			}
			// Audit actual official Mach-O files even when publication is blocked.
			var binaries []map[string]any
			nativeMust(t, filepath.WalkDir(old, func(p string, d fs.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.Type().IsRegular() && (strings.Contains(p, "/MacOS/") || strings.HasSuffix(p, ".dylib")) {
					var e error
					arches := strings.Fields(string(nativeCommand(t, "/usr/bin/lipo", "-archs", p)))
					arm := false
					for _, arch := range arches {
						arm = arm || arch == "arm64"
					}
					if !arm {
						e = errors.New("official Mach-O has no arm64 slice")
					}
					if strings.Contains(p, "/MacOS/") {
						e = errors.Join(e, RequireARM64(p))
					}
					binaries = append(binaries, map[string]any{"path": strings.TrimPrefix(p, m.Root+"/"), "arm64_error": fmt.Sprint(e), "arches": arches, "file": string(nativeCommand(t, "/usr/bin/file", p))})
					if e != nil {
						t.Error(e)
					}
				}
				return nil
			}))
			report["binaries"] = binaries
			signature, sigErr := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", "--verbose=2", old).CombinedOutput()
			report["signature_error"] = fmt.Sprint(sigErr)
			nativeMust(t, os.WriteFile(filepath.Join(dest, "codesign.log"), signature, 0600))
			if err != nil {
				t.Fatalf("real official ZIP rejected: %v", err)
			}
			nativeMust(t, checkComponent(result.Path, v.component))
			if v.component == ComponentEasyRPG && result.BuildNumber != 1969 {
				t.Error("wrong build number", result)
			}
			// Metadata replay SHA is deliberately synthetic, not a claim about artifact provenance.
			if v.component == ComponentMKXPZ && result.CommitSHA != strings.Repeat("a", 40) {
				t.Error("metadata propagation failed", result)
			}
			dataFiles, ignoredMetadata := 0, 0
			var dataBytes uint64
			for _, f := range z.File {
				if strings.HasPrefix(f.Name, "__MACOSX/") {
					// Independently inspect all resource-fork descriptors in the pinned fixture.
					r, e := f.Open()
					nativeMust(t, e)
					b, e := readBounded(r, 64<<10)
					nativeMust(t, errors.Join(e, r.Close()))
					if len(b) < 26 || binary.BigEndian.Uint32(b[:4]) != 0x00051607 {
						t.Fatal("fixture metadata is not AppleDouble", f.Name)
					}
					for i := 0; i < int(binary.BigEndian.Uint16(b[24:26])); i++ {
						offset := 26 + i*12
						if offset+12 > len(b) {
							t.Fatal("truncated AppleDouble table", f.Name)
						}
						if binary.BigEndian.Uint32(b[offset:offset+4]) == 2 && binary.BigEndian.Uint32(b[offset+8:offset+12]) != 0 {
							t.Fatal("nonempty resource fork must not be discarded", f.Name)
						}
					}
					ignoredMetadata++
					continue
				}
				p := filepath.Join(m.Root, filepath.FromSlash(f.Name))
				info, e := os.Lstat(p)
				if e != nil {
					t.Error(e)
					continue
				}
				if f.Mode().Type() != info.Mode().Type() {
					t.Errorf("type changed: %s zip=%s disk=%s", f.Name, f.Mode(), info.Mode())
				}
				if f.Mode().IsRegular() {
					r, e := f.Open()
					nativeMust(t, e)
					h := sha256.New()
					_, e = io.Copy(h, r)
					nativeMust(t, errors.Join(e, r.Close()))
					if nativeHash(t, p) != fmt.Sprintf("%x", h.Sum(nil)) {
						t.Errorf("extracted bytes differ: %s", f.Name)
					}
					dataFiles++
					dataBytes += f.UncompressedSize64
				}
				if f.Mode()&os.ModeSymlink != 0 {
					r, e := f.Open()
					nativeMust(t, e)
					b, e := io.ReadAll(r)
					r.Close()
					nativeMust(t, e)
					target, e := os.Readlink(p)
					nativeMust(t, e)
					if target != string(b) {
						t.Errorf("symlink changed: %s", p)
					}
				}
			}
			if _, e := os.Lstat(filepath.Join(m.Root, "__MACOSX")); !errors.Is(e, os.ErrNotExist) {
				t.Error("AppleDouble metadata was written to managed root", e)
			}
			report["compared_data_files"] = dataFiles
			report["compared_data_bytes"] = dataBytes
			report["ignored_appledouble_entries"] = ignoredMetadata
			report["nonempty_resource_forks"] = 0
		})
	}
}

func TestNativeGameResultWarning(t *testing.T) {
	_, root := nativeRoot(t)
	p := filepath.Join(root, "package.json")
	nativeMust(t, os.WriteFile(p, []byte(`{"name":"warning-probe","main":"index.html","bg-script":"user-owned.js"}`), 0600))
	before := nativeHash(t, p)
	result, err := PreparePackage(root, "", false)
	nativeMust(t, err)
	nativeJSON(t, filepath.Join(root, "warning.json"), result)
	if !result.OK || len(result.Warnings) == 0 || before != nativeHash(t, p) {
		t.Fatal("warning propagation/preservation failed", result)
	}
}

package launcher

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestUnsupportedHostHasNoSideEffects(t *testing.T) {
	production := officialConfig()
	if production.hostOS != runtime.GOOS || production.hostArch != runtime.GOARCH {
		t.Fatal("production host must come from Go runtime, not user configuration")
	}
	for _, host := range [][2]string{{"darwin", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}, {"", ""}} {
		t.Run(strings.Join(host[:], "/"), func(t *testing.T) {
			c := fixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				t.Error("unsupported host reached network")
				w.WriteHeader(500)
			})
			c.hostOS, c.hostArch = host[0], host[1]
			root := filepath.Join(t.TempDir(), "managed")
			for _, a := range [][]string{{"versions"}, args(root)} {
				code, out, diag := invoke(context.Background(), c, a)
				if code == 0 || out != "" || !strings.Contains(diag, "requires macOS arm64") {
					t.Errorf("unsupported host: %d %q %q", code, out, diag)
				}
				if _, err := os.Lstat(root); !os.IsNotExist(err) {
					t.Fatalf("unsupported host created managed root: %v", err)
				}
			}
		})
	}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		var data []byte
		if info.Mode().IsRegular() {
			data, err = os.ReadFile(path)
		} else if info.Mode()&os.ModeSymlink != 0 {
			var link string
			link, err = os.Readlink(path)
			data = []byte(link)
		}
		result[path] = fmt.Sprintf("%v:%s", info.Mode(), data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestLegacyReceiptsAndMarkersPreserved(t *testing.T) {
	for _, kind := range []string{"x64", "x64-rosetta", "arm64-rosetta", "unknown-arch", "windows-receipt", "legacy", "marker", "dangling-marker"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, testVersion)
			exe := filepath.Join(target, "nwjs.app/Contents/MacOS/nwjs")
			if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
				t.Fatal(err)
			}
			r := receipt{Version: testVersion, Arch: "arm64"}
			cpu := macho.CpuArm64
			switch kind {
			case "x64", "x64-rosetta":
				r.Arch, cpu = "x64", macho.CpuAmd64
			case "unknown-arch":
				r.Arch = "unknown"
			}
			r.Rosetta = strings.HasSuffix(kind, "-rosetta")
			if err := os.WriteFile(exe, executable(cpu), 0755); err != nil {
				t.Fatal(err)
			}
			if kind != "legacy" {
				b, err := json.Marshal(r)
				if kind == "windows-receipt" {
					b = []byte(`{"version":"v0.100.0","os":"windows","arch":"x64","rosetta":false}`)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, ".nwjs-install.json"), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			marker := filepath.Join(target, "run-with-rosetta")
			if kind == "dangling-marker" {
				if err := os.Symlink("missing", marker); err != nil {
					t.Fatal(err)
				}
			} else if kind == "marker" || r.Rosetta {
				if err := os.WriteFile(marker, []byte("keep original marker"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshot(t, target)
			c := fixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				t.Error("legacy target reached network")
				w.WriteHeader(500)
			})
			code, out, diag := invoke(context.Background(), c, args(root))
			if code == 0 || out != "" || !strings.Contains(diag, "existing target preserved") {
				t.Fatalf("legacy target not safely rejected: %d %q %q", code, out, diag)
			}
			if !reflect.DeepEqual(before, snapshot(t, target)) {
				t.Fatal("existing files, receipt or marker changed")
			}
			noStaging(t, root)
		})
	}
}

func universalExecutable(cpus ...macho.Cpu) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(macho.MagicFat))
	_ = binary.Write(&b, binary.BigEndian, uint32(len(cpus)))
	offset := 8 + 20*len(cpus)
	for _, cpu := range cpus {
		size := len(executable(cpu))
		for _, v := range []uint32{uint32(cpu), 0, uint32(offset), uint32(size), 0} {
			_ = binary.Write(&b, binary.BigEndian, v)
		}
		offset += size
	}
	for _, cpu := range cpus {
		b.Write(executable(cpu))
	}
	return b.Bytes()
}

func TestARM64MachOValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		ok   bool
	}{
		{"arm64", executable(macho.CpuArm64), true},
		{"intel", executable(macho.CpuAmd64), false},
		{"universal-arm64", universalExecutable(macho.CpuAmd64, macho.CpuArm64), true},
		{"universal-intel-only", universalExecutable(macho.CpuAmd64), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			es := goodEntries()
			es[0].data = tc.data
			c := fixture(t, makeZIP(t, es), nil)
			root := t.TempDir()
			if !tc.ok {
				code, out, diag := invoke(context.Background(), c, args(root))
				if code == 0 || !strings.Contains(diag, "Mach-O") || strings.Contains(out, `"event":"complete"`) {
					t.Fatalf("Intel executable not rejected by Mach-O validation: %d %q %q", code, out, diag)
				}
				if _, err := os.Lstat(filepath.Join(root, testVersion)); !os.IsNotExist(err) {
					t.Fatalf("Intel target published: %v", err)
				}
				noStaging(t, root)
				return
			}
			for i := 0; i < 2; i++ {
				code, out, diag := invoke(context.Background(), c, args(root))
				if code != 0 {
					t.Fatalf("install/no-op: %d %q %q", code, out, diag)
				}
				if i == 1 && len(events(t, out)) != 1 {
					t.Fatalf("existing ARM bundle downloaded again: %s", out)
				}
			}
			target := filepath.Join(root, testVersion)
			b, err := os.ReadFile(filepath.Join(target, "nwjs.app/Contents/MacOS/nwjs"))
			if err != nil || !bytes.Equal(b, tc.data) {
				t.Fatal("executable rewritten (universal slices must be preserved)")
			}
			b, err = os.ReadFile(filepath.Join(target, ".nwjs-install.json"))
			if err != nil || string(b) != `{"version":"v0.100.0","arch":"arm64","rosetta":false}` {
				t.Fatalf("non-ARM receipt: %s %v", b, err)
			}
			if _, err := os.Lstat(filepath.Join(target, "run-with-rosetta")); !os.IsNotExist(err) {
				t.Fatalf("ARM install created a Rosetta marker: %v", err)
			}
		})
	}
}

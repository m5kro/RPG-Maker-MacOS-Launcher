package launcher

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The 163-byte AppleDouble used by the real Z-universal.zip: empty resource
// fork, zero FinderInfo and com.apple.provenance (no runtime data).
func componentAppleDouble(t *testing.T) []byte {
	t.Helper()
	b, err := hex.DecodeString("00051607000200004d6163204f5320582020202020202020000200000009000000320000007100000002000000a300000000000000000000000000000000000000000000000000000000000000000000000000004154545200000000000000a3000000980000000b00000000000000000000000000000001000000980000000b000015636f6d2e6170706c652e70726f76656e616e636500010200b4b77dcf47ff5615")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestComponentZIPAppleDouble(t *testing.T) {
	const app = "Z-universal.app"
	const meta = "__MACOSX/._Z-universal.app"
	ad := componentAppleDouble(t)
	base := []zipEntry{
		{app + "/", os.ModeDir | 0755, nil},
		{app + "/Contents/", os.ModeDir | 0755, nil},
		{app + "/Contents/MacOS/", os.ModeDir | 0755, nil},
		{app + "/Contents/MacOS/Z-universal", 0755, engineContractARM()},
	}
	good := []zipEntry{
		{meta, 0644, ad},
		{"__MACOSX/" + app + "/._Contents", 0644, ad},
		{"__MACOSX/" + app + "/Contents/MacOS/._Z-universal", 0644, ad},
	}
	cases := []struct {
		name      string
		extra     []zipEntry
		noApp, ok bool
	}{
		{name: "real-layout", extra: good, ok: true},
		{name: "metadata-before-app", extra: good, ok: true},
		{name: "metadata-only", extra: good, noApp: true},
		{name: "empty-zip", noApp: true},
		{name: "no-executable", extra: []zipEntry{base[0], good[0]}, noApp: true},
		{name: "other-root", extra: []zipEntry{{"Other.app/Contents/payload", 0644, ad}}},
		{name: "dotfile", extra: []zipEntry{{".DS_Store", 0644, ad}}},
		{name: "extra-payload", extra: []zipEntry{{"__MACOSX/payload", 0644, ad}}},
		{name: "wrong-app", extra: []zipEntry{{"__MACOSX/._Other.app", 0644, ad}}},
		{name: "orphan", extra: []zipEntry{{"__MACOSX/" + app + "/._missing", 0644, ad}}},
		{name: "case-alias-target", extra: []zipEntry{{"__MACOSX/._z-universal.app", 0644, ad}}},
		{name: "duplicate", extra: append(append([]zipEntry{}, good...), good[0])},
		{name: "case-duplicate", extra: []zipEntry{good[0], {"__MACOSX/._Z-UNIVERSAL.app", 0644, ad}}},
		{name: "symlink", extra: []zipEntry{{meta, os.ModeSymlink | 0777, []byte("../outside")}}},
		{name: "fifo", extra: []zipEntry{{meta, os.ModeNamedPipe | 0600, nil}}},
		{name: "metadata-directory", extra: []zipEntry{{meta + "/", os.ModeDir | 0755, nil}}},
		{name: "metadata-ancestor-link", extra: []zipEntry{{"__MACOSX/" + app, os.ModeSymlink | 0777, []byte("../" + app)}, good[1]}},
		{name: "not-appledouble", extra: []zipEntry{{meta, 0644, []byte("arbitrary payload")}}},
	}
	for _, p := range []string{"__MACOSX/../._Z-universal.app", "__MACOSX//._Z-universal.app", "__MACOSX/./._Z-universal.app", "/__MACOSX/._Z-universal.app", `__MACOSX\._Z-universal.app`, "__MACOSX/._Z-universal.app\x00", "__MACOSX/._C:drive", "__MACOSX/" + strings.Repeat("x", maxPath)} {
		cases = append(cases, struct {
			name      string
			extra     []zipEntry
			noApp, ok bool
		}{name: p, extra: []zipEntry{{p, 0644, ad}}})
	}
	mutations := map[string]func([]byte) []byte{
		"magic":         func(b []byte) []byte { b[0] = 1; return b },
		"version":       func(b []byte) []byte { b[5] = 3; return b },
		"entry-table":   func(b []byte) []byte { b[25] = 255; return b },
		"unknown-entry": func(b []byte) []byte { b[29] = 1; return b },
		"out-of-bounds": func(b []byte) []byte { b[30] = 255; return b },
		"duplicate-id":  func(b []byte) []byte { b[41] = 9; return b },
		"resource-fork": func(b []byte) []byte { b[49] = 1; return append(b, 42) },
		"finder-info":   func(b []byte) []byte { b[50] = 1; return b },
		"attr-magic":    func(b []byte) []byte { b[84] = 0; return b },
		"attr-range":    func(b []byte) []byte { b[120] = 255; return b },
		"unknown-xattr": func(b []byte) []byte {
			return bytes.Replace(b, []byte("com.apple.provenance"), []byte("com.apple.quarantinE"), 1)
		},
	}
	for name, mutate := range mutations {
		cases = append(cases, struct {
			name      string
			extra     []zipEntry
			noApp, ok bool
		}{name: name, extra: []zipEntry{{meta, 0644, mutate(bytes.Clone(ad))}}})
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := engineContractManager(t)
			engineContractRuntime(t, m, MKXPZ)
			marker := filepath.Join(m.Root, app, "keep")
			engineContractWrite(t, marker, []byte("old"), 0600)
			entries := append([]zipEntry{}, base...)
			if tt.noApp {
				entries = nil
			}
			if tt.name == "metadata-before-app" {
				entries = append(append([]zipEntry{}, tt.extra...), entries...)
			} else {
				entries = append(entries, tt.extra...)
			}
			body := makeZIP(t, entries)
			engineContractHTTP(t, m, body, "")
			result, err := m.InstallComponent(context.Background(), ComponentMKXPZ, nil)
			if tt.ok {
				if err != nil {
					t.Fatal(err)
				}
				b, e := os.ReadFile(filepath.Join(result.Path, "Contents/MacOS/Z-universal"))
				if e != nil || !bytes.Equal(b, engineContractARM()) {
					t.Fatal("data fork changed", e)
				}
				if _, e = os.Lstat(marker); !errors.Is(e, os.ErrNotExist) {
					t.Fatal("old install not replaced", e)
				}
			} else {
				if err == nil {
					t.Fatal("unsafe component ZIP accepted")
				}
				if b, e := os.ReadFile(marker); e != nil || string(b) != "old" {
					t.Fatal("old install changed", e)
				}
			}
			if _, e := os.Lstat(filepath.Join(m.Root, "__MACOSX")); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("metadata published", e)
			}
			noStaging(t, m.Root)
		})
	}
}

func TestComponentZIPMetadataLimits(t *testing.T) {
	for _, kind := range []string{"size", "combined-size", "count", "crc", "metadata-cap", "understated-size"} {
		t.Run(kind, func(t *testing.T) {
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			root := &zip.FileHeader{Name: "Z-universal.app/"}
			root.SetMode(os.ModeDir | 0755)
			if _, err := z.CreateHeader(root); err != nil {
				t.Fatal(err)
			}
			count := 1
			if kind == "count" {
				count = maxEntries + 1
			}
			if kind == "combined-size" {
				h := &zip.FileHeader{Name: "Z-universal.app/data", Method: zip.Store, UncompressedSize64: maxExtracted, CompressedSize64: 1}
				h.SetMode(0600)
				w, e := z.CreateRaw(h)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = w.Write([]byte{0}); e != nil {
					t.Fatal(e)
				}
			}
			data := componentAppleDouble(t)
			if kind == "metadata-cap" {
				data = append(data, make([]byte, (64<<10)+1-len(data))...)
			}
			for i := 0; i < count; i++ {
				h := &zip.FileHeader{Name: "__MACOSX/._Z-universal.app", Method: zip.Store, UncompressedSize64: uint64(len(data)), CompressedSize64: uint64(len(data)), CRC32: crc32.ChecksumIEEE(data)}
				h.SetMode(0600)
				switch kind {
				case "size":
					h.UncompressedSize64 = maxExtracted + 1
				case "crc":
					h.CRC32++
				case "understated-size":
					h.UncompressedSize64--
				}
				w, e := z.CreateRaw(h)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = w.Write(data); e != nil {
					t.Fatal(e)
				}
			}
			if e := z.Close(); e != nil {
				t.Fatal(e)
			}
			m := engineContractManager(t)
			engineContractHTTP(t, m, b.Bytes(), "")
			_, err := m.InstallComponent(context.Background(), ComponentMKXPZ, nil)
			if err == nil {
				t.Fatal("bad metadata accepted")
			}
			want := ""
			switch kind {
			case "size", "combined-size":
				want = "extracted size limit"
			case "count":
				want = "archive entry limit"
			case "metadata-cap":
				want = "size limit exceeded"
			case "crc":
				if !errors.Is(err, zip.ErrChecksum) {
					t.Fatalf("wanted ZIP CRC failure, got %v", err)
				}
			case "understated-size":
				if !errors.Is(err, zip.ErrFormat) {
					t.Fatalf("wanted ZIP size mismatch, got %v", err)
				}
			}
			if want != "" && !strings.Contains(err.Error(), want) {
				t.Fatalf("wanted %s, got %v", want, err)
			}
			noStaging(t, m.Root)
		})
	}
}

func TestComponentZIPMetadataNotWritten(t *testing.T) {
	root := t.TempDir()
	archive, dest := filepath.Join(root, "input.zip"), filepath.Join(root, "unpacked")
	ad := componentAppleDouble(t)
	entries := []zipEntry{
		{"Z-universal.app/", os.ModeDir | 0755, nil},
		{"Z-universal.app/data", 0644, []byte("unchanged")},
		{"__MACOSX/._Z-universal.app", 0644, ad},
	}
	engineContractWrite(t, archive, makeZIP(t, entries), 0600)
	var last Progress
	err := extractZIP(context.Background(), archive, dest, "Z-universal.app", func(p Progress) error { last = p; return nil }, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "__MACOSX")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("metadata written in staging", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "Z-universal.app/data"))
	if err != nil || string(b) != "unchanged" {
		t.Fatal("data fork changed", err)
	}
	if last.Completed != int64(len(ad)+len("unchanged")) || last.Total != last.Completed {
		t.Fatal("metadata not included in progress accounting", last)
	}
}

func TestNWRejectsAppleDouble(t *testing.T) {
	es := append(goodEntries(), zipEntry{"__MACOSX/._" + testPrefix, 0644, componentAppleDouble(t)})
	failedInstall(t, fixture(t, makeZIP(t, es), nil))
}

// Keep the fixture's resource fork descriptor explicit as a guard against
// accidentally treating nonempty forks as disposable packaging metadata.
func TestComponentAppleDoubleFixture(t *testing.T) {
	b := componentAppleDouble(t)
	if binary.BigEndian.Uint32(b[38:42]) != 2 || binary.BigEndian.Uint32(b[46:50]) != 0 {
		t.Fatal("fixture resource fork is not empty")
	}
}

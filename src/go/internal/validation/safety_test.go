package validation

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func canonicalTemp(t *testing.T) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFreshRoot(t *testing.T) {
	parent := canonicalTemp(t)
	root := filepath.Join(parent, "run")
	if err := createRoot(root); err != nil {
		t.Fatal(err)
	}
	if err := createRoot(root); err == nil {
		t.Fatal("existing root accepted")
	}
	if err := os.Symlink(root, filepath.Join(parent, "link")); err != nil {
		t.Fatal(err)
	}
	if err := createRoot(filepath.Join(parent, "link", "nested")); err == nil {
		t.Fatal("symlink parent accepted")
	}
}

func TestExtractRejectsPathsSymlinksAndSaves(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "fixture/../../escape", "fixture/save/file1.rmmzsave", "fixture/save", "fixture/profile/data", "fixture/a\\b", "fixture/link"} {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			h := &zip.FileHeader{Name: name, Method: zip.Store}
			h.SetMode(0600)
			if name == "fixture/link" {
				h.SetMode(os.ModeSymlink | 0777)
			}
			w, err := z.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, "outside")
			if err = z.Close(); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(t.TempDir(), "test.zip")
			if err = os.WriteFile(archive, b.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if err = extractFixture(context.Background(), archive, t.TempDir(), "fixture"); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}

func TestFixtureCopyPreservesSource(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("fixture/index.html")
	_, _ = io.WriteString(w, "original")
	_ = z.Close()
	archive := filepath.Join(t.TempDir(), "fixture.zip")
	if err := os.WriteFile(archive, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := hashFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err = extractFixture(context.Background(), archive, dest, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dest, "fixture", "index.html"), []byte("changed copy"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := hashFile(archive)
	if err != nil || before != after {
		t.Fatalf("source changed: %v", err)
	}
}

func TestPinnedArchiveUsesOwnedBytes(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.zip")
	if err := os.WriteFile(source, []byte("reviewed"), 0600); err != nil {
		t.Fatal(err)
	}
	pin, err := hashFile(source)
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(t.TempDir(), "fixture.zip")
	if err = copyPinnedArchive(source, owned, pin); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, []byte("replaced after review"), 0600); err != nil {
		t.Fatal(err)
	}
	copied, err := hashFile(owned)
	if err != nil || copied != pin {
		t.Fatal("owned archive followed source mutation")
	}
	if err = copyPinnedArchive(source, filepath.Join(t.TempDir(), "rejected.zip"), pin); err == nil {
		t.Fatal("unreviewed bytes accepted")
	}
	if err = copyPinnedArchive(source, owned, pin); err == nil {
		t.Fatal("existing archive overwritten")
	}
}

func TestRuntimeCopyAndSymlinkBoundary(t *testing.T) {
	source := canonicalTemp(t)
	if err := os.WriteFile(filepath.Join(source, "binary"), []byte("runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("binary", filepath.Join(source, "link")); err != nil {
		t.Fatal(err)
	}
	before, err := treeHash(source)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(canonicalTemp(t), "copy")
	if err = copyRuntime(context.Background(), source, dest); err != nil {
		t.Fatal(err)
	}
	copied, err := treeHash(dest)
	if err != nil || copied != before {
		t.Fatalf("copy mismatch: %v", err)
	}
	if err = os.WriteFile(filepath.Join(dest, "binary"), []byte("changed"), 0700); err != nil {
		t.Fatal(err)
	}
	after, err := treeHash(source)
	if err != nil || after != before {
		t.Fatal("source changed")
	}
	if err = os.Symlink("../outside", filepath.Join(source, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err = treeHash(source); err == nil {
		t.Fatal("escaping symlink accepted")
	}
}

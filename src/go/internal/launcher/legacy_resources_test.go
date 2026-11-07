package launcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyResourceDigest(t *testing.T) {
	p := t.TempDir()
	engineContractWrite(t, filepath.Join(p, "a", "file"), []byte("hello"), 0600)
	engineContractWrite(t, filepath.Join(p, "a.rb"), []byte("world"), 0600)
	digest, err := legacyResourceDigest(p)
	if err != nil {
		t.Fatal(err)
	}
	if digest != "7694be8678efff70b9732d53ed5be525d0e02cff4246ac51067ff00df3ec6d50" {
		t.Fatal(digest)
	}
	engineContractWrite(t, filepath.Join(p, "a", "file"), []byte("broken"), 0600)
	changed, err := legacyResourceDigest(p)
	if err != nil || changed == digest {
		t.Fatal(changed, err)
	}
	if err := os.Remove(filepath.Join(p, "a.rb")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(p, "a", "file"), filepath.Join(p, "a.rb")); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyResourceDigest(p); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestLegacyPartialResourcesRejected(t *testing.T) {
	for _, c := range []Component{ComponentRTP, ComponentKawariki} {
		t.Run(string(c), func(t *testing.T) {
			p := filepath.Join(t.TempDir(), componentName(c))
			if c == ComponentRTP {
				for _, name := range []string{"Standard", "RPGVX", "RPGVXace"} {
					engineContractWrite(t, filepath.Join(p, name, "partial"), []byte("partial"), 0600)
				}
			} else {
				engineContractWrite(t, filepath.Join(p, "preload.rb"), []byte("partial"), 0600)
			}
			if err := checkComponent(p, c); err == nil {
				t.Fatal("partial resource accepted")
			}
			if _, err := os.Lstat(filepath.Join(p, ".rpgm-complete.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("partial resource was modified", err)
			}
		})
	}
}

func TestInstallPreservesMarkerlessResources(t *testing.T) {
	for _, c := range []Component{ComponentRTP, ComponentKawariki} {
		t.Run(string(c), func(t *testing.T) {
			m := engineContractManager(t)
			extra := filepath.Join(m.Root, componentName(c), "extra", "user-data")
			engineContractWrite(t, extra, []byte("keep"), 0600)
			engineContractHTTP(t, m, engineContractArchive(t, c, false), "")
			if _, err := m.InstallComponent(context.Background(), c, nil); err == nil || !strings.Contains(err.Error(), "markerless") {
				t.Fatalf("expected markerless resource refusal: %v", err)
			}
			if b, err := os.ReadFile(extra); err != nil || string(b) != "keep" {
				t.Fatalf("user data changed: %q %v", b, err)
			}
			noStaging(t, m.Root)
		})
	}
}

// Set RPGM_LEGACY_FIXTURES to a directory containing the original extracted ZIPs.
func TestLegacyOfficialResources(t *testing.T) {
	root := os.Getenv("RPGM_LEGACY_FIXTURES")
	if root == "" {
		t.Skip("original release ZIP fixtures not supplied")
	}
	for _, c := range []Component{ComponentRTP, ComponentKawariki} {
		t.Run(string(c), func(t *testing.T) {
			p := filepath.Join(root, componentName(c))
			if err := checkComponent(p, c); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(filepath.Join(p, ".rpgm-complete.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("legacy check wrote a marker", err)
			}
		})
	}
	for _, change := range []string{"missing", "corrupt", "extra", "invalid-marker", "symlink"} {
		t.Run(change, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "kawariki")
			if err := os.CopyFS(p, os.DirFS(filepath.Join(root, "kawariki"))); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(p, "patches.rb")
			switch change {
			case "missing":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(file, []byte("broken"), 0600); err != nil {
					t.Fatal(err)
				}
			case "extra":
				if err := os.WriteFile(filepath.Join(p, "extra.rb"), []byte("extra"), 0600); err != nil {
					t.Fatal(err)
				}
			case "invalid-marker":
				if err := os.WriteFile(filepath.Join(p, ".rpgm-complete.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "kawariki", "patches.rb"), file); err != nil {
					t.Fatal(err)
				}
			}
			if err := checkComponent(p, ComponentKawariki); err == nil {
				t.Fatal("damaged legacy installation accepted")
			}
		})
	}
}

package validation

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

const fixtureCommit = "81abd876e3c339fbd7d74323ad8817ad7011cade"
const fixtureSHA256 = "ed4ee4c57d1c7b4ababf8900c8e4eb2fc4df1a3094911a6cefa66209810d2b7c"
const fixtureDirectory = "rpgmaker-mz-demo-" + fixtureCommit

func canonicalInput(p string) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return errors.New("input must be a clean absolute path")
	}
	actual, err := filepath.EvalSymlinks(p)
	if err != nil {
		return err
	}
	if actual != p {
		return fmt.Errorf("symlink input path refused: %s", p)
	}
	return nil
}

// Refuse reuse, even an empty root: profiles/saves can never be overwritten.
func createRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return errors.New("root must be a clean absolute path")
	}
	parent := filepath.Dir(root)
	if err := canonicalInput(parent); err != nil {
		return err
	}
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (stat.Uid != uint32(os.Getuid()) && info.Mode()&os.ModeSticky == 0) || (info.Mode().Perm()&0022 != 0 && info.Mode()&os.ModeSticky == 0) {
		return errors.New("root parent must be owned and not writable by others (or sticky temporary directory)")
	}
	return os.Mkdir(root, 0700)
}

func openRegular(p string) (*os.File, error) {
	fd, err := syscall.Open(p, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), p)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("not a regular file: %s", p)
	}
	return f, nil
}

func hashFile(p string) (string, error) {
	f, err := openRegular(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return hashReader(f)
}

// Extract only private, pinned bytes, never a source archive that can change after review.
func copyPinnedArchive(source, dest, pin string) error {
	in, err := openRegular(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(in, (100<<20)+1))
	if err = errors.Join(copyErr, out.Close()); err != nil {
		return err
	}
	copied, err := hashFile(dest)
	if err != nil {
		return err
	}
	if copied != pin {
		return errors.New("owned fixture archive does not match reviewed SHA-256")
	}
	return nil
}

func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func forbiddenData(name string) bool {
	for _, part := range strings.Split(strings.ToLower(name), "/") {
		if part == "save" || part == "saves" || part == "profile" || part == "user-data-dir" || strings.HasSuffix(part, ".rmmzsave") {
			return true
		}
	}
	return false
}

func extractFixture(ctx context.Context, archive, dest, top string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	seen := map[string]bool{}
	var size uint64
	// Validate every member before creating any payload, including unused entries.
	for _, f := range z.File {
		name := strings.TrimSuffix(f.Name, "/")
		if name == "" || strings.ContainsAny(name, "\\:\x00") || path.Clean(name) != name || (!strings.HasPrefix(name, top+"/") && name != top) || forbiddenData(name) || (!f.Mode().IsRegular() && !f.Mode().IsDir()) || seen[strings.ToLower(name)] {
			return fmt.Errorf("unsafe archive member: %s", f.Name)
		}
		seen[strings.ToLower(name)] = true
		if f.UncompressedSize64 > 200<<20 || size > 200<<20-f.UncompressedSize64 {
			return errors.New("fixture exceeds 200 MiB")
		}
		size += f.UncompressedSize64
	}
	for _, f := range z.File {
		if err = ctx.Err(); err != nil {
			return err
		}
		p := filepath.Join(dest, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(p, 0700); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return err
		}
		in, e := f.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			in.Close()
			return e
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, int64(f.UncompressedSize64)+1))
		closeErr := errors.Join(in.Close(), out.Close())
		if err = errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		if uint64(n) != f.UncompressedSize64 {
			return errors.New("archive size mismatch")
		}
	}
	return nil
}

// Fingerprint all file bytes, modes and link targets; never follow a link outside.
func treeHash(root string) (string, error) {
	if err := canonicalInput(root); err != nil {
		return "", err
	}
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		value := ""
		switch {
		case info.Mode().IsRegular():
			value, err = hashFile(p)
		case info.IsDir():
		case info.Mode()&os.ModeSymlink != 0:
			value, err = os.Readlink(p)
			if err != nil {
				return err
			}
			if filepath.IsAbs(value) {
				return fmt.Errorf("absolute runtime link: %s", p)
			}
			resolved, e := filepath.EvalSymlinks(p)
			if e != nil {
				return e
			}
			if !within(root, resolved) {
				return fmt.Errorf("runtime link escapes copy: %s", p)
			}
		default:
			return fmt.Errorf("special runtime file: %s", p)
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(h, "%s\x00%d\x00%s\n", rel, info.Mode(), value)
		return err
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

func copyRuntime(ctx context.Context, source, dest string) error {
	before, err := treeHash(source)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		return errors.New("runtime destination already exists or cannot be inspected")
	}
	// ditto preserves macOS resource forks, signatures and quarantine xattrs; do not strip them.
	command := exec.CommandContext(ctx, "/usr/bin/ditto", "--rsrc", "--extattr", source, dest)
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	if output, e := command.CombinedOutput(); e != nil {
		return fmt.Errorf("runtime copy: %w: %s", e, output)
	}
	copied, err := treeHash(dest)
	if err != nil {
		return err
	}
	after, err := treeHash(source)
	if err != nil {
		return err
	}
	if before != copied || before != after {
		return errors.New("runtime bytes/modes/links changed during copy")
	}
	return nil
}

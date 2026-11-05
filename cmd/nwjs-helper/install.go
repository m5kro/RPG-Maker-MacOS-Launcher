package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/macho"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type installOptions struct {
	root, version string
}

type receipt struct {
	Version string `json:"version"`
	Arch    string `json:"arch"`
	Rosetta bool   `json:"rosetta"`
}

func install(ctx context.Context, c config, e *emitter, o installOptions) (app string, err error) {
	if !versionPattern.MatchString(o.version) {
		return "", errors.New("version must be vX.Y.Z (stable numeric release)")
	}
	if !filepath.IsAbs(o.root) || filepath.Clean(o.root) == string(os.PathSeparator) || strings.ContainsRune(o.root, 0) {
		return "", errors.New("root must be an absolute managed directory, not filesystem root")
	}
	root, err := managedRoot(o.root)
	if err != nil {
		return "", err
	}
	lock, err := acquireLock(root)
	if err != nil {
		return "", err
	}
	defer lock.Close() // Closing releases flock even when a previous process crashed.
	target := filepath.Join(root, o.version)
	app = filepath.Join(target, "nwjs.app")
	if _, err := os.Lstat(target); err == nil {
		if err := validateInstalled(ctx, target, o); err != nil {
			return "", fmt.Errorf("existing target preserved: %w", err)
		}
		return app, ctx.Err()
	} else if !os.IsNotExist(err) {
		return "", err
	}
	stage, err := os.MkdirTemp(root, ".nwjs-stage-")
	if err != nil {
		return "", err
	}
	defer func() {
		if cleanup := os.RemoveAll(stage); cleanup != nil {
			err = errors.Join(err, fmt.Errorf("cleaning owned staging %s: %w", stage, cleanup))
		}
	}()
	prefix := "nwjs-sdk-" + o.version + "-osx-arm64"
	archive := prefix + ".zip"
	manifestURL := c.downloadBase + "/" + o.version + "/SHASUMS256.txt"
	manifest, err := c.metadata(ctx, manifestURL, maxManifest)
	if err != nil {
		return "", fmt.Errorf("official checksum manifest unavailable; refusing unverified install: %w", err)
	}
	checksum, err := archiveChecksum(manifest, archive)
	if err != nil {
		return "", err
	}
	zipPath := filepath.Join(stage, "download.zip")
	if err := download(ctx, c, e, c.downloadBase+"/"+o.version+"/"+archive, zipPath, checksum); err != nil {
		return "", err
	}
	unpacked := filepath.Join(stage, "unpacked")
	if err := os.Mkdir(unpacked, 0700); err != nil {
		return "", err
	}
	if err := extract(ctx, zipPath, unpacked, prefix, e); err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	payload := filepath.Join(stage, "payload")
	if err := os.Mkdir(payload, 0700); err != nil {
		return "", err
	}
	stagedApp := filepath.Join(payload, "nwjs.app")
	if err := os.Rename(filepath.Join(unpacked, prefix, "nwjs.app"), stagedApp); err != nil {
		return "", err
	}
	if err := validateBundle(ctx, stagedApp); err != nil {
		return "", err
	}
	b, err := json.Marshal(receipt{o.version, "arm64", false})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(payload, ".nwjs-install.json"), b, 0600); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Reserve exclusively, then atomically replace only our own empty directory under flock.
	// shortcut: a crash between mkdir and rename leaves an empty target; manual inspection is required.
	if err := os.Mkdir(target, 0700); err != nil {
		return "", fmt.Errorf("target appeared; preserving it: %w", err)
	}
	if err := syscall.Rename(payload, target); err != nil {
		return "", errors.Join(err, os.Remove(target)) // Remove only the empty reservation, never a version tree.
	}
	return app, nil
}

func archiveChecksum(manifest []byte, name string) ([]byte, error) {
	var found []byte
	for _, line := range strings.Split(string(manifest), "\n") {
		// Filenames elsewhere in official manifests contain spaces; only match the exact archive basename.
		if len(line) < 66 {
			continue
		}
		filename := strings.TrimSuffix(line[66:], "\r")
		if filename != name {
			continue
		}
		if line[64] != ' ' || (line[65] != ' ' && line[65] != '*') {
			return nil, errors.New("malformed official checksum entry")
		}
		sum, err := hex.DecodeString(line[:64])
		if err != nil || len(sum) != sha256.Size {
			return nil, errors.New("malformed official SHA-256")
		}
		if found != nil {
			return nil, errors.New("duplicate official archive checksum")
		}
		found = sum
	}
	if found == nil {
		return nil, fmt.Errorf("official manifest has no SHA-256 for %s; refusing unverified install", name)
	}
	return found, nil
}

func download(ctx context.Context, c config, e *emitter, url, path string, want []byte) error {
	resp, err := c.get(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.ContentLength > maxArchive {
		return errors.New("archive size limit exceeded")
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	if err := e.progress("download", 0, total); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := copyProgress(ctx, f, resp.Body, maxArchive, e, "download", 0, total)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return io.ErrUnexpectedEOF
	}
	if err := e.progress("download", n, n); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := e.progress("verify", 0, n); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := copyProgress(ctx, h, f, maxArchive, e, "verify", 0, n); err != nil {
		return err
	}
	if !bytes.Equal(h.Sum(nil), want) {
		return errors.New("archive SHA-256 checksum mismatch")
	}
	return e.progress("verify", n, n)
}

func regularFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	return info, nil
}

func realDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode().Type() != os.ModeDir {
		return fmt.Errorf("not a real directory: %s", path)
	}
	return nil
}

func validateInstalled(ctx context.Context, target string, o installOptions) error {
	if err := realDirectory(target); err != nil {
		return err
	}
	p := filepath.Join(target, ".nwjs-install.json")
	info, err := regularFile(p)
	if err != nil {
		return fmt.Errorf("cannot prove prior helper install: %w", err)
	}
	if info.Size() > 4096 {
		return errors.New("invalid install receipt")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var got receipt
	if err := json.Unmarshal(b, &got); err != nil {
		return err
	}
	if got != (receipt{o.version, "arm64", false}) {
		return errors.New("install receipt must match this version and arm64 architecture with no Rosetta")
	}
	if _, err := os.Lstat(filepath.Join(target, "run-with-rosetta")); !os.IsNotExist(err) {
		return errors.New("Rosetta marker present or unreadable; this helper cannot reuse it")
	}
	return validateBundle(ctx, filepath.Join(target, "nwjs.app"))
}

func validateMachO(exe string) error {
	var arm64 bool
	fat, err := macho.OpenFat(exe)
	switch {
	case err == nil:
		defer fat.Close()
		for _, slice := range fat.Arches {
			if slice.Cpu == macho.CpuArm64 && slice.Type == macho.TypeExec {
				arm64 = true
				break
			}
		}
	case errors.Is(err, macho.ErrNotFat):
		f, err := macho.Open(exe)
		if err != nil {
			return fmt.Errorf("NW.js executable is not a supported Mach-O: %w", err)
		}
		defer f.Close()
		arm64 = f.Cpu == macho.CpuArm64 && f.Type == macho.TypeExec
	default:
		return fmt.Errorf("NW.js executable is not a supported Mach-O: %w", err)
	}
	if !arm64 {
		return errors.New("NW.js Mach-O must contain an arm64 executable")
	}
	return nil
}

func validateBundle(ctx context.Context, app string) error {
	for _, dir := range []string{app, filepath.Join(app, "Contents"), filepath.Join(app, "Contents/MacOS")} {
		if err := realDirectory(dir); err != nil {
			return err
		}
	}
	exe := filepath.Join(app, "Contents/MacOS/nwjs")
	info, err := regularFile(exe)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0100 == 0 {
		return errors.New("Contents/MacOS/nwjs is not executable")
	}
	if err := validateMachO(exe); err != nil {
		return err
	}
	var count int
	var size int64
	return filepath.WalkDir(app, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		count++
		if count > maxEntries {
			return errors.New("bundle entry limit exceeded")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return validateLink(app, path)
		case info.Mode().Type() == os.ModeDir:
			return nil
		case info.Mode().IsRegular():
			size += info.Size()
			if size > maxExtracted {
				return errors.New("bundle size limit exceeded")
			}
			return nil
		default:
			return fmt.Errorf("unsafe bundle file: %s", path)
		}
	})
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func validateLink(root, path string) error {
	link, err := os.Readlink(path)
	if err != nil {
		return err
	}
	if filepath.IsAbs(link) || strings.ContainsAny(link, "\\\x00:") || !within(root, filepath.Join(filepath.Dir(path), link)) {
		return fmt.Errorf("unsafe symlink: %s", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("broken or cyclic symlink %s: %w", path, err)
	}
	if !within(root, resolved) {
		return fmt.Errorf("symlink escapes bundle: %s", path)
	}
	return nil
}

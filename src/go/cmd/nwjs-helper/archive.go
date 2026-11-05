package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func archiveName(name, prefix string) (string, error) {
	clean := strings.TrimSuffix(name, "/")
	if len(name) > maxPath || strings.ContainsAny(name, "\\\x00:") || clean == "" || path.IsAbs(clean) || path.Clean(clean) != clean {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	if clean != prefix && !strings.HasPrefix(clean, prefix+"/") {
		return "", fmt.Errorf("unexpected archive root %q", name)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "." || part == ".." || part == "" {
			return "", fmt.Errorf("unsafe archive path %q", name)
		}
	}
	return clean, nil
}

func extract(ctx context.Context, archive, dest, prefix string, e *emitter) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) > maxEntries {
		return errors.New("archive entry limit exceeded")
	}
	names := make(map[string]os.FileMode, len(z.File))
	var total int64
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, err := archiveName(f.Name, prefix)
		if err != nil {
			return err
		}
		// macOS commonly uses case-insensitive volumes. Reject ambiguous aliases on every volume.
		key := strings.ToLower(name)
		if _, exists := names[key]; exists {
			return fmt.Errorf("duplicate archive path %q", name)
		}
		mode := f.Mode()
		if !mode.IsRegular() && !mode.IsDir() && mode.Type() != os.ModeSymlink {
			return fmt.Errorf("unsupported archive file %q", name)
		}
		if f.UncompressedSize64 > uint64(maxExtracted-total) {
			return errors.New("extracted size limit exceeded")
		}
		if mode.Type() == os.ModeSymlink && f.UncompressedSize64 > maxPath {
			return errors.New("symlink size limit exceeded")
		}
		if mode.IsDir() && f.UncompressedSize64 != 0 {
			return errors.New("directory contains data")
		}
		total += int64(f.UncompressedSize64)
		names[key] = mode
	}
	for name := range names {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if mode, exists := names[parent]; exists && !mode.IsDir() {
				return fmt.Errorf("archive writes through non-directory ancestor %q", parent)
			}
		}
	}
	if err := e.progress("extract", 0, total); err != nil {
		return err
	}
	type link struct{ path, target string }
	var links []link
	var completed int64
	last := time.Now()
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, _ := archiveName(f.Name, prefix)
		target := filepath.Join(dest, filepath.FromSlash(name))
		if f.Mode().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		var n int64
		if f.Mode()&os.ModeSymlink != 0 {
			b, readErr := readBounded(r, maxPath)
			closeErr := r.Close()
			if err := errors.Join(readErr, closeErr); err != nil {
				return err
			}
			value := string(b)
			if value == "" || filepath.IsAbs(value) || strings.ContainsAny(value, "\\\x00:") || !within(dest, filepath.Join(filepath.Dir(target), value)) {
				return fmt.Errorf("unsafe symlink %q", name)
			}
			links = append(links, link{target, value})
			n = int64(len(b))
		} else {
			w, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.Mode().Perm())
			if err != nil {
				r.Close()
				return err
			}
			n, err = copyProgress(ctx, w, r, int64(f.UncompressedSize64), e, "extract", completed, total)
			closeErr := errors.Join(w.Close(), r.Close())
			if err := errors.Join(err, closeErr); err != nil {
				return err
			}
			// Chmod restores archive executable permissions independently of the caller's umask.
			if err := os.Chmod(target, f.Mode().Perm()); err != nil {
				return err
			}
		}
		if n != int64(f.UncompressedSize64) {
			return io.ErrUnexpectedEOF
		}
		completed += n
		if time.Since(last) >= 100*time.Millisecond {
			if err := e.progress("extract", completed, total); err != nil {
				return err
			}
			last = time.Now()
		}
	}
	// Links are created only after all data writes, so no archive link can redirect a write.
	for _, l := range links {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Symlink(l.target, l.path); err != nil {
			return err
		}
	}
	for _, l := range links {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateLink(dest, l.path); err != nil {
			return err
		}
	}
	return e.progress("extract", completed, total)
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// The macOS backend uses flock without cgo; host selection permits only darwin/arm64.
func managedRoot(name string) (string, error) {
	name = filepath.Clean(name)
	if err := os.MkdirAll(name, 0700); err != nil {
		return "", err
	}
	if err := realDirectory(name); err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(name) // macOS /var and /tmp are system symlinks.
	if err != nil {
		return "", err
	}
	if root == "/" {
		return "", errors.New("filesystem root is not a managed root")
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 {
		return "", errors.New("managed root must be owned by this user and not group/world writable")
	}
	return root, nil
}

func acquireLock(root string) (*os.File, error) {
	name := filepath.Join(root, ".nwjs-install.lock")
	fd, err := syscall.Open(name, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, fmt.Errorf("install lock: %w", err)
	}
	f := os.NewFile(uintptr(fd), name)
	fail := func(err error) (*os.File, error) { f.Close(); return nil, fmt.Errorf("install lock: %w", err) }
	info, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0077 != 0 {
		return fail(errors.New("unsafe lock file"))
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fail(fmt.Errorf("another helper may be installing: %w", err))
	}
	current, err := os.Lstat(name)
	if err != nil {
		return fail(err)
	}
	if !os.SameFile(info, current) {
		return fail(errors.New("lock file changed"))
	}
	// Never unlink: waiting/opening processes must all lock the same persistent inode.
	return f, nil
}

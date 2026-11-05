package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// AcquireManagedLock obtains the same nonblocking, exclusive lock used by InstallNW
// and the helper CLI. Callers modifying external engines under root must hold it
// throughout their mutation, but must not call InstallNW while holding it.
// It requires macOS arm64 and an absolute, user-owned managed directory that is
// not group/world writable. A missing root is created with mode 0700; a final
// symlink is rejected (system ancestor symlinks such as /var remain allowed).
// The returned release is safe to call more than once. The lock file is never
// removed: every cooperating caller must keep using the same persistent inode.
func AcquireManagedLock(root string) (release func(), err error) {
	if err := checkHost(runtime.GOOS, runtime.GOARCH); err != nil {
		return nil, err
	}
	root, err = managedRoot(root)
	if err != nil {
		return nil, err
	}
	lock, err := acquireLock(root)
	if err != nil {
		return nil, err
	}
	return func() { _ = lock.Close() }, nil
}

// The macOS backend uses flock without cgo; host selection permits only darwin/arm64.
func managedRoot(name string) (string, error) {
	if !filepath.IsAbs(name) || filepath.Clean(name) == string(os.PathSeparator) || strings.ContainsRune(name, 0) {
		return "", errors.New("root must be an absolute managed directory, not filesystem root")
	}
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

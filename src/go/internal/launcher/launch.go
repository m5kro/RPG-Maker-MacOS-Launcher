package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Process owns one started game. Wait cancellation only stops waiting, never the
// game. Signal is explicit. The game is detached from the launch context/session.
type Process struct {
	PID      int
	Done     <-chan struct{}
	LogPath  string
	Warnings []string
	cmd      *exec.Cmd
	exitCode int
	err      error
}

func (p *Process) Wait(ctx context.Context) (int, error) {
	select {
	case <-p.Done:
		return p.exitCode, p.err
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}
func (p *Process) Signal(signal os.Signal) error { return p.cmd.Process.Signal(signal) }
func engineGame(game GameInfo) (GameInfo, error) {
	if game.Path == "" {
		return game, errors.New("select a game folder")
	}
	p, err := filepath.Abs(game.Path)
	if err != nil {
		return game, err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return game, err
	}
	detected, err := DetectGame(p)
	if err != nil {
		return game, err
	}
	if detected.Engine != game.Engine {
		return game, errors.New("selected game engine changed")
	}
	return detected, nil
}
func engineStringList(v any) ([]string, error) {
	switch a := v.(type) {
	case []string:
		return append([]string(nil), a...), nil
	case []any:
		r := make([]string, len(a))
		for i, v := range a {
			s, ok := v.(string)
			if !ok {
				return nil, errors.New("path list must contain strings")
			}
			r[i] = s
		}
		return r, nil
	default:
		return nil, fmt.Errorf("expected path list, got %T", v)
	}
}
func engineResourcePath(base, p string) (string, error) {
	if p == "" || strings.ContainsRune(p, 0) {
		return "", errors.New("empty or NUL resource path")
	}
	if strings.HasPrefix(p, "~") {
		return "", errors.New("use an absolute resource path, not ~")
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p), nil
}
func validateMKXPPaths(game GameInfo, c map[string]any) error {
	for _, key := range []string{"gameFolder", "midiSoundFont", "iconPath", "customScript", "RTP", "preloadScript", "postloadScript", "patches", "rubyLoadpath"} {
		value, ok := c[key]
		if !ok {
			continue
		}
		list := []string{}
		switch key {
		case "RTP", "preloadScript", "postloadScript", "patches", "rubyLoadpath":
			var err error
			list, err = engineStringList(value)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		default:
			s, ok := value.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", key)
			}
			if s != "" {
				list = []string{s}
			}
		}
		for _, p := range list {
			p, err := engineResourcePath(game.Path, p)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			info, err := os.Stat(p)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			directory := key == "gameFolder" || key == "RTP" || key == "rubyLoadpath"
			if directory && !info.IsDir() || !directory && !info.Mode().IsRegular() {
				return fmt.Errorf("%s has wrong file type: %s", key, p)
			}
			if key == "midiSoundFont" {
				if err := checkSoundFont(p); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Launch validates the runtime before repairing a game. Advanced settings are
// read from Root. It never downloads or opens dialogs. The exclusive managed lock
// is held until exit, also inherited by the child for launcher shutdown safety.
func (m *Manager) Launch(ctx context.Context, game GameInfo, s Settings) (*Process, error) {
	if err := engineContext(ctx); err != nil {
		return nil, err
	}
	root, err := managedRoot(m.Root)
	if err != nil {
		return nil, err
	}
	lock, err := acquireLock(root)
	if err != nil {
		return nil, err
	}
	owned := true
	defer func() {
		if owned {
			lock.Close()
		}
	}()
	executable, err := m.CheckRuntime(game, s)
	if err != nil {
		return nil, err
	}
	game, err = engineGame(game)
	if err != nil {
		return nil, err
	}
	a, err := LoadAdvancedSettings(m.Root)
	if err != nil {
		return nil, err
	}
	args := []string{game.Path}
	program := executable
	var warnings []string
	switch game.Engine {
	case NWJS:
		// Assets may legitimately be linked elsewhere; only repair targets must stay inside the game.
		for _, dir := range []string{"js", "www/js"} {
			path := filepath.Join(game.Path, dir)
			info, e := os.Lstat(path)
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e != nil {
				return nil, e
			}
			if info.IsDir() {
				resolved, e := filepath.EvalSymlinks(path)
				if e != nil {
					return nil, e
				}
				if !within(game.Path, resolved) {
					return nil, errors.New("refusing to repair JS outside the selected game")
				}
			}
		}
		warnings, err = m.prepareNW(game.Path, s)
		if err != nil {
			return nil, err
		}
	case MKXPZ:
		if err = engineSafeTree(ctx, filepath.Join(m.Root, "Z-universal.app")); err != nil {
			return nil, err
		}
		missing, err := m.MissingComponents(game, s)
		if err != nil {
			return nil, err
		}
		if len(missing) != 0 {
			return nil, fmt.Errorf("install or repair required components first: %v", missing)
		}
		c, err := m.mkxpConfig(game, a)
		if err != nil {
			return nil, err
		}
		if err = validateMKXPPaths(game, c); err != nil {
			return nil, err
		}
		if err = engineWriteJSON(filepath.Join(m.Root, "Z-universal.app", "Contents", "Game", "mkxp.json"), c); err != nil {
			return nil, err
		}
		program = "/usr/bin/arch"
		args = []string{"-arm64", executable, game.Path}
	case EasyRPG:
		extra, err := easyArguments(a)
		if err != nil {
			return nil, err
		}
		program = "/usr/bin/arch"
		args = append([]string{"-arm64", executable, "--window", "--project-path", game.Path}, extra...)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	logPath := filepath.Join(m.Root, "game.log")
	fd, err := syscall.Open(logPath, syscall.O_WRONLY|syscall.O_APPEND|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	log := os.NewFile(uintptr(fd), logPath)
	defer log.Close()
	info, err := log.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("game log is not a privately owned regular file")
	}
	if err = m.logf("Launching %s from %s; game log: %s\n", game.Engine, game.Path, logPath); err != nil {
		return nil, err
	}
	nwVersion := ""
	if game.Engine == NWJS {
		nwVersion = s.LastSelectedVersion
	}
	if _, err = fmt.Fprintf(log, "\n--- launch time=%s engine=%q game=%q nw_version=%q ---\n", time.Now().UTC().Format(time.RFC3339Nano), game.Engine, game.Path, nwVersion); err != nil {
		return nil, err
	}
	cmd := m.command(program, args...)
	cmd.Dir = game.Path
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.ExtraFiles = []*os.File{lock}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	p := &Process{PID: cmd.Process.Pid, Done: done, LogPath: logPath, Warnings: warnings, cmd: cmd, exitCode: -1}
	owned = false
	go func() {
		p.err = cmd.Wait()
		if cmd.ProcessState != nil {
			p.exitCode = cmd.ProcessState.ExitCode()
		}
		lock.Close()
		close(done)
	}()
	return p, nil
}

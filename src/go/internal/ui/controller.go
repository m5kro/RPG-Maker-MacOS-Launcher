// Package ui binds the native GUI to launcher; it owns jobs, not game lifetimes.
package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"rpgm-launcher/nwjs-helper/internal/launcher"
)

var ErrBusy = errors.New("処理中です。完了またはキャンセルの後始末をお待ちください")
var ErrClosing = errors.New("終了処理中です")
var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

const maxLogBytes = 128 * 1024

type Preferences struct {
	Version           string
	Folder            string
	DisableProtection bool
	PatchGetPixel     bool
}
type State struct {
	Busy, Cancelling, Closing bool
	Operation                 string
}
type Snapshot struct {
	Preferences   Preferences
	Installed     []string
	Game          *launcher.GameInfo
	SettingsError string
	State         State
}
type Request struct {
	Action      string
	Component   string
	Destination string
	Name        string
	Approved    bool
}
type Result struct {
	Versions []launcher.NWVersion
	Missing  []launcher.Component
	Update   *launcher.UpdateInfo
	Path     string
	PID      int
}

type UI struct {
	installComponent    func(context.Context, launcher.Component, launcher.ProgressFunc) (launcher.InstallResult, error)
	installedNWVersions func() ([]string, error)
	native              *nativeUI
	manager             *launcher.Manager
	logs                *boundedLog
	mu                  sync.Mutex
	state               State
	cancel              context.CancelFunc
	done                chan struct{}
	emit                func(string, any)
}

func New(root, resources string) (*UI, error) {
	if !filepath.IsAbs(root) || !filepath.IsAbs(resources) {
		return nil, errors.New("root/resources must be absolute")
	}
	u := &UI{logs: &boundedLog{}}
	m, err := launcher.NewManager(root, resources, u.logs)
	if err != nil {
		return nil, err
	}
	u.manager = m
	u.logs.path = filepath.Join(m.Root, "launcher.log")
	u.installComponent = m.InstallComponent
	u.installedNWVersions = m.InstalledNWVersions
	return u, nil
}
func (u *UI) State() State { u.mu.Lock(); defer u.mu.Unlock(); return u.state }
func (u *UI) event(name string, value any) {
	if u.emit != nil {
		u.emit(name, value)
	}
}
func (u *UI) operation(name string, work func(context.Context) error) error {
	u.mu.Lock()
	if u.state.Closing {
		u.mu.Unlock()
		return ErrClosing
	}
	if u.state.Busy {
		u.mu.Unlock()
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel
	u.done = make(chan struct{})
	u.state.Busy = true
	u.state.Operation = name
	u.event("state", u.state)
	u.mu.Unlock()
	err := work(ctx)
	cancel()
	if err != nil {
		u.log("%s: %v", name, err)
	}
	u.mu.Lock()
	u.state.Busy = false
	u.state.Cancelling = false
	u.state.Operation = ""
	u.cancel = nil
	close(u.done)
	u.event("state", u.state)
	u.mu.Unlock()
	return err
}
func (u *UI) CancelOperation() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.cancel != nil {
		u.state.Cancelling = true
		u.cancel()
		u.event("state", u.state)
	}
}
func (u *UI) closeAfterCancel(quit func()) {
	u.mu.Lock()
	u.state.Closing = true
	if u.cancel != nil {
		u.state.Cancelling = true
		u.cancel()
	}
	done := u.done
	u.event("state", u.state)
	u.mu.Unlock()
	go func() {
		if done != nil {
			<-done
		}
		quit()
	}()
}
func preferences(s launcher.Settings) Preferences {
	return Preferences{s.LastSelectedVersion, s.LastSelectedFolder, s.DisableProtection, s.PatchGetPixel}
}
func (u *UI) Snapshot() (Snapshot, error) {
	s, err := launcher.LoadSettings(u.manager.Root)
	view := Snapshot{Preferences: preferences(s), State: u.State()}
	if err != nil {
		view.SettingsError = "設定を読み込めません。元ファイルを保全して修復してください: " + err.Error()
	}
	if _, e := launcher.LoadAdvancedSettings(u.manager.Root); e != nil {
		view.SettingsError += "\n詳細設定を保全して修復してください: " + e.Error()
	}
	view.Installed, err = u.installedNWVersions()
	if err != nil {
		return view, err
	}
	if s.LastSelectedFolder != "" && filepath.IsAbs(s.LastSelectedFolder) {
		if g, e := launcher.DetectGame(s.LastSelectedFolder); e == nil {
			view.Game = &g
		}
	}
	return view, nil
}
func (u *UI) SavePreferences(p Preferences) error {
	return u.operation("設定保存", func(context.Context) error {
		if p.Version != "" && !versionPattern.MatchString(p.Version) {
			return errors.New("NW.js の版が不正です")
		}
		// A remembered folder may have moved; only selection/launch needs it to exist.
		if p.Folder != "" && (!filepath.IsAbs(p.Folder) || strings.ContainsRune(p.Folder, 0)) {
			return errors.New("ゲームフォルダは絶対パスで指定してください")
		}
		s, err := launcher.LoadSettings(u.manager.Root)
		if err != nil {
			return err
		}
		s.LastSelectedVersion = p.Version
		s.LastSelectedFolder = p.Folder
		s.DisableProtection = p.DisableProtection
		s.PatchGetPixel = p.PatchGetPixel
		return launcher.SaveSettings(u.manager.Root, s)
	})
}
func absoluteDirectory(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return "", errors.New("絶対フォルダパスを指定してください")
	}
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("フォルダではありません")
	}
	return p, nil
}
func (u *UI) SelectFolder(path string) (game launcher.GameInfo, err error) {
	err = u.operation("ゲーム判定", func(context.Context) error {
		p, e := absoluteDirectory(path)
		if e != nil {
			return e
		}
		game, e = launcher.DetectGame(p)
		if e != nil {
			return e
		}
		s, e := launcher.LoadSettings(u.manager.Root)
		if e != nil {
			return e
		}
		s.LastSelectedFolder = p
		return launcher.SaveSettings(u.manager.Root, s)
	})
	return
}
func component(value string) (launcher.Component, error) {
	switch launcher.Component(value) {
	case launcher.ComponentMKXPZ, launcher.ComponentEasyRPG, launcher.ComponentRTP, launcher.ComponentSoundFont, launcher.ComponentKawariki:
		return launcher.Component(value), nil
	}
	if versionPattern.MatchString(value) {
		return launcher.Component(value), nil
	}
	return "", errors.New("コンポーネントが不正です")
}
func (u *UI) Run(r Request) (result Result, err error) {
	err = u.operation(r.Action, func(ctx context.Context) error {
		var c launcher.Component
		var e error
		switch r.Action {
		case "versions":
			result.Versions, e = launcher.ListNWVersions(ctx)
			return e
		case "install", "uninstall", "update":
			c, e = component(r.Component)
			if e != nil {
				return e
			}
		case "uninstall-all", "missing", "launch", "export":
		default:
			return errors.New("操作が不正です")
		}
		switch r.Action {
		case "install", "uninstall", "uninstall-all", "export":
			if !r.Approved {
				return errors.New("利用者の確認が必要です")
			}
		}
		last := time.Time{}
		progress := func(p launcher.Progress) error {
			if time.Since(last) > 100*time.Millisecond || p.Completed == p.Total {
				u.event("progress", p)
				last = time.Now()
			}
			return ctx.Err()
		}
		switch r.Action {
		case "update":
			v, e := u.manager.CheckUpdate(ctx, c)
			result.Update = &v
			return e
		case "install":
			if _, e := launcher.LoadSettings(u.manager.Root); e != nil {
				return e
			}
			installed, e := u.installComponent(ctx, c, progress)
			if e != nil {
				return e
			}
			isNW := versionPattern.MatchString(string(c))
			if isNW {
				root := filepath.Join(u.manager.Root, string(c))
				if installed.Path != filepath.Join(root, "nwjs.app") {
					return errors.New("NW.js 導入結果のパスが対象版と一致しません")
				}
				if _, e := launcher.RequireNWArm(root); e != nil {
					return fmt.Errorf("NW.js 導入結果を確認できません: %w", e)
				}
				// Discovery permits legacy receiptless ARM; installation success does not.
				if _, e := os.Lstat(filepath.Join(root, ".nwjs-install.json")); e != nil {
					return fmt.Errorf("NW.js 導入記録を確認できません: %w", e)
				}
				versions, e := u.installedNWVersions()
				if e != nil {
					return e
				}
				if !slices.Contains(versions, string(c)) {
					return errors.New("NW.js 導入結果が導入済み一覧にありません")
				}
			}
			s, e := launcher.LoadSettings(u.manager.Root)
			if e != nil {
				return e
			}
			if isNW {
				s.LastSelectedVersion = string(c)
			}
			if c == launcher.ComponentMKXPZ || installed.CommitSHA != "" {
				s.LastCommitSHA = installed.CommitSHA
			}
			if c == launcher.ComponentEasyRPG || installed.BuildNumber != 0 {
				s.LastBuildNumber = installed.BuildNumber
			}
			if e = launcher.SaveSettings(u.manager.Root, s); e != nil {
				return e
			}
			result.Path = installed.Path
			u.log("導入完了: %s", installed.Path)
			return nil
		case "uninstall":
			return u.manager.Uninstall(ctx, c)
		case "uninstall-all":
			// There is no autosave snapshot to resurrect removed settings, even on failure.
			e := u.manager.UninstallAll(ctx)
			_, reload := launcher.LoadSettings(u.manager.Root)
			return errors.Join(e, reload)
		}
		s, e := launcher.LoadSettings(u.manager.Root)
		if e != nil {
			return e
		}
		path, e := absoluteDirectory(s.LastSelectedFolder)
		if e != nil {
			return e
		}
		game, e := launcher.DetectGame(path)
		if e != nil {
			return e
		}
		switch r.Action {
		case "missing":
			result.Missing, e = u.manager.MissingComponents(game, s)
			return e
		case "export":
			dest, e := absoluteDirectory(r.Destination)
			if e != nil {
				return e
			}
			result.Path, e = u.manager.Export(ctx, game, s, dest, r.Name, progress)
			if e == nil {
				u.log("書き出し完了: %s", result.Path)
			}
			return e
		case "launch":
			p, e := u.manager.Launch(ctx, game, s)
			if e != nil {
				return e
			}
			result.PID = p.PID
			result.Path = p.LogPath
			for _, w := range p.Warnings {
				u.log("警告: %s", w)
			}
			u.log("ゲーム起動 PID %d / %s", p.PID, p.LogPath)
			go func() {
				code, e := p.Wait(context.Background())
				u.log("ゲーム終了 PID %d / code %d / %v", p.PID, code, e)
				u.event("game-exit", map[string]any{"PID": p.PID, "Code": code})
			}()
			return nil
		}
		return errors.New("操作が不正です")
	})
	return
}
func (u *UI) log(format string, args ...any) {
	if _, err := fmt.Fprintf(u.logs, format+"\n", args...); err != nil {
		u.logs.mu.Lock()
		u.logs.text = trimLog(u.logs.text + "ログ保存失敗: " + err.Error() + "\n")
		u.logs.mu.Unlock()
	}
	u.event("log", nil)
}
func (u *UI) Logs() (string, error) {
	text := u.logs.String()
	if len(text) > maxLogBytes/4 {
		text = strings.ToValidUTF8(text[len(text)-maxLogBytes/4:], "�")
	}
	for _, name := range []string{"log.txt", "launcher.log", "game.log"} {
		part, err := readLogTail(filepath.Join(u.manager.Root, name))
		if err != nil {
			return text, err
		}
		if part != "" {
			text += "\n--- " + name + " (末尾) ---\n" + part
		}
	}
	return trimLog(text), nil
}

func readLogTail(path string) (string, error) {
	text := ""
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if errors.Is(err, os.ErrNotExist) {
		return text, nil
	}
	if err != nil {
		return text, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return text, err
	}
	if !info.Mode().IsRegular() {
		return text, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	offset := info.Size() - maxLogBytes/4
	if offset < 0 {
		offset = 0
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return text, err
	}
	b, err := io.ReadAll(io.LimitReader(f, maxLogBytes/4))
	return strings.ToValidUTF8(string(b), "�"), err
}

type boundedLog struct {
	mu   sync.Mutex
	text string
	path string
}

func trimLog(s string) string {
	if len(s) > maxLogBytes {
		s = s[len(s)-maxLogBytes:]
		for len(s) > 0 && !utf8.RuneStart(s[0]) {
			s = s[1:]
		}
	}
	return s
}
func (l *boundedLog) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.text = trimLog(l.text + strings.ToValidUTF8(string(b), "�"))
	if l.path == "" {
		return len(b), nil
	}
	fd, err := syscall.Open(l.path, syscall.O_WRONLY|syscall.O_APPEND|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return 0, err
	}
	f := os.NewFile(uintptr(fd), l.path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) {
		return 0, errors.New("launcher log is not a privately owned regular file")
	}
	return f.Write(b)
}
func (l *boundedLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.text }

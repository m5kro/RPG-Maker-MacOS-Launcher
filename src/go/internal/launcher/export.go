package launcher

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

type exportResource struct{ source, relative string }

func engineSafeTree(ctx context.Context, root string) error {
	if err := realDirectory(root); err != nil {
		return err
	}
	var err error
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	count := 0
	var total int64
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		count++
		if count > maxEntries {
			return errors.New("file count limit exceeded")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode().IsRegular():
			total += info.Size()
			if total > maxExtracted {
				return errors.New("copy size limit exceeded")
			}
		case info.IsDir():
		case info.Mode()&os.ModeSymlink != 0:
			value, err := os.Readlink(p)
			if err != nil {
				return err
			}
			if filepath.IsAbs(value) || !within(root, filepath.Join(filepath.Dir(p), value)) {
				return fmt.Errorf("outward symlink: %s", p)
			}
			if err = validateLink(root, p); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported file: %s", p)
		}
		return nil
	})
}
func engineCopyFile(ctx context.Context, source, dest string, p ProgressFunc) error {
	f, err := dataContractOpenRegular(source)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	w, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	n, copyErr := copyProgress(ctx, w, f, maxExtracted, p, "copy", 0, info.Size())
	err = errors.Join(copyErr, w.Close())
	if err != nil {
		return err
	}
	if n != info.Size() {
		return errors.New("source changed while copying")
	}
	if err = os.Chmod(dest, info.Mode().Perm()); err != nil {
		return err
	}
	return p.report("copy", n, info.Size())
}
func engineCopyTree(ctx context.Context, source, dest string, p ProgressFunc) error {
	if err := engineSafeTree(ctx, source); err != nil {
		return err
	}
	var err error
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	var links []exportResource
	err = filepath.WalkDir(source, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			if err = os.MkdirAll(target, 0755); err != nil {
				return err
			}
			return realDirectory(target)
		}
		if d.Type()&os.ModeSymlink != 0 {
			links = append(links, exportResource{name, target})
			return nil
		}
		return engineCopyFile(ctx, name, target, p)
	})
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := ctx.Err(); err != nil {
			return err
		}
		value, err := os.Readlink(l.source)
		if err != nil {
			return err
		}
		if filepath.IsAbs(value) || !within(source, filepath.Join(filepath.Dir(l.source), value)) {
			return errors.New("symlink changed during copy")
		}
		if err = os.Symlink(value, l.relative); err != nil {
			return err
		}
	}
	resolvedDest, err := filepath.EvalSymlinks(dest)
	if err != nil {
		return err
	}
	for _, l := range links {
		rel, e := filepath.Rel(dest, l.relative)
		if e != nil {
			return e
		}
		if err = validateLink(resolvedDest, filepath.Join(resolvedDest, rel)); err != nil {
			return err
		}
	}
	return nil
}
func engineRelative(root, p string) (string, bool) {
	rel, err := filepath.Rel(root, p)
	return filepath.ToSlash(rel), err == nil && within(root, p)
}
func (m *Manager) exportMKXPPlan(game GameInfo, a AdvancedSettings) (map[string]any, []exportResource, error) {
	c, err := m.mkxpConfig(game, a)
	if err != nil {
		return nil, nil, err
	}
	if err = validateMKXPPaths(game, c); err != nil {
		return nil, nil, err
	}
	resources := []exportResource{}
	kawarikiCopied := false
	for _, key := range []string{"gameFolder", "RTP", "midiSoundFont", "preloadScript", "postloadScript", "iconPath", "customScript", "patches", "rubyLoadpath"} {
		v, ok := c[key]
		if !ok {
			continue
		}
		listType := key == "RTP" || key == "preloadScript" || key == "postloadScript" || key == "patches" || key == "rubyLoadpath"
		var paths []string
		if listType {
			paths, err = engineStringList(v)
			if err != nil {
				return nil, nil, err
			}
		} else {
			paths = []string{v.(string)}
		}
		for i, p := range paths {
			if p == "" {
				continue
			}
			source, err := engineResourcePath(game.Path, p)
			if err != nil {
				return nil, nil, err
			}
			resolved, err := filepath.EvalSymlinks(source)
			if err != nil {
				return nil, nil, err
			}
			source = resolved
			if key == "gameFolder" {
				if source != game.Path {
					return nil, nil, errors.New("standalone export does not support a different gameFolder")
				}
				paths[i] = "./"
				continue
			}
			if rel, inside := engineRelative(game.Path, source); inside {
				paths[i] = "./" + rel
				continue
			}
			switch key {
			case "RTP":
				relative := ".rpgm-resources/rtp-" + strconv.Itoa(i)
				resources = append(resources, exportResource{source, relative})
				paths[i] = "./" + relative
			case "midiSoundFont":
				relative := ".rpgm-resources/GMGSx.SF2"
				resources = append(resources, exportResource{source, relative})
				paths[i] = "./" + relative
			case "preloadScript":
				kawariki := filepath.Join(m.Root, "kawariki")
				rel, inside := engineRelative(kawariki, source)
				if !inside {
					return nil, nil, fmt.Errorf("external preloadScript %s is unsupported: move its complete dependency tree inside the game", p)
				}
				if !kawarikiCopied {
					resources = append(resources, exportResource{kawariki, ".rpgm-resources/kawariki"})
					kawarikiCopied = true
				}
				paths[i] = "./.rpgm-resources/kawariki/" + rel
			default:
				return nil, nil, fmt.Errorf("external %s is unsupported in standalone export: %s", key, p)
			}
		}
		if listType {
			c[key] = paths
		} else {
			c[key] = paths[0]
		}
	}
	return c, resources, nil
}
func exportEasyArguments(game GameInfo, a AdvancedSettings) ([]string, error) {
	if _, err := easyArguments(a); err != nil {
		return nil, err
	}
	values := make(map[string]any, len(a.EasyRPG))
	for key, value := range a.EasyRPG {
		values[key] = value
	}
	a.EasyRPG = values
	for _, key := range []string{"--config-path", "--font1", "--font2", "--font-path", "--rtp-path", "--save-path", "--soundfont", "--soundfont-path", "--record-input", "--replay-input", "--log-file"} {
		if !SettingEnabled(a.EasyRPGEnabled[key]) {
			continue
		}
		value := values[key]
		if disabled, ok := value.(bool); ok && !disabled {
			continue
		}
		path, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s requires a string path", key)
		}
		if path == "" {
			continue
		}
		source, err := engineResourcePath(game.Path, path)
		if err != nil {
			return nil, err
		}
		resolved, err := filepath.EvalSymlinks(source)
		if err != nil {
			return nil, fmt.Errorf("standalone %s needs an existing in-game path: %w", key, err)
		}
		rel, inside := engineRelative(game.Path, resolved)
		if !inside {
			return nil, fmt.Errorf("external %s cannot be used by standalone export; put it inside the game", key)
		}
		values[key] = "./" + rel
	}
	return easyArguments(a)
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func engineXML(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Export preflights paths/settings before creating an owned temporary output.
// Original game/runtime files are read-only. Existing destinations are never replaced.
func (m *Manager) Export(ctx context.Context, game GameInfo, s Settings, dest, name string, progress ProgressFunc) (result string, err error) {
	if err = engineContext(ctx); err != nil {
		return "", err
	}
	if strings.TrimSpace(name) != name || name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") || strings.ContainsAny(name, "/\\:") || strings.IndexFunc(name, unicode.IsControl) >= 0 || !utf8.ValidString(name) || len(name) > 200 {
		return "", errors.New("invalid standalone application name")
	}
	if dest == "" {
		return "", errors.New("select an export destination")
	}
	dest, err = filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	if err = realDirectory(dest); err != nil {
		return "", err
	}
	dest, err = filepath.EvalSymlinks(dest)
	if err != nil {
		return "", err
	}
	release, err := AcquireManagedLock(m.Root)
	if err != nil {
		return "", err
	}
	defer release()
	executable, err := m.CheckRuntime(game, s)
	if err != nil {
		return "", err
	}
	game, err = engineGame(game)
	if err != nil {
		return "", err
	}
	target := filepath.Join(dest, name+".app")
	if within(game.Path, target) || within(m.Root, target) {
		return "", errors.New("output must be outside the game and managed runtime root")
	}
	if _, err = os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("standalone destination already exists or is inaccessible")
	}
	if err = engineSafeTree(ctx, game.Path); err != nil {
		return "", err
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if err = engineSafeTree(ctx, bundle); err != nil {
		return "", err
	}
	a, err := LoadAdvancedSettings(m.Root)
	if err != nil {
		return "", err
	}
	var config map[string]any
	var resources []exportResource
	var args []string
	switch game.Engine {
	case MKXPZ:
		missing, e := m.MissingComponents(game, s)
		if e != nil {
			return "", e
		}
		if len(missing) != 0 {
			return "", fmt.Errorf("install or repair required components first: %v", missing)
		}
		config, resources, err = m.exportMKXPPlan(game, a)
	case EasyRPG:
		args, err = exportEasyArguments(game, a)
	}
	if err != nil {
		return "", err
	}
	if len(resources) > 0 {
		if _, err = os.Lstat(filepath.Join(game.Path, ".rpgm-resources")); !errors.Is(err, os.ErrNotExist) {
			return "", errors.New("game uses reserved .rpgm-resources directory")
		}
	}
	for _, r := range resources {
		info, e := os.Lstat(r.source)
		if e != nil {
			return "", e
		}
		if info.IsDir() {
			if within(r.source, target) {
				return "", errors.New("output must be outside every copied resource directory")
			}
			if e = engineSafeTree(ctx, r.source); e != nil {
				return "", e
			}
		} else if !info.Mode().IsRegular() {
			return "", fmt.Errorf("unsafe export resource %s", r.source)
		}
	}
	if err = progress.report("copy", 0, 0); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(dest, ".rpgm-export-")
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	app := filepath.Join(stage, "payload.app")
	gameDir := ""
	if game.Engine == EasyRPG {
		gameDir = filepath.Join(app, "Contents", "Resources", "game")
		if err = engineCopyTree(ctx, bundle, filepath.Join(app, "Contents", "Resources", "EasyRPG Player.app"), progress); err != nil {
			return "", err
		}
	} else {
		if err = engineCopyTree(ctx, bundle, app, progress); err != nil {
			return "", err
		}
		gameDir = filepath.Join(app, "Contents", "Resources", "app.nw")
		if game.Engine == MKXPZ {
			gameDir = filepath.Join(app, "Contents", "Game")
		}
		// Only the owned runtime copy's placeholder game is replaced, never the source.
		if err = os.RemoveAll(gameDir); err != nil {
			return "", err
		}
	}
	if err = engineCopyTree(ctx, game.Path, gameDir, progress); err != nil {
		return "", err
	}
	switch game.Engine {
	case NWJS:
		if _, err = m.prepareNW(gameDir, s); err != nil {
			return "", err
		}
	case MKXPZ:
		for _, r := range resources {
			to := filepath.Join(gameDir, filepath.FromSlash(r.relative))
			info, e := os.Stat(r.source)
			if e != nil {
				return "", e
			}
			if info.IsDir() {
				err = engineCopyTree(ctx, r.source, to, progress)
			} else {
				err = engineCopyFile(ctx, r.source, to, progress)
			}
			if err != nil {
				return "", err
			}
		}
		if err = engineWriteJSON(filepath.Join(gameDir, "mkxp.json"), config); err != nil {
			return "", err
		}
	case EasyRPG:
		script := "#!/bin/sh\nset -eu\nHERE=$(CDPATH= cd -- \"$(dirname -- \"$0\")/../Resources\" && pwd)\ncd -- \"$HERE/game\"\nexec /usr/bin/arch -arm64 \"$HERE/EasyRPG Player.app/Contents/MacOS/EasyRPG Player\" --window --project-path \"$HERE/game\""
		for _, arg := range args {
			script += " " + shellQuote(arg)
		}
		script += "\n"
		if err = os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0755); err != nil {
			return "", err
		}
		if err = os.WriteFile(filepath.Join(app, "Contents", "MacOS", "launch"), []byte(script), 0755); err != nil {
			return "", err
		}
		plist := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleExecutable</key><string>launch</string><key>CFBundleName</key><string>` + engineXML(name) + `</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`
		if err = os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0644); err != nil {
			return "", err
		}
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = progress.report("publish", 1, 1); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = os.Mkdir(target, 0700); err != nil {
		return "", fmt.Errorf("destination appeared; preserving it: %w", err)
	}
	if err = syscall.Rename(app, target); err != nil {
		return "", errors.Join(err, os.Remove(target))
	}
	return target, nil
}

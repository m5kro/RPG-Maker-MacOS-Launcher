package launcher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Component string

const (
	ComponentMKXPZ     Component = "MKXPZ"
	ComponentEasyRPG   Component = "EasyRPG"
	ComponentRTP       Component = "RTP"
	ComponentSoundFont Component = "SoundFont"
	ComponentKawariki  Component = "Kawariki"
)

// A stable NW.js version string is also a Component, e.g. Component("v0.100.0").
type UpdateInfo struct {
	Component   Component
	Current     string
	Latest      string
	Available   bool
	DownloadURL string
}

// InstallResult does not save settings. On successful MKXP-Z installation, GUI
// must save CommitSHA even when empty (unknown), while preserving other settings.
type InstallResult struct {
	Path        string
	CommitSHA   string
	BuildNumber int
}

const componentRelease = "https://github.com/m5kro/mkxp-z/releases/download/launcher/"
const easyJob = "https://ci.easyrpg.org/job/player-macos/"

func componentName(c Component) string {
	switch c {
	case ComponentMKXPZ:
		return "Z-universal.app"
	case ComponentEasyRPG:
		return "EasyRPG Player.app"
	case ComponentRTP:
		return "RTP"
	case ComponentSoundFont:
		return "GMGSx.SF2"
	case ComponentKawariki:
		return "kawariki"
	}
	if versionPattern.MatchString(string(c)) {
		return string(c)
	}
	return ""
}
func componentRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many component redirects")
	}
	u := req.URL
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return errors.New("component download requires HTTPS")
	}
	// Separate from NW.js's official allowlist; no caller-supplied download URLs.
	initial := u.Hostname()
	if len(via) > 0 {
		initial = via[0].URL.Hostname()
	}
	host := u.Hostname()
	allowed := host == initial
	if initial == "github.com" {
		allowed = host == "github.com" || host == "release-assets.githubusercontent.com" || host == "objects.githubusercontent.com"
	}
	if !allowed {
		return fmt.Errorf("refusing component redirect to %s", host)
	}
	return nil
}
func (m *Manager) componentConfig() config { return config{client: m.client} }
func (m *Manager) CheckUpdate(ctx context.Context, c Component) (UpdateInfo, error) {
	u := UpdateInfo{Component: c}
	if err := engineContext(ctx); err != nil {
		return u, err
	}
	s, err := LoadSettings(m.Root)
	if err != nil {
		return u, err
	}
	switch c {
	case ComponentMKXPZ:
		u.Current = s.LastCommitSHA
		u.DownloadURL = componentRelease + "Z-universal.zip"
		b, err := m.componentConfig().metadata(ctx, "https://api.github.com/repos/m5kro/mkxp-z/commits/dev", maxMetadata)
		if err != nil {
			return u, err
		}
		var data struct {
			SHA string `json:"sha"`
		}
		if err = json.Unmarshal(b, &data); err != nil {
			return u, err
		}
		if !regexp.MustCompile(`^[a-fA-F0-9]{40}$`).MatchString(data.SHA) {
			return u, errors.New("invalid MKXP-Z commit SHA")
		}
		u.Latest = data.SHA
	case ComponentEasyRPG:
		if s.LastBuildNumber > 0 {
			u.Current = strconv.Itoa(s.LastBuildNumber)
		}
		u.DownloadURL = easyJob + "lastSuccessfulBuild/artifact/osxbuild/EasyRPG-Player-macos.app.zip"
		b, err := m.componentConfig().metadata(ctx, easyJob+"lastSuccessfulBuild/api/json", maxMetadata)
		if err != nil {
			return u, err
		}
		u.DownloadURL = ""
		var data struct {
			Number    int    `json:"number"`
			Result    string `json:"result"`
			Building  bool   `json:"building"`
			Artifacts []struct {
				RelativePath string `json:"relativePath"`
			} `json:"artifacts"`
		}
		if err = json.Unmarshal(b, &data); err != nil {
			return u, err
		}
		if data.Number <= 0 || data.Building || data.Result != "SUCCESS" {
			return u, errors.New("EasyRPG has no completed successful build")
		}
		for _, a := range data.Artifacts {
			p := a.RelativePath
			if path.Base(p) != "EasyRPG-Player-macos.app.zip" {
				continue
			}
			if path.IsAbs(p) || path.Clean(p) != p || strings.ContainsAny(p, "\\\x00:?#") || strings.HasPrefix(p, "../") {
				return u, errors.New("unsafe EasyRPG artifact path")
			}
			if u.DownloadURL != "" {
				return u, errors.New("ambiguous EasyRPG artifact")
			}
			parts := strings.Split(p, "/")
			for i := range parts {
				parts[i] = url.PathEscape(parts[i])
			}
			u.DownloadURL = easyJob + strconv.Itoa(data.Number) + "/artifact/" + strings.Join(parts, "/")
		}
		if u.DownloadURL == "" {
			return u, errors.New("successful EasyRPG build has no macOS app artifact; refusing to label an unrelated latest download")
		}
		u.Latest = strconv.Itoa(data.Number)
	default:
		return u, fmt.Errorf("update check unsupported for %q (NW.js uses ListNWVersions)", c)
	}
	u.Available = u.Current != u.Latest
	return u, nil
}
func (m *Manager) InstallComponent(ctx context.Context, c Component, progress ProgressFunc) (result InstallResult, err error) {
	if err = engineContext(ctx); err != nil {
		return result, err
	}
	name := componentName(c)
	if name == "" {
		return result, fmt.Errorf("unknown component %q", c)
	}
	if versionPattern.MatchString(string(c)) {
		result.Path, err = InstallNW(ctx, m.Root, string(c), progress)
		return result, err
	}
	release, err := AcquireManagedLock(m.Root)
	if err != nil {
		return result, err
	}
	defer release()
	var source string
	var update UpdateInfo
	switch c {
	case ComponentMKXPZ, ComponentEasyRPG:
		update, err = m.CheckUpdate(ctx, c)
		if err != nil {
			if ctx.Err() != nil {
				return result, err
			}
			if update.DownloadURL == "" {
				return result, err
			}
			err = nil
		}
		source = update.DownloadURL
	case ComponentRTP:
		source = componentRelease + "RTP.zip"
	case ComponentKawariki:
		source = componentRelease + "kawariki.zip"
	case ComponentSoundFont:
		source = componentRelease + "GMGSx.SF2"
	}
	stage, err := os.MkdirTemp(m.Root, ".runtime-stage-")
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	archive := filepath.Join(stage, "download")
	if err = m.downloadComponent(ctx, source, archive, progress); err != nil {
		return result, err
	}
	payload := archive
	if c != ComponentSoundFont {
		unpacked := filepath.Join(stage, "unpacked")
		if err = os.Mkdir(unpacked, 0700); err != nil {
			return result, err
		}
		if err = extractZIP(ctx, archive, unpacked, name, progress, c == ComponentMKXPZ || c == ComponentEasyRPG || c == ComponentKawariki); err != nil {
			return result, err
		}
		payload = filepath.Join(unpacked, name)
		if c == ComponentRTP || c == ComponentKawariki {
			if err = engineWriteJSON(filepath.Join(payload, ".rpgm-complete.json"), map[string]string{"component": string(c)}); err != nil {
				return result, err
			}
		}
	}
	if c == ComponentMKXPZ || c == ComponentEasyRPG {
		exe := "Z-universal"
		if c == ComponentEasyRPG {
			exe = "EasyRPG Player"
		}
		binaryPath := filepath.Join(payload, "Contents", "MacOS", exe)
		info, e := regularFile(binaryPath)
		if e != nil {
			return result, e
		}
		if err = os.Chmod(binaryPath, info.Mode().Perm()|0100); err != nil {
			return result, err
		}
	}
	if err = checkComponent(payload, c); err != nil {
		return result, err
	}
	if err = progress.report("verify", 1, 1); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	target := filepath.Join(m.Root, name)
	if c == ComponentRTP || c == ComponentKawariki {
		if info, e := os.Lstat(target); e == nil && info.IsDir() {
			if _, e = os.Lstat(filepath.Join(target, ".rpgm-complete.json")); errors.Is(e, os.ErrNotExist) {
				return result, fmt.Errorf("refusing to replace markerless %s: it may contain user data; back it up and move it before reinstalling", name)
			} else if e != nil {
				return result, e
			}
		} else if e != nil && !errors.Is(e, os.ErrNotExist) {
			return result, e
		}
	}
	if err = m.publishComponent(payload, target); err != nil {
		return result, err
	}
	result.Path = target
	if c == ComponentMKXPZ {
		result.CommitSHA = update.Latest
	}
	if c == ComponentEasyRPG {
		result.BuildNumber, _ = strconv.Atoi(update.Latest)
	}
	return result, nil
}

// Accept only the macOS AppleDouble layout with empty resource fork and known FinderInfo.
// Unknown forks/attributes fail closed rather than silently losing runtime data.
func validateComponentAppleDouble(b []byte, name string) error {
	invalid := errors.New("unsupported AppleDouble metadata (refusing to discard unknown data)")
	be := binary.BigEndian
	if len(b) < 120 || be.Uint32(b[:4]) != 0x00051607 || be.Uint32(b[4:8]) != 0x00020000 || be.Uint16(b[24:26]) != 2 {
		return invalid
	}
	finderInfo := make([]byte, 34)
	if path.Base(name) == "._.DS_Store" && bytes.Equal(b[50:60], []byte("        \x40\x10")) {
		// Finder's .DS_Store uses blank type/creator plus invisible and initialized flags.
		copy(finderInfo, b[50:60])
	}
	// FinderInfo (ID 9) occupies the body; resource fork (ID 2) must be empty at EOF.
	if be.Uint32(b[26:30]) != 9 || be.Uint32(b[30:34]) != 50 || uint64(be.Uint32(b[34:38])) != uint64(len(b)-50) ||
		be.Uint32(b[38:42]) != 2 || uint64(be.Uint32(b[42:46])) != uint64(len(b)) || be.Uint32(b[46:50]) != 0 ||
		!bytes.Equal(b[50:84], finderInfo) || string(b[84:88]) != "ATTR" {
		return invalid
	}
	dataStart := uint64(be.Uint32(b[96:100]))
	if uint64(be.Uint32(b[92:96])) != uint64(len(b)) || dataStart < 120 || dataStart > uint64(len(b)) ||
		uint64(be.Uint32(b[100:104])) != uint64(len(b))-dataStart || !bytes.Equal(b[104:118], make([]byte, 14)) {
		return invalid
	}
	p, data := uint64(120), dataStart
	seen := map[string]bool{}
	for i := 0; i < int(be.Uint16(b[118:120])); i++ {
		if p+11 > dataStart {
			return invalid
		}
		offset, size := uint64(be.Uint32(b[p:p+4])), uint64(be.Uint32(b[p+4:p+8]))
		n := uint64(b[p+10])
		if n < 2 || p+11+n > dataStart || b[p+10+n] != 0 || be.Uint16(b[p+8:p+10]) != 0 || offset != data || size > uint64(len(b))-data {
			return invalid
		}
		name := string(b[p+11 : p+10+n])
		switch name {
		case "com.apple.provenance", "com.apple.TextEncoding", "com.apple.lastuseddate#PS", "com.apple.quarantine":
			// These are distributor metadata, not runtime data; never apply their xattrs.
		default:
			if !strings.HasPrefix(name, "com.apple.metadata:kMDLabel_") || strings.ContainsRune(name, 0) {
				return invalid
			}
		}
		if seen[name] {
			return invalid
		}
		seen[name] = true
		p = (p + 11 + n + 3) &^ 3
		data += size
	}
	if p != dataStart || data != uint64(len(b)) {
		return invalid
	}
	return nil
}

func (m *Manager) downloadComponent(ctx context.Context, source, dest string, p ProgressFunc) error {
	resp, err := m.componentConfig().get(ctx, source)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.ContentLength > maxArchive {
		return errors.New("component download exceeds size limit")
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	if err = p.report("download", 0, total); err != nil {
		return err
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := copyProgress(ctx, f, resp.Body, maxArchive, p, "download", 0, total)
	err = errors.Join(copyErr, f.Close())
	if err != nil {
		return err
	}
	if n == 0 || (resp.ContentLength >= 0 && n != resp.ContentLength) {
		return io.ErrUnexpectedEOF
	}
	return p.report("download", n, n)
}
func checkComponent(p string, c Component) error {
	switch c {
	case ComponentMKXPZ, ComponentEasyRPG:
		exe := "Z-universal"
		if c == ComponentEasyRPG {
			exe = "EasyRPG Player"
		}
		for _, dir := range []string{p, filepath.Join(p, "Contents"), filepath.Join(p, "Contents", "MacOS")} {
			if err := realDirectory(dir); err != nil {
				return err
			}
		}
		return RequireARM64(filepath.Join(p, "Contents", "MacOS", exe))
	case ComponentSoundFont:
		return checkSoundFont(p)
	case ComponentRTP, ComponentKawariki:
		if err := realDirectory(p); err != nil {
			return err
		}
		b, err := dataContractReadRegular(filepath.Join(p, ".rpgm-complete.json"))
		if errors.Is(err, os.ErrNotExist) {
			if err = checkLegacyResource(p, c); err != nil {
				return fmt.Errorf("resource has no completed-install marker and does not match the legacy distribution: %w", err)
			}
		} else if err != nil {
			return err
		} else {
			var marker map[string]string
			if err = json.Unmarshal(b, &marker); err != nil || marker["component"] != string(c) {
				return errors.New("invalid resource completion marker")
			}
		}
		if c == ComponentKawariki {
			f, err := regularFile(filepath.Join(p, "preload.rb"))
			if err != nil {
				return err
			}
			if f.Size() == 0 {
				return errors.New("empty Kawariki preload")
			}
			return nil
		}
		for _, n := range []string{"Standard", "RPGVX", "RPGVXace"} {
			dir := filepath.Join(p, n)
			if err := realDirectory(dir); err != nil {
				return err
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return fmt.Errorf("empty RTP %s", n)
			}
		}
		return nil
	}
	return fmt.Errorf("unknown component %q", c)
}

// The Python installer extracted these release archives directly, without a receipt.
// Hash every distributed file; do not write a marker into the user's directory.
func checkLegacyResource(p string, c Component) error {
	want := map[Component]string{
		ComponentRTP:      "3832debd08a0fcf1093fc0267a6fcc1f9e476c2f42ec1dbe42e664572cf245ac",
		ComponentKawariki: "785c614ba701453b68b6613c5d363391744fadc75ce5f8148f8a15c8a67702aa",
	}[c]
	digest, err := legacyResourceDigest(p)
	if err != nil {
		return err
	}
	if digest != want {
		return errors.New("legacy resource contents differ")
	}
	return nil
}

func legacyResourceDigest(p string) (string, error) {
	var files []string
	err := filepath.WalkDir(p, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == ".DS_Store" {
			return nil
		}
		files = append(files, name)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, name := range files {
		rel, err := filepath.Rel(p, name)
		if err != nil {
			return "", err
		}
		f, err := dataContractOpenRegular(name)
		if err != nil {
			return "", err
		}
		fileHash := sha256.New()
		_, readErr := io.Copy(fileHash, f)
		if err = errors.Join(readErr, f.Close()); err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, filepath.ToSlash(rel)+"\x00")
		_, _ = h.Write(fileHash.Sum(nil))
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func checkSoundFont(p string) error {
	f, err := dataContractOpenRegular(p)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	var h [12]byte
	if _, err = io.ReadFull(f, h[:]); err != nil {
		return err
	}
	if string(h[:4]) != "RIFF" || string(h[8:]) != "sfbk" || int64(binary.LittleEndian.Uint32(h[4:8]))+8 != info.Size() {
		return errors.New("invalid or truncated SF2 (RIFF sfbk required)")
	}
	seen := map[string]bool{}
	remaining := info.Size() - 12
	for remaining > 0 {
		var chunk [8]byte
		if remaining < 8 {
			return io.ErrUnexpectedEOF
		}
		if _, err = io.ReadFull(f, chunk[:]); err != nil {
			return err
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		padded := size + (size & 1)
		if padded > remaining-8 {
			return io.ErrUnexpectedEOF
		}
		if string(chunk[:4]) == "LIST" && size >= 4 {
			var kind [4]byte
			if _, err = io.ReadFull(f, kind[:]); err != nil {
				return err
			}
			seen[string(kind[:])] = true
			if _, err = f.Seek(padded-4, io.SeekCurrent); err != nil {
				return err
			}
		} else {
			if _, err = f.Seek(padded, io.SeekCurrent); err != nil {
				return err
			}
		}
		remaining -= 8 + padded
	}
	if !seen["INFO"] || !seen["sdta"] || !seen["pdta"] {
		return errors.New("SF2 missing required LIST chunks")
	}
	return nil
}
func (m *Manager) publishComponent(payload, target string) error {
	rename := m.renameComponent
	if rename == nil {
		rename = os.Rename
	}
	backup := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".previous")
	if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		return errors.New("existing recovery copy must be inspected before replacement")
	}
	previous := false
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("unsafe component target")
		}
		previous = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if previous {
		if err := rename(target, backup); err != nil {
			return err
		}
	}
	if err := rename(payload, target); err != nil {
		if previous {
			return errors.Join(err, rename(backup, target))
		}
		return err
	}
	if previous {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("installed, but recovery copy cleanup failed: %w", err)
		}
	}
	return nil
}

func (m *Manager) managedNWVersion(name string) bool {
	dir := filepath.Join(m.Root, name)
	if realDirectory(dir) != nil {
		return false
	}
	// A receipt also identifies a damaged managed install; legacy installs need valid ARM.
	if _, err := regularFile(filepath.Join(dir, ".nwjs-install.json")); err == nil {
		return true
	}
	_, err := RequireNWArm(dir)
	return err == nil
}

// Uninstall(MKXPZ) also removes its RTP, soundfont and Kawariki, matching the GUI.
// Settings are retained. Running games hold the same OS lock and block deletion.
func (m *Manager) Uninstall(ctx context.Context, c Component) error {
	if err := engineContext(ctx); err != nil {
		return err
	}
	name := componentName(c)
	if name == "" {
		return fmt.Errorf("unknown component %q", c)
	}
	release, err := AcquireManagedLock(m.Root)
	if err != nil {
		return err
	}
	defer release()
	if versionPattern.MatchString(name) && !m.managedNWVersion(name) {
		if _, e := os.Lstat(filepath.Join(m.Root, name)); errors.Is(e, os.ErrNotExist) {
			return nil
		}
		return errors.New("refusing to remove an unrecognized version-like directory")
	}
	names := []string{name}
	if c == ComponentMKXPZ {
		names = append(names, "RTP", "GMGSx.SF2", "kawariki")
	}
	for _, n := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Join(m.Root, n)); err != nil {
			return err
		}
	}
	return nil
}

// UninstallAll removes recognized components and the five settings files, but
// preserves Root, the persistent lock inode, and unknown user files. After any
// result GUI must reload settings and discard stale close-time autosave snapshots:
// cancellation/IO errors may leave a partially completed uninstall.
func (m *Manager) UninstallAll(ctx context.Context) error {
	if err := engineContext(ctx); err != nil {
		return err
	}
	release, err := AcquireManagedLock(m.Root)
	if err != nil {
		return err
	}
	defer release()
	entries, err := os.ReadDir(m.Root)
	if err != nil {
		return err
	}
	known := map[string]bool{"Z-universal.app": true, "EasyRPG Player.app": true, "RTP": true, "GMGSx.SF2": true, "kawariki": true, "settings.json": true, "enabled-mkxpz-settings.json": true, "mkxpz-advanced.json": true, "enabled-easyrpg-settings.json": true, "easyrpg-advanced.json": true}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if known[entry.Name()] || (versionPattern.MatchString(entry.Name()) && m.managedNWVersion(entry.Name())) {
			if err := os.RemoveAll(filepath.Join(m.Root, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

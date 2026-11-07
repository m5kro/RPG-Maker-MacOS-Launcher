package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Manager fields are configuration: do not change them while an operation is running.
// ResourcesDir is the directory containing bg.js, disable-child.js and disable-net.js.
// Advanced settings are loaded from Root for each operation; no downloads are implicit.
type Manager struct {
	Root         string
	ResourcesDir string
	Log          io.Writer
	client       *http.Client
	// Tests replace transport/command creation, never production URL or ARM validation.
	command         func(string, ...string) *exec.Cmd
	renameComponent func(string, string) error // Private transaction fault injection; nil uses os.Rename.
	logMu           sync.Mutex
}

func NewManager(root, resourcesDir string, log io.Writer) (*Manager, error) {
	if err := EnsureSupportedHost(runtime.GOOS, runtime.GOARCH); err != nil {
		return nil, err
	}
	root, err := managedRoot(root)
	if err != nil {
		return nil, err
	}
	if resourcesDir != "" {
		resourcesDir, err = filepath.Abs(resourcesDir)
		if err != nil {
			return nil, err
		}
	}
	if log == nil {
		log = io.Discard
	}
	c := officialConfig()
	c.client.CheckRedirect = componentRedirect
	return &Manager{Root: root, ResourcesDir: resourcesDir, Log: log, client: c.client, command: exec.Command}, nil
}
func (m *Manager) logf(format string, args ...any) error {
	m.logMu.Lock()
	defer m.logMu.Unlock()
	_, err := fmt.Fprintf(m.Log, format, args...)
	return err
}

// CheckRuntime is read-only and permits receiptless legacy ARM runtimes for launch.
func (m *Manager) CheckRuntime(game GameInfo, s Settings) (string, error) {
	if game.Engine == NWJS {
		if !versionPattern.MatchString(s.LastSelectedVersion) {
			return "", errors.New("select a valid NW.js version")
		}
		return RequireNWArm(filepath.Join(m.Root, s.LastSelectedVersion))
	}
	var bundle, exe string
	switch game.Engine {
	case MKXPZ:
		bundle, exe = "Z-universal.app", "Z-universal"
	case EasyRPG:
		bundle, exe = "EasyRPG Player.app", "EasyRPG Player"
	default:
		return "", fmt.Errorf("unknown engine %q", game.Engine)
	}
	p := filepath.Join(m.Root, bundle, "Contents", "MacOS", exe)
	for _, d := range []string{m.Root, filepath.Join(m.Root, bundle), filepath.Join(m.Root, bundle, "Contents"), filepath.Dir(p)} {
		if err := realDirectory(d); err != nil {
			return "", err
		}
	}
	return p, RequireARM64(p)
}

// InstalledNWVersions is a read-only snapshot, newest numeric version first.
// Discovery accepts legacy ARM runtimes; InstallNW still requires its receipt.
func (m *Manager) InstalledNWVersions() ([]string, error) {
	entries, err := os.ReadDir(m.Root)
	if err != nil {
		return nil, err
	}
	versions := []string{}
	for _, entry := range entries {
		if !versionPattern.MatchString(entry.Name()) {
			continue
		}
		if _, err := RequireNWArm(filepath.Join(m.Root, entry.Name())); err == nil {
			versions = append(versions, entry.Name())
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		left, right := strings.Split(versions[i][1:], "."), strings.Split(versions[j][1:], ".")
		for n := range left {
			a, _ := strconv.Atoi(left[n])
			b, _ := strconv.Atoi(right[n])
			if a != b {
				return a > b
			}
		}
		return false
	})
	return versions, nil
}

// MissingComponents returns components requiring installation or repair, without downloading.
func (m *Manager) MissingComponents(game GameInfo, s Settings) ([]Component, error) {
	result := []Component{}
	switch game.Engine {
	case NWJS:
		if !versionPattern.MatchString(s.LastSelectedVersion) {
			return nil, errors.New("select a valid NW.js version")
		}
	case MKXPZ, EasyRPG:
	default:
		return nil, fmt.Errorf("unknown engine %q", game.Engine)
	}
	if _, err := m.CheckRuntime(game, s); err != nil {
		c := Component(game.Engine)
		if game.Engine == NWJS {
			c = Component(s.LastSelectedVersion)
		}
		result = append(result, c)
	}
	if game.Engine == MKXPZ {
		a, err := LoadAdvancedSettings(m.Root)
		if err != nil {
			return nil, err
		}
		for _, v := range []struct {
			key string
			c   Component
		}{{"RTP", ComponentRTP}, {"midiSoundFont", ComponentSoundFont}, {"preloadScript", ComponentKawariki}} {
			if SettingEnabled(a.MKXPZEnabled[v.key]) && engineEmpty(a.MKXPZ[v.key]) {
				if err := checkComponent(filepath.Join(m.Root, componentName(v.c)), v.c); err != nil {
					result = append(result, v.c)
				}
			}
		}
	}
	return result, nil
}
func engineEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	}
	return false
}
func engineZero(v any) bool {
	switch x := v.(type) {
	case json.Number:
		f, e := x.Float64()
		return e == nil && f == 0
	case int:
		return x == 0
	case float64:
		return x == 0
	}
	return false
}
func (m *Manager) mkxpConfig(game GameInfo, a AdvancedSettings) (map[string]any, error) {
	c := map[string]any{}
	p := filepath.Join(m.Root, "Z-universal.app", "Contents", "Game", "mkxp.json")
	if b, err := dataContractReadRegular(p); err == nil {
		txt, _, err := dataContractDecode(b)
		if err != nil {
			return nil, err
		}
		c, err = dataContractObject([]byte(strings.TrimPrefix(txt, "\ufeff")))
		if err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	rtp, rgss, err := GetRTPValue(game.Path)
	if err != nil {
		return nil, err
	}
	automatic := map[string]any{"gameFolder": game.Path, "RTP": []string{filepath.Join(m.Root, "RTP", rtp)}, "midiSoundFont": filepath.Join(m.Root, "GMGSx.SF2"), "preloadScript": []string{filepath.Join(m.Root, "kawariki", "preload.rb")}, "rgssVersion": rgss}
	for k, enabled := range a.MKXPZEnabled {
		if !SettingEnabled(enabled) {
			delete(c, k)
			continue
		}
		v, ok := a.MKXPZ[k]
		if !ok {
			return nil, fmt.Errorf("enabled MKXP-Z setting %s has no value", k)
		}
		if d, auto := automatic[k]; auto && (engineEmpty(v) || (k == "rgssVersion" && engineZero(v))) {
			v = d
		}
		c[k] = v
	}
	return c, nil
}
func easyArguments(a AdvancedSettings) ([]string, error) {
	keys := make([]string, 0, len(a.EasyRPGEnabled))
	for k, v := range a.EasyRPGEnabled {
		if SettingEnabled(v) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	args := []string{}
	for _, k := range keys {
		if !strings.HasPrefix(k, "--") || strings.ContainsAny(k, " \t\r\n\x00=") {
			return nil, fmt.Errorf("invalid EasyRPG option %q", k)
		}
		if k == "--project-path" {
			return nil, errors.New("--project-path is controlled by the selected game")
		}
		v, ok := a.EasyRPG[k]
		if !ok {
			return nil, fmt.Errorf("enabled EasyRPG option %s has no value", k)
		}
		switch x := v.(type) {
		case bool:
			if x {
				args = append(args, k)
			}
		case string:
			if strings.ContainsRune(x, 0) {
				return nil, fmt.Errorf("NUL in %s", k)
			}
			if x != "" {
				args = append(args, k, x)
			}
		case json.Number:
			if _, err := json.Marshal(x); err != nil {
				return nil, err
			}
			args = append(args, k, x.String())
		case int:
			args = append(args, k, strconv.Itoa(x))
		case float64:
			b, err := json.Marshal(x)
			if err != nil {
				return nil, err
			}
			args = append(args, k, string(b))
		default:
			return nil, fmt.Errorf("unsupported EasyRPG value for %s: %T", k, v)
		}
	}
	return args, nil
}

// A non-RPG NW.js game may have no Bitmap.getPixel. Keep the original warning-
// not-fatal behavior. Export warnings go to Log; Launch also returns them on Process.
func (m *Manager) prepareNW(folder string, s Settings) ([]string, error) {
	result, err := PreparePackage(folder, m.ResourcesDir, s.DisableProtection)
	if err != nil {
		return nil, err
	}
	warnings := append([]string(nil), result.Warnings...)
	if _, err = PatchGetPixel(folder, s.PatchGetPixel); err != nil {
		warnings = append(warnings, "getPixel: "+err.Error())
	}
	for _, warning := range warnings {
		if err = m.logf("Warning: %s\n", warning); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}
func engineWriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return dataContractAtomic(path, append(b, '\n'), 0600)
}
func engineContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	return ctx.Err()
}

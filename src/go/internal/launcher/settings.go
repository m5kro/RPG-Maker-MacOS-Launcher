package launcher

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// Settings retains the existing settings.json keys; Extra preserves future keys.
type Settings struct {
	OptimizeSpace       bool                       `json:"optimize_space"`
	DisableProtection   bool                       `json:"disable_protection"`
	PatchGetPixel       bool                       `json:"patch_getpixel"`
	LastSelectedVersion string                     `json:"last_selected_version"`
	LastSelectedFolder  string                     `json:"last_selected_folder"`
	LauncherVersion     string                     `json:"launcher_version"`
	LastCommitSHA       string                     `json:"last_commit_sha"`
	LastBuildNumber     int                        `json:"last_build_number"`
	Extra               map[string]json.RawMessage `json:"-"`
}

// AdvancedSettings preserves JSON types, including json.Number for exact numbers.
// Text entered for a string setting must remain a string, even "false" or "0012".
type AdvancedSettings struct {
	MKXPZEnabled   map[string]any
	MKXPZ          map[string]any
	EasyRPGEnabled map[string]any
	EasyRPG        map[string]any
}

func DefaultRoot(home string) string {
	return filepath.Join(home, "Library", "Application Support", "RPGM-Launcher")
}
func DefaultSettings(home string) Settings {
	return Settings{PatchGetPixel: true, LastSelectedFolder: filepath.Join(home, "Downloads")}
}
func SettingEnabled(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "True" || v == "Force"
	}
	return false
}

func (s *Settings) UnmarshalJSON(b []byte) error {
	object, err := dataContractObject(b)
	if err != nil {
		return err
	}
	type plain Settings
	next := plain(*s)
	next.Extra = make(map[string]json.RawMessage)
	known, _ := json.Marshal(plain{})
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(known, &keys)
	for k, v := range object {
		if k == "extract_game_en" || k == "add_cheat_menu" || k == "optimize_space" {
			delete(object, k)
			continue
		}
		if _, ok := keys[k]; ok {
			if v == nil {
				return fmt.Errorf("settings %s must not be null", k)
			}
		} else {
			raw, _ := json.Marshal(v)
			next.Extra[k] = raw
		}
	}
	clean, err := json.Marshal(object)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(clean, &next); err != nil {
		return err
	}
	next.OptimizeSpace = false
	*s = Settings(next)
	return nil
}
func (s Settings) MarshalJSON() ([]byte, error) {
	type plain Settings
	s.OptimizeSpace = false
	b, err := json.Marshal(plain(s))
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	for k, v := range s.Extra {
		if k == "extract_game_en" || k == "add_cheat_menu" {
			continue
		}
		if _, known := obj[k]; !known {
			obj[k] = v
		}
	}
	return json.Marshal(obj)
}
func LoadSettings(root string) (Settings, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Settings{}, err
	}
	s := DefaultSettings(home)
	b, err := dataContractReadRegular(filepath.Join(root, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}
func SaveSettings(root string, s Settings) error {
	path := filepath.Join(root, "settings.json")
	if b, e := dataContractReadRegular(path); e == nil {
		var existing Settings
		if e = json.Unmarshal(b, &existing); e != nil {
			return fmt.Errorf("preserving invalid %s: %w", path, e)
		}
		if s.Extra == nil {
			s.Extra = existing.Extra
		} else {
			merged := make(map[string]json.RawMessage)
			for k, v := range existing.Extra {
				merged[k] = v
			}
			for k, v := range s.Extra {
				merged[k] = v
			}
			s.Extra = merged
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	b, err := json.MarshalIndent(s, "", "    ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	return dataContractAtomic(path, append(b, '\n'), 0600)
}

func dataContractAdvancedFiles(a AdvancedSettings) []struct {
	name   string
	values map[string]any
} {
	return []struct {
		name   string
		values map[string]any
	}{{"enabled-mkxpz-settings.json", a.MKXPZEnabled}, {"mkxpz-advanced.json", a.MKXPZ}, {"enabled-easyrpg-settings.json", a.EasyRPGEnabled}, {"easyrpg-advanced.json", a.EasyRPG}}
}
func LoadAdvancedSettings(root string) (AdvancedSettings, error) {
	a := DefaultAdvancedSettings()
	for _, f := range dataContractAdvancedFiles(a) {
		b, err := dataContractReadRegular(filepath.Join(root, f.name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return a, err
		}
		obj, err := dataContractObject(b)
		if err != nil {
			return a, fmt.Errorf("%s: %w", f.name, err)
		}
		for k, v := range obj {
			f.values[k] = v
		}
	}
	return a, nil
}
func SaveAdvancedSettings(root string, a AdvancedSettings) error {
	// Validate all four documents before any writes. Each replacement is atomic.
	existing, err := LoadAdvancedSettings(root)
	if err != nil {
		return err
	}
	old := dataContractAdvancedFiles(existing)
	files := dataContractAdvancedFiles(a)
	encoded := make([][]byte, len(files))
	for i, f := range files {
		if f.values == nil {
			return fmt.Errorf("missing %s values", f.name)
		}
		for k, v := range f.values {
			old[i].values[k] = v
		}
		encoded[i], err = json.MarshalIndent(old[i].values, "", "    ")
		if err != nil {
			return err
		}
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	for i, f := range files {
		if err = dataContractAtomic(filepath.Join(root, f.name), append(encoded[i], '\n'), 0600); err != nil {
			return err
		}
	}
	return nil
}
func dataContractObject(b []byte) (map[string]any, error) {
	if !utf8.Valid(b) {
		return nil, errors.New("JSON is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	var obj map[string]any
	if err := decoder.Decode(&obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, errors.New("JSON must contain an object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("trailing JSON data")
	}
	return obj, nil
}
func dataContractReadRegular(path string) ([]byte, error) {
	f, err := dataContractOpenRegular(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
func dataContractAtomic(path string, b []byte, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular file: %s", path)
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".rpgm-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

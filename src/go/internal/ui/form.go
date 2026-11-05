package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"rpgm-launcher/nwjs-helper/internal/launcher"
)

// JSON travels as text: JavaScript must never parse numbers, arrays or objects.
type Field struct {
	Key         string
	Type        string
	ValueJSON   string
	EnabledJSON string
	Forced      bool
	ReadOnly    bool
	Reset       bool // Explicit default-type intent, only for known editable fields.
}

func formMaps(a launcher.AdvancedSettings, engine string) (map[string]any, map[string]any, error) {
	switch engine {
	case "MKXPZ":
		return a.MKXPZ, a.MKXPZEnabled, nil
	case "EasyRPG":
		return a.EasyRPG, a.EasyRPGEnabled, nil
	}
	return nil, nil, errors.New("詳細設定の engine が不正です")
}
func valueType(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	}
	return "unknown"
}
func raw(v any) string { b, _ := json.Marshal(v); return string(b) }
func fieldsFor(a launcher.AdvancedSettings, engine string) ([]Field, error) {
	values, enabled, err := formMaps(a, engine)
	if err != nil {
		return nil, err
	}
	_, defaults, _ := formMaps(launcher.DefaultAdvancedSettings(), engine)
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fields := make([]Field, 0, len(values))
	for _, key := range keys {
		forced := defaults[key] == "Force" || enabled[key] == "Force"
		active := enabled[key]
		if forced {
			active = "Force"
		}
		fields = append(fields, Field{Key: key, Type: valueType(values[key]), ValueJSON: raw(values[key]), EnabledJSON: raw(active), Forced: forced, ReadOnly: engine == "EasyRPG" && key == "--project-path"})
	}
	return fields, nil
}
func (u *UI) Advanced(engine string, reset bool) ([]Field, error) {
	a, err := launcher.LoadAdvancedSettings(u.manager.Root)
	if err != nil {
		return nil, err
	}
	fields, err := fieldsFor(a, engine)
	if err != nil {
		return nil, err
	}
	if reset {
		dv, de, _ := formMaps(launcher.DefaultAdvancedSettings(), engine)
		for i := range fields {
			f := &fields[i]
			if v, known := dv[f.Key]; known && !f.ReadOnly {
				f.Reset = true
				f.Type, f.ValueJSON = valueType(v), raw(v)
				if !f.Forced {
					f.EnabledJSON = raw(de[f.Key])
				}
			}
		}
	}
	return fields, nil
}
func decodeValue(text string) (any, error) {
	if len(text) > 1024*1024 {
		return nil, errors.New("詳細設定の値が大きすぎます")
	}
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("JSON の後に余分な値があります")
	}
	return v, nil
}
func (u *UI) SaveAdvanced(engine string, fields []Field) error {
	return u.operation("詳細設定保存", func(context.Context) error {
		a, err := launcher.LoadAdvancedSettings(u.manager.Root)
		if err != nil {
			return err
		}
		values, enabled, err := formMaps(a, engine)
		if err != nil {
			return err
		}
		baseline, err := fieldsFor(a, engine)
		if err != nil {
			return err
		}
		expected := map[string]Field{}
		for _, f := range baseline {
			expected[f.Key] = f
		}
		if len(fields) > 512 {
			return errors.New("詳細設定の項目が多すぎます")
		}
		defaults, _, _ := formMaps(launcher.DefaultAdvancedSettings(), engine)
		seen := map[string]bool{}
		for _, f := range fields {
			old, ok := expected[f.Key]
			if !ok || seen[f.Key] {
				return fmt.Errorf("未知または重複した項目: %s", f.Key)
			}
			seen[f.Key] = true
			if f.Reset {
				v, known := defaults[f.Key]
				if !known || old.ReadOnly {
					return fmt.Errorf("Reset できない項目: %s", f.Key)
				}
				old.Type = valueType(v)
			}
			if old.ReadOnly {
				if f.ValueJSON != old.ValueJSON || f.EnabledJSON != old.EnabledJSON {
					return errors.New("project-path はゲーム選択で管理されます")
				}
				continue
			}
			v, e := decodeValue(f.ValueJSON)
			if e != nil {
				return fmt.Errorf("%s: %w", f.Key, e)
			}
			if valueType(v) != old.Type || f.Type != old.Type {
				return fmt.Errorf("%s は %s 型です", f.Key, old.Type)
			}
			on, e := decodeValue(f.EnabledJSON)
			if e != nil {
				return e
			}
			if old.Forced {
				if on != "Force" {
					return fmt.Errorf("%s は Force 必須です", f.Key)
				}
			} else {
				switch x := on.(type) {
				case bool:
				case string:
					if x != "True" && x != "False" {
						return errors.New("Enabled は bool / True / False のみです")
					}
				case nil:
					if old.EnabledJSON != "null" {
						return errors.New("Enabled を削除できません")
					}
				default:
					return errors.New("Enabled 型が不正です")
				}
			}
			values[f.Key] = v
			if on != nil {
				enabled[f.Key] = on
			}
		}
		return launcher.SaveAdvancedSettings(u.manager.Root, a)
	})
}

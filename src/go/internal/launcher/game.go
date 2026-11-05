package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type GameEngine string

const (
	NWJS    GameEngine = "NWJS"
	MKXPZ   GameEngine = "MKXPZ"
	EasyRPG GameEngine = "EasyRPG"
)

type GameInfo struct {
	Path        string
	Engine      GameEngine
	RTP         string
	RGSSVersion int
}

// GameResult is suitable for a GUI message/warning without UI dependencies.
type GameResult struct {
	OK       bool
	Changed  bool
	Patched  bool
	JSFile   string
	Message  string
	Warnings []string
}

// DetectGame is read-only, including for malformed package.json files.
func DetectGame(folder string) (GameInfo, error) {
	g := GameInfo{Path: folder}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return g, err
	}
	if info, e := os.Stat(filepath.Join(folder, "package.json")); e == nil && info.Mode().IsRegular() {
		g.Engine = NWJS
		return g, nil
	}
	if info, e := os.Stat(filepath.Join(folder, "Game.ini")); e == nil && info.Mode().IsRegular() {
		g.Engine = MKXPZ
		g.RTP, g.RGSSVersion, err = GetRTPValue(folder)
		return g, err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "RPG_RT") {
			info, e := os.Stat(filepath.Join(folder, entry.Name()))
			if e == nil && info.Mode().IsRegular() {
				g.Engine = EasyRPG
				return g, nil
			}
		}
	}
	return g, errors.New("no package.json, Game.ini, or RPG_RT files found")
}
func GetRTPValue(folder string) (string, int, error) {
	values := []string{"", "Standard", "RPGVX", "RPGVXace"}
	raw, err := dataContractReadRegular(filepath.Join(folder, "Game.ini"))
	if err == nil {
		text, _, e := dataContractDecode(raw)
		if e != nil {
			return "", 0, e
		}
		lines := strings.Split(strings.TrimPrefix(text, "\ufeff"), "\n")
		for _, key := range []string{"rtp", "library"} {
			for _, line := range lines {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), key) {
					continue
				}
				value := strings.TrimSpace(parts[1])
				if key == "rtp" {
					for i := 1; i <= 3; i++ {
						if strings.EqualFold(value, values[i]) {
							return values[i], i, nil
						}
					}
				} else if version := dataContractRGSS(value); version != 0 {
					return values[version], version, nil
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", 0, err
	}
	for _, dir := range []string{folder, filepath.Join(folder, "System")} {
		entries, e := os.ReadDir(dir)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return "", 0, e
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "RGSS") && strings.HasSuffix(entry.Name(), ".dll") {
				if version := dataContractRGSS(entry.Name()); version != 0 {
					return values[version], version, nil
				}
			}
		}
	}
	return "Standard", 1, nil
}
func dataContractRGSS(value string) int {
	match := regexp.MustCompile(`(?i)RGSS([0-9]+)`).FindStringSubmatch(value)
	if len(match) > 1 && match[1][0] >= '1' && match[1][0] <= '3' {
		return int(match[1][0] - '0')
	}
	return 0
}

// Only UTF-8 and losslessly round-trippable CP932 are supported. Never replace bytes.
func dataContractDecode(raw []byte) (string, string, error) {
	if utf8.Valid(raw) {
		return string(raw), "UTF-8", nil
	}
	converted, err := dataContractIconv(raw, "CP932", "UTF-8")
	if err != nil {
		return "", "", fmt.Errorf("unsupported or invalid text encoding (UTF-8/CP932 required): %w", err)
	}
	back, err := dataContractIconv(converted, "UTF-8", "CP932")
	if err != nil || !bytes.Equal(raw, back) || !utf8.Valid(converted) {
		return "", "", errors.New("CP932 conversion is not lossless; original preserved")
	}
	return string(converted), "CP932", nil
}
func dataContractIconv(raw []byte, from, to string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/iconv", "-f", from, "-t", to)
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("system iconv: %w", err)
	}
	return out, nil
}
func dataContractEncode(text, encoding string) ([]byte, error) {
	if encoding == "UTF-8" {
		return []byte(text), nil
	}
	return dataContractIconv([]byte(text), "UTF-8", encoding)
}

type dataContractScript struct {
	path string
	body []byte
}

func dataContractProtectionPlan(folder, resources string) ([]dataContractScript, error) {
	var pending []dataContractScript
	for _, name := range []string{"bg.js", "disable-child.js", "disable-net.js"} {
		source, err := dataContractReadRegular(filepath.Join(resources, name))
		if err != nil {
			return nil, err
		}
		path := filepath.Join(folder, name)
		target, err := dataContractReadRegular(path)
		if err == nil {
			if !bytes.Equal(source, target) {
				return nil, fmt.Errorf("refusing to overwrite existing protection script: %s", path)
			}
		} else if errors.Is(err, os.ErrNotExist) {
			pending = append(pending, dataContractScript{path, source})
		} else {
			return nil, err
		}
	}
	return pending, nil
}
func dataContractWriteScripts(pending []dataContractScript) error {
	for _, s := range pending {
		f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		_, err = f.Write(s.body)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
func AddProtectionScripts(folder, resources string) (GameResult, error) {
	result := GameResult{}
	pending, err := dataContractProtectionPlan(folder, resources)
	if err != nil {
		result.Message = err.Error()
		return result, err
	}
	if err = dataContractWriteScripts(pending); err != nil {
		result.Message = err.Error()
		return result, err
	}
	result.OK = true
	result.Changed = len(pending) > 0
	result.Message = "Protection scripts available."
	return result, nil
}

// PreparePackage must only be called at launch/export, after validating the runtime.
// It never derives paths from the package name and never deletes a shared profile.
func PreparePackage(folder, resources string, disableProtection bool) (result GameResult, err error) {
	defer func() {
		if err != nil {
			result.Message = err.Error()
		}
	}()
	path := filepath.Join(folder, "package.json")
	raw, err := dataContractReadRegular(path)
	if err != nil {
		return result, err
	}
	text, encoding, err := dataContractDecode(raw)
	if err != nil {
		return result, err
	}
	bom := ""
	if strings.HasPrefix(text, "\ufeff") {
		bom = "\ufeff"
		text = strings.TrimPrefix(text, bom)
	}
	obj, err := dataContractObject([]byte(text))
	if err != nil {
		return result, err
	}
	changed := false
	if name, exists := obj["name"]; exists {
		s, ok := name.(string)
		if !ok {
			return result, errors.New("package.json name must be a string")
		}
		if strings.TrimSpace(s) == "" {
			obj["name"] = "tempname"
			changed = true
		}
	} else {
		obj["name"] = "tempname"
		changed = true
	}
	bg, hasBG := obj["bg-script"]
	knownBG := false
	if bg == "bg.js" {
		target, e := dataContractReadRegular(filepath.Join(folder, "bg.js"))
		if e == nil {
			source, e := dataContractReadRegular(filepath.Join(resources, "bg.js"))
			if e != nil {
				return result, e
			}
			knownBG = bytes.Equal(target, source)
		} else if !errors.Is(e, os.ErrNotExist) { // A symlink is user-owned, never an ownership proof.
			info, statErr := os.Lstat(filepath.Join(folder, "bg.js"))
			if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
				return result, e
			}
		}
	}
	var pending []dataContractScript
	if disableProtection {
		if knownBG {
			delete(obj, "bg-script")
			changed = true
		}
	} else if !hasBG || knownBG {
		pending, err = dataContractProtectionPlan(folder, resources)
		if err != nil {
			return result, err
		}
		if !hasBG {
			obj["bg-script"] = "bg.js"
			changed = true
		}
	} else {
		result.Warnings = append(result.Warnings, "The game's existing bg-script was preserved; launcher protection was not applied.")
	}
	var output []byte
	if changed {
		encoded, e := json.MarshalIndent(obj, "", "    ")
		if e != nil {
			return result, e
		}
		output, err = dataContractEncode(bom+string(encoded)+"\n", encoding)
		if err != nil {
			return result, err
		}
	}
	// No validation or encoding is deferred until after the first write.
	if err = dataContractWriteScripts(pending); err != nil {
		return result, err
	}
	if changed {
		if err = dataContractAtomic(path, output, 0644); err != nil {
			return result, err
		}
	}
	result.OK = true
	result.Changed = changed || len(pending) > 0
	result.Message = "Package ready."
	return result, nil
}

const dataContractPatchBegin = "/* RPGM-Launcher getPixel finite patch begin */"
const dataContractPatchEnd = "/* RPGM-Launcher getPixel finite patch end */"

var dataContractSignature = regexp.MustCompile(`(?m)^[\t ]*Bitmap\.prototype\.getPixel[\t ]*=[\t ]*function[\t ]*\([\t ]*x[\t ]*,[\t ]*y[\t ]*\)[\t ]*\{[\t ]*\r?$`)
var dataContractLegacy = regexp.MustCompile(`^[\t ]*if[\t ]*\([\t ]*!Number\.isFinite\(x\)[\t ]*\|\|[\t ]*!Number\.isFinite\(y\)[\t ]*\)[\t ]*\{[\t ]*\r?\n[\t ]*return[\t ]*['"]#000000['"];[\t ]*\r?\n[\t ]*\}[\t ]*\r?\n`)

func PatchGetPixel(folder string, enable bool) (result GameResult, err error) {
	defer func() {
		if err != nil {
			result.Message = err.Error()
		}
	}()
	if folder == "" {
		return result, errors.New("invalid game path")
	}
	jsDir := ""
	for _, p := range []string{filepath.Join(folder, "js"), filepath.Join(folder, "www", "js")} {
		if info, e := os.Lstat(p); e == nil && info.IsDir() {
			jsDir = p
			break
		}
	}
	if jsDir == "" {
		return result, errors.New("could not find js or www/js directory")
	}
	for _, name := range []string{"rmmz_core.js", "rpg_core.js"} {
		p := filepath.Join(jsDir, name)
		if _, e := os.Lstat(p); e == nil {
			result.JSFile = p
			break
		} else if !errors.Is(e, os.ErrNotExist) {
			return result, e
		}
	}
	if result.JSFile == "" {
		return result, errors.New("could not find rmmz_core.js or rpg_core.js")
	}
	raw, err := dataContractReadRegular(result.JSFile)
	if err != nil {
		return result, err
	}
	if !utf8.Valid(raw) {
		return result, errors.New("core JS must be UTF-8; original preserved")
	}
	text := string(raw)
	masked, markers, err := dataContractMaskJS(text)
	if err != nil {
		return result, err
	}
	match := dataContractSignature.FindStringIndex(masked)
	if match == nil {
		return result, errors.New("could not locate Bitmap.prototype.getPixel function")
	}
	open := strings.Index(masked[match[0]:match[1]], "{") + match[0]
	depth := 1
	closeIndex := -1
	for i := open + 1; i < len(masked); i++ {
		if masked[i] == '{' {
			depth++
		}
		if masked[i] == '}' {
			depth--
			if depth == 0 {
				closeIndex = i
				break
			}
		}
	}
	if closeIndex < 0 {
		return result, errors.New("unbalanced getPixel function; original preserved")
	}
	after := strings.IndexByte(text[open:], '\n')
	if after < 0 {
		return result, errors.New("getPixel signature needs a newline")
	}
	after += open + 1
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	block := "    " + dataContractPatchBegin + nl + "    if (!Number.isFinite(x) || !Number.isFinite(y)) {" + nl + "        return '#000000';" + nl + "    }" + nl + "    " + dataContractPatchEnd + nl
	body := text[after:closeIndex]
	patchEnd := 0
	if strings.HasPrefix(body, block) {
		patchEnd = len(block)
	} else if found := dataContractLegacy.FindStringIndex(body); found != nil {
		patchEnd = found[1]
	} else {
		for _, position := range markers {
			if position >= after && position < closeIndex {
				return result, errors.New("unrecognized or incomplete getPixel patch markers; original preserved")
			}
		}
	}
	result.Patched = patchEnd > 0
	result.OK = true
	if enable && patchEnd > 0 {
		result.Message = "Already patched."
		return result, nil
	}
	if !enable && patchEnd == 0 {
		result.Message = "No patch found; nothing to remove."
		return result, nil
	}
	updated := ""
	if enable {
		updated = text[:after] + block + text[after:]
		result.Message = "Patch applied."
	} else {
		updated = text[:after] + text[after+patchEnd:]
		result.Message = "Patch removed."
	}
	if err = dataContractAtomic(result.JSFile, []byte(updated), 0644); err != nil {
		result.OK = false
		return result, err
	}
	result.Changed = true
	result.Patched = enable
	return result, nil
}

// Mask literals/comments without shifting byte positions; brace matching must not
// mistake a quoted brace or a documented example for executable getPixel code.
// shortcut: this is a conservative core-JS scanner, not a parser; extend it if a core uses nested template literals.
func dataContractMaskJS(text string) (string, []int, error) {
	var markers []int
	out := []byte(text)
	last := byte(0)
	word := ""
	mask := func(start, end int) {
		for j := start; j < end; j++ {
			if out[j] != '\n' && out[j] != '\r' {
				out[j] = ' '
			}
		}
	}
	for i := 0; i < len(text); {
		c := text[i]
		if c == '\'' || c == '"' || c == '`' {
			start := i
			quote := c
			i++
			closed := false
			for i < len(text) {
				if text[i] == '\\' {
					i += 2
					continue
				}
				if text[i] == quote {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return "", nil, errors.New("unterminated JS string; original preserved")
			}
			mask(start, i)
			last = 'v'
			word = ""
			continue
		}
		if c == '/' && i+1 < len(text) && text[i+1] == '/' {
			start := i
			for i < len(text) && text[i] != '\n' {
				i++
			}
			mask(start, i)
			continue
		}
		if c == '/' && i+1 < len(text) && text[i+1] == '*' {
			start := i
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return "", nil, errors.New("unterminated JS comment; original preserved")
			}
			i = i + 2 + end + 2
			if text[start:i] == dataContractPatchBegin || text[start:i] == dataContractPatchEnd {
				markers = append(markers, start)
			}
			mask(start, i)
			continue
		}
		if c == '/' && (last == 0 || strings.ContainsRune("=(:,![{;?&|", rune(last)) || word == "return" || word == "throw") {
			start := i
			i++
			inClass := false
			closed := false
			for i < len(text) {
				if text[i] == '\\' {
					i += 2
					continue
				}
				if text[i] == '[' {
					inClass = true
				}
				if text[i] == ']' {
					inClass = false
				}
				if text[i] == '/' && !inClass {
					i++
					closed = true
					break
				}
				if text[i] == '\n' {
					break
				}
				i++
			}
			if !closed {
				return "", nil, errors.New("unrecognized JS regular expression; original preserved")
			}
			mask(start, i)
			last = 'v'
			word = ""
			continue
		}
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '$' {
			start := i
			for i < len(text) && ((text[i] >= 'a' && text[i] <= 'z') || (text[i] >= 'A' && text[i] <= 'Z') || (text[i] >= '0' && text[i] <= '9') || text[i] == '_' || text[i] == '$') {
				i++
			}
			word = text[start:i]
			last = 'v'
			continue
		}
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			last = c
			word = ""
		}
		i++
	}
	return string(out), markers, nil
}

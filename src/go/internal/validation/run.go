// Package validation is an opt-in acceptance harness for one reviewed public MZ fixture.
// HOME/profile separation is hygiene, NOT an OS sandbox: NW.js retains user privileges.
package validation

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/coder/websocket"
	"rpgm-launcher/nwjs-helper/internal/launcher"
)

type Config struct {
	Root            string
	FixtureZIP      string
	Runtime         string // Existing v0.94.1 directory, not an installer or download URL.
	ReviewedFixture bool
}

type State struct {
	Scene        string `json:"scene"`
	Map          int    `json:"map"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Direction    int    `json:"direction"`
	Switch2      bool   `json:"switch2"`
	Moving       bool   `json:"moving"`
	EventRunning bool   `json:"eventRunning"`
	Error        string `json:"error"`
}

const stateExpression = `({scene:SceneManager._scene?.constructor.name,
 map:$gameMap?.mapId(),x:$gamePlayer?.x,y:$gamePlayer?.y,
 direction:$gamePlayer?.direction(),switch2:$gameSwitches?.value(2),
 moving:$gamePlayer?.isMoving(),eventRunning:$gameMap?.isEventRunning(),
 error:document.getElementById('errorPrinter')?.innerText || ""})`
const titleReady = `typeof SceneManager !== 'undefined' && SceneManager._scene?.constructor.name === 'Scene_Title' && SceneManager._scene._commandWindow?.active && SceneManager._scene._commandWindow.isOpen() && Graphics.frameCount > 30 && SceneManager._scene._fadeDuration === 0`
const mapReady = `SceneManager._scene.constructor.name === 'Scene_Map' && $gamePlayer.canMove()`

func writeJSON(p string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0600)
}

// Run never downloads or patches anything. It requires a nonexistent absolute Root.
// Success includes source hash rechecks and shutdown of both owned process groups.
func Run(ctx context.Context, c Config) (err error) {
	if err = launcher.EnsureSupportedHost(runtime.GOOS, runtime.GOARCH); err != nil {
		return err
	}
	if !c.ReviewedFixture {
		return errors.New("--reviewed-fixture consent required; this is not a sandbox")
	}
	for _, p := range []string{c.FixtureZIP, c.Runtime} {
		if err = canonicalInput(p); err != nil {
			return err
		}
	}
	if filepath.Base(c.Runtime) != "v0.94.1" {
		return errors.New("only existing NW.js v0.94.1 is supported")
	}
	if within(c.Runtime, c.Root) || within(c.Root, c.Runtime) {
		return errors.New("root and source runtime must not overlap")
	}
	if _, err = launcher.RequireNWArm(c.Runtime); err != nil {
		return err
	}
	fixtureBefore, err := hashFile(c.FixtureZIP)
	if err != nil {
		return err
	}
	if fixtureBefore != fixtureSHA256 {
		return errors.New("fixture ZIP does not match the reviewed unacro commit/archive SHA-256")
	}
	runtimeBefore, err := treeHash(c.Runtime)
	if err != nil {
		return err
	}
	if err = createRoot(c.Root); err != nil {
		return err
	}
	evidence := filepath.Join(c.Root, "evidence")
	if err = os.Mkdir(evidence, 0700); err != nil {
		return err
	}
	started := time.Now()
	report := map[string]any{"passed": false, "started_utc": started.UTC(), "fixture_commit": fixtureCommit, "fixture_archive_sha256": fixtureBefore, "runtime_source_tree_sha256": runtimeBefore, "fixture_source": c.FixtureZIP, "runtime_source": c.Runtime, "fixture_archive_hash_basis": "locally measured reviewed archive; docs/validation/nwjs-go-migration.md, not an author-published checksum", "fixture_url": "https://github.com/unacro/rpgmaker-mz-demo/archive/" + fixtureCommit + ".zip", "isolation": "dedicated HOME/cache/TMP/user-data-dir; not an OS sandbox", "input": "CDP keyboard events only; Runtime.evaluate observes state only", "audibility": "not tested", "host": runtime.GOOS + "/" + runtime.GOARCH, "go": runtime.Version()}
	defer func() {
		fixtureAfter, e1 := hashFile(c.FixtureZIP)
		runtimeAfter, e2 := treeHash(c.Runtime)
		report["fixture_source_unchanged"] = e1 == nil && fixtureAfter == fixtureBefore
		report["runtime_source_unchanged"] = e2 == nil && runtimeAfter == runtimeBefore
		err = errors.Join(err, e1, e2)
		if fixtureAfter != fixtureBefore || runtimeAfter != runtimeBefore {
			err = errors.Join(err, errors.New("original fixture/runtime hash changed"))
		}
		report["elapsed_seconds"] = time.Since(started).Seconds()
		report["passed"] = err == nil
		if err != nil {
			report["error"] = err.Error()
		}
		err = errors.Join(err, writeJSON(filepath.Join(evidence, "result.json"), report))
	}()
	if err = writeJSON(filepath.Join(evidence, "provenance.json"), report); err != nil {
		return err
	}
	for _, dir := range []string{"home", "cache", "tmp", "profile", "runtime", "game"} {
		if err = os.Mkdir(filepath.Join(c.Root, dir), 0700); err != nil {
			return err
		}
	}
	copiedRuntime := filepath.Join(c.Root, "runtime", "v0.94.1")
	if err = copyRuntime(ctx, c.Runtime, copiedRuntime); err != nil {
		return err
	}
	binary, err := launcher.RequireNWArm(copiedRuntime)
	if err != nil {
		return err
	}
	signatureCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	signature := exec.CommandContext(signatureCtx, "/usr/bin/codesign", "--verify", "--deep", "--strict", "--verbose=2", filepath.Join(copiedRuntime, "nwjs.app"))
	signature.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	output, signatureErr := signature.CombinedOutput()
	cancel()
	if err = os.WriteFile(filepath.Join(evidence, "codesign.log"), output, 0600); err != nil {
		return err
	}
	report["signature_verified"] = signatureErr == nil
	if signatureErr != nil {
		report["signature_limitation"] = string(output)
		var exit *exec.ExitError
		if !errors.As(signatureErr, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(output), "code has no resources but signature indicates they must be present") {
			return fmt.Errorf("unexpected signature verification failure: %w: %s", signatureErr, output)
		}
	}
	ownedArchive := filepath.Join(c.Root, "fixture.zip")
	if err = copyPinnedArchive(c.FixtureZIP, ownedArchive, fixtureSHA256); err != nil {
		return err
	}
	if err = extractFixture(ctx, ownedArchive, filepath.Join(c.Root, "game"), fixtureDirectory); err != nil {
		return err
	}
	gamePath := filepath.Join(c.Root, "game", fixtureDirectory)
	if err = verifyFixtureCopy(ownedArchive, gamePath); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, verifyFixtureCopy(ownedArchive, gamePath)) }()
	var saved, loaded State
	for run := 1; run <= 2; run++ {
		err = func() (runErr error) {
			g := &game{root: c.Root, evidence: evidence, path: gamePath, run: run}
			defer func() { runErr = errors.Join(runErr, g.close()) }()
			if runErr = g.start(ctx, binary); runErr != nil {
				return runErr
			}
			if run == 1 {
				saved, runErr = g.first(ctx)
			} else {
				loaded, runErr = g.second(ctx)
			}
			return runErr
		}()
		if err != nil {
			return err
		}
	}
	if !restored(saved) || !restored(loaded) {
		return fmt.Errorf("unexpected saved/loaded state: saved=%+v loaded=%+v", saved, loaded)
	}
	report["saved"] = saved
	report["loaded"] = loaded
	report["fixture_original_files_byte_identical"] = 1332
	return nil
}

func verifyFixtureCopy(archive, game string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := strings.TrimPrefix(f.Name, fixtureDirectory+"/")
		p := filepath.Join(game, filepath.FromSlash(rel))
		actual, err := hashFile(p)
		if err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		// Compare CRC-checked ZIP bytes without loading game assets into memory.
		expected, e := hashReader(in)
		err = errors.Join(e, in.Close())
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("original fixture file changed in copy: %s", rel)
		}
	}
	return nil
}

type game struct {
	root, evidence, path string
	run                  int
	process              *ownedProcess
	cdp                  *cdp
	output, protocol     *os.File
}

func (g *game) file(name string) string {
	return filepath.Join(g.evidence, fmt.Sprintf("%s-%d", name, g.run))
}

func (g *game) start(ctx context.Context, binary string) error {
	var err error
	g.output, err = os.OpenFile(g.file("nwjs")+".log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	g.protocol, err = os.OpenFile(g.file("cdp")+".jsonl", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	command := exec.Command(binary, "--user-data-dir="+filepath.Join(g.root, "profile"), "--remote-debugging-address=127.0.0.1", fmt.Sprintf("--remote-debugging-port=%d", port), g.path)
	command.Dir = g.path
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + filepath.Join(g.root, "home"), "TMPDIR=" + filepath.Join(g.root, "tmp"), "XDG_CACHE_HOME=" + filepath.Join(g.root, "cache")}
	command.Stdout = g.output
	command.Stderr = g.output
	g.process, err = startOwned(command, port)
	if err != nil {
		return err
	}
	if err = writeJSON(g.file("launch")+".json", map[string]any{"command": command.Args, "env": command.Env, "pid": command.Process.Pid, "pgid": command.Process.Pid, "debug_port": port}); err != nil {
		return err
	}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/json/list", port)
	if err = checkDebugURL(endpoint, "http", port); err != nil {
		return err
	}
	client := debugHTTPClient()
	defer client.CloseIdleConnections()
	var wsURL string
	for wsURL == "" {
		select {
		case exit := <-g.process.done:
			return fmt.Errorf("NW.js exited before title: %v", exit)
		default:
		}
		if !portClosed(port) {
			listenerLog, e := os.OpenFile(g.file("listeners")+".txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if e != nil {
				return e
			}
			e = verifyListener(startup, g.process, listenerLog)
			closeErr := listenerLog.Close()
			if e != nil || closeErr != nil {
				return errors.Join(e, closeErr)
			}
			wsURL, err = discoverTarget(startup, client, endpoint, g.path, g.protocol)
			if err != nil {
				return err
			}
		}
		if wsURL == "" {
			if err = pause(startup, 150*time.Millisecond); err != nil {
				return fmt.Errorf("no real game renderer (GUI unavailable or startup timed out): %w", err)
			}
		}
	}
	if err = checkDebugURL(wsURL, "ws", port); err != nil {
		return err
	}
	conn, _, err := websocket.Dial(startup, wsURL, &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		return err
	}
	conn.SetReadLimit(screenshotReadLimit)
	g.cdp = &cdp{conn: conn, log: g.protocol}
	for _, method := range []string{"Runtime.enable", "Log.enable", "Network.enable", "Page.bringToFront"} {
		if _, err = g.cdp.call(ctx, method, map[string]any{}); err != nil {
			return err
		}
	}
	if err = g.cdp.wait(ctx, titleReady, 30*time.Second); err != nil {
		return err
	}
	var versions struct {
		Engine        string            `json:"engine"`
		EngineVersion string            `json:"engineVersion"`
		Arch          string            `json:"arch"`
		Versions      map[string]string `json:"versions"`
		SavePath      string            `json:"savePath"`
		Plugins       []string          `json:"plugins"`
		WebGL         int               `json:"webgl"`
		AudioContext  string            `json:"audioContext"`
	}
	if err = g.cdp.evaluate(ctx, `({engine:Utils.RPGMAKER_NAME,engineVersion:Utils.RPGMAKER_VERSION,arch:process.arch,versions:process.versions,savePath:StorageManager.fileDirectoryPath(),plugins:$plugins.filter(p=>p.status).map(p=>p.name),webgl:Graphics.app.renderer.type,audioContext:WebAudio._context?.state})`, &versions); err != nil {
		return err
	}
	if err = writeJSON(g.file("versions")+".json", versions); err != nil {
		return err
	}
	if versions.Engine != "MZ" || versions.EngineVersion != "1.8.0" || versions.Arch != "arm64" || versions.Versions["nw"] != "0.94.1" || filepath.Clean(versions.SavePath) != filepath.Join(g.path, "save") {
		return fmt.Errorf("unexpected live runtime or save directory: %+v", versions)
	}
	return nil
}

func discoverTarget(ctx context.Context, client *http.Client, endpoint, gamePath string, log io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("debug discovery status %s", response.Status)
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return "", err
	}
	if len(b) > 1<<20 {
		return "", errors.New("oversized debug discovery")
	}
	var targets []struct {
		Type      string `json:"type"`
		URL       string `json:"url"`
		WebSocket string `json:"webSocketDebuggerUrl"`
	}
	if err = json.Unmarshal(b, &targets); err != nil {
		return "", err
	}
	if err = json.NewEncoder(log).Encode(map[string]any{"discovery_url": endpoint, "targets": json.RawMessage(b)}); err != nil {
		return "", err
	}
	found := ""
	for _, target := range targets {
		u, e := url.Parse(target.URL)
		// NW.js exposes this reviewed package under its stable extension origin (past live evidence).
		localFile := e == nil && u.Scheme == "file" && u.Host == "" && u.Path == filepath.Join(gamePath, "index.html") && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
		reviewedOrigin := target.URL == "chrome-extension://njgcanhfjdabfmnlmpmdedalocpafnhl/index.html"
		if target.Type == "page" && (localFile || reviewedOrigin) {
			if found != "" {
				return "", errors.New("multiple game renderer targets")
			}
			found = target.WebSocket
		}
	}
	return found, nil
}

func (g *game) close() error {
	var err error
	if g.cdp != nil {
		err = g.cdp.conn.CloseNow()
	}
	if g.process != nil {
		cleanup := g.process.close()
		err = errors.Join(err, cleanup)
		err = errors.Join(err, writeJSON(g.file("cleanup")+".json", map[string]any{"pid": g.process.cmd.Process.Pid, "debug_port": g.process.port, "waited": g.process.waited, "group_gone": groupGone(g.process.cmd.Process.Pid), "port_closed": portClosed(g.process.port), "success": cleanup == nil}))
	}
	if g.output != nil {
		err = errors.Join(err, g.output.Close())
	}
	if g.protocol != nil {
		err = errors.Join(err, g.protocol.Close())
	}
	return err
}

func (g *game) capture(ctx context.Context, name string) (State, error) {
	var state State
	if err := pause(ctx, 700*time.Millisecond); err != nil {
		return state, err
	}
	if err := g.cdp.evaluate(ctx, stateExpression, &state); err != nil {
		return state, err
	}
	if state.Error != "" {
		return state, fmt.Errorf("engine error overlay: %s", state.Error)
	}
	if err := writeJSON(filepath.Join(g.evidence, name+".json"), state); err != nil {
		return state, err
	}
	b, err := g.cdp.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png"})
	if err != nil {
		return state, err
	}
	var result struct {
		Data string `json:"data"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		return state, err
	}
	image, err := screenshotPNG(result.Data)
	if err != nil {
		return state, err
	}
	return state, os.WriteFile(filepath.Join(g.evidence, name+".png"), image, 0600)
}

func (g *game) expect(ctx context.Context, expression string) error {
	var ok bool
	if err := g.cdp.evaluate(ctx, expression, &ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unexpected UI state: %s", expression)
	}
	return nil
}
func restored(s State) bool {
	return s.Scene == "Scene_Map" && s.Map == 1 && s.X == 7 && s.Y == 6 && s.Direction == 8 && s.Switch2 && !s.Moving && !s.EventRunning && s.Error == ""
}

func (g *game) first(ctx context.Context) (saved State, err error) {
	if _, err = g.capture(ctx, "01-title"); err != nil {
		return
	}
	if err = g.expect(ctx, `SceneManager._scene._commandWindow.currentSymbol() === 'newGame'`); err != nil {
		return
	}
	if err = g.cdp.key(ctx, "Enter", 13); err != nil {
		return
	}
	if err = g.cdp.wait(ctx, mapReady, 30*time.Second); err != nil {
		return
	}
	initial, err := g.capture(ctx, "02-new-game")
	if err != nil {
		return saved, err
	}
	if initial.Map != 1 || initial.X != 8 || initial.Y != 7 || initial.Switch2 {
		return saved, fmt.Errorf("unexpected new game: %+v", initial)
	}
	if err = g.cdp.key(ctx, "ArrowLeft", 37); err != nil {
		return
	}
	if err = g.cdp.key(ctx, "ArrowUp", 38); err != nil {
		return
	}
	if err = g.cdp.wait(ctx, `!$gamePlayer.isMoving()`, 30*time.Second); err != nil {
		return
	}
	moved, err := g.capture(ctx, "03-movement")
	if err != nil {
		return saved, err
	}
	if moved.X != 7 || moved.Y != 6 || moved.Direction != 8 || moved.Switch2 {
		return saved, fmt.Errorf("unexpected movement: %+v", moved)
	}
	if err = g.cdp.key(ctx, "Enter", 13); err != nil {
		return
	}
	if err = g.cdp.wait(ctx, `$gameSwitches.value(2) && !$gameMap.isEventRunning()`, 30*time.Second); err != nil {
		return
	}
	saved, err = g.capture(ctx, "04-switch-event")
	if err != nil {
		return
	}
	if !restored(saved) {
		return saved, fmt.Errorf("unexpected event state: %+v", saved)
	}
	if err = g.cdp.key(ctx, "Escape", 27); err != nil {
		return
	}
	if err = g.cdp.wait(ctx, `SceneManager._scene.constructor.name === 'Scene_Menu' && SceneManager._scene._commandWindow.active`, 30*time.Second); err != nil {
		return
	}
	for i := 0; i < 10; i++ {
		var selected bool
		if err = g.cdp.evaluate(ctx, `SceneManager._scene._commandWindow.currentSymbol() === 'save'`, &selected); err != nil {
			return
		}
		if selected {
			break
		}
		if err = g.cdp.key(ctx, "ArrowDown", 40); err != nil {
			return
		}
	}
	if err = g.expect(ctx, `SceneManager._scene._commandWindow.currentSymbol() === 'save'`); err != nil {
		return
	}
	if err = g.cdp.key(ctx, "Enter", 13); err != nil {
		return
	}
	if err = g.cdp.wait(ctx, `SceneManager._scene.constructor.name === 'Scene_Save' && SceneManager._scene._listWindow.active`, 30*time.Second); err != nil {
		return
	}
	if _, err = g.capture(ctx, "05-save-menu"); err != nil {
		return
	}
	if err = g.expect(ctx, `SceneManager._scene.savefileId() === 1`); err != nil {
		return
	}
	if err = g.cdp.key(ctx, "Enter", 13); err != nil {
		return
	}
	if err = g.cdp.wait(ctx, `SceneManager._scene.constructor.name === 'Scene_Menu'`, 30*time.Second); err != nil {
		return
	}
	info, err := os.Lstat(filepath.Join(g.path, "save", "file1.rmmzsave"))
	if err != nil {
		return saved, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return saved, errors.New("menu Save did not write slot 1")
	}
	_, err = g.capture(ctx, "06-saved")
	return
}

func (g *game) second(ctx context.Context) (loaded State, err error) {
	if _, err = g.capture(ctx, "07-relaunch-title"); err != nil {
		return
	}
	if err = g.expect(ctx, `SceneManager._scene._commandWindow.currentSymbol() === 'continue'`); err != nil {
		return
	}
	if err = g.cdp.key(ctx, "Enter", 13); err != nil {
		return
	}
	if err = g.cdp.wait(ctx, `SceneManager._scene.constructor.name === 'Scene_Load' && SceneManager._scene._listWindow.active`, 30*time.Second); err != nil {
		return
	}
	if _, err = g.capture(ctx, "08-load-menu"); err != nil {
		return
	}
	if err = g.expect(ctx, `SceneManager._scene.savefileId() === 1`); err != nil {
		return
	}
	if err = g.cdp.key(ctx, "Enter", 13); err != nil {
		return
	}
	if err = g.cdp.wait(ctx, mapReady, 30*time.Second); err != nil {
		return
	}
	return g.capture(ctx, "09-loaded")
}

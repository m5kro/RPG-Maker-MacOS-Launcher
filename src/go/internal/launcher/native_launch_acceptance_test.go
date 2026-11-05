//go:build nativeacceptance

package launcher

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type nativeLaunchState struct {
	PID             int
	ContextCanceled bool
	WaitCanceled    bool
	LockRefused     bool
	Warnings        []string
}

// This is a genuine separate Go caller, with the production command factory.
// Returning normally exits the caller without waiting for or killing the game.
func TestNativeLaunchCaller(t *testing.T) {
	if os.Getenv("RPGM_NATIVE_CALLER") != "1" {
		return
	}
	root := os.Getenv("RPGM_NATIVE_WORK")
	if !filepath.IsAbs(root) || !strings.Contains(root, "/build/validation/python-zero/native-backend/TestNativeManagerLaunch-") {
		t.Fatal("invalid owned worker root")
	}
	m, err := NewManager(filepath.Join(root, "managed"), "", os.Stdout)
	nativeMust(t, err)
	game, err := DetectGame(filepath.Join(root, "game", nativeFixtureTop))
	nativeMust(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	p, err := m.Launch(ctx, game, Settings{LastSelectedVersion: "v0.94.1", DisableProtection: true})
	nativeMust(t, err)
	// Publish ownership before any later assertion, so the parent can always clean up.
	state := nativeLaunchState{PID: p.PID, Warnings: p.Warnings}
	nativeJSON(t, filepath.Join(root, "pid.json"), state)
	cancel()
	time.Sleep(500 * time.Millisecond)
	state.ContextCanceled = syscall.Kill(p.PID, 0) == nil
	_, err = p.Wait(ctx)
	state.WaitCanceled = errors.Is(err, context.Canceled)
	release, err := AcquireManagedLock(m.Root)
	state.LockRefused = err != nil && errors.Is(err, syscall.EWOULDBLOCK)
	if release != nil {
		release()
	}
	nativeJSON(t, filepath.Join(root, "caller-state.json"), state)
	// The parent closes stdin only after observing the rendered title with this caller alive.
	gate := make(chan struct{})
	go func() { _, _ = bufio.NewReader(os.Stdin).ReadString('\n'); close(gate) }()
	select {
	case <-gate:
	case <-time.After(90 * time.Second):
		_ = p.Signal(syscall.SIGTERM)
		t.Fatal("parent did not release caller")
	}
}

func nativeAlive(pid int) bool {
	b, err := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "stat=").Output()
	return err == nil && strings.TrimSpace(string(b)) != "" && !strings.HasPrefix(strings.TrimSpace(string(b)), "Z")
}
func nativeReadState(p string) (nativeLaunchState, error) {
	var s nativeLaunchState
	b, e := os.ReadFile(p)
	if e == nil {
		e = json.Unmarshal(b, &s)
	}
	return s, e
}
func nativeWait(d time.Duration, f func() bool) bool {
	until := time.Now().Add(d)
	for time.Now().Before(until) {
		if f() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return f()
}
func nativePortClosed(port int) bool {
	c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
	if e != nil {
		return true
	}
	c.Close()
	return false
}

// Track only members of the new session/group created by Manager.Launch.
func nativeGroup(pgid int) []int {
	b, e := exec.Command("/bin/ps", "-axo", "pid=,pgid=,stat=").Output()
	if e != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || strings.HasPrefix(f[2], "Z") {
			continue
		}
		p, e := strconv.Atoi(f[0])
		g, e2 := strconv.Atoi(f[1])
		if e == nil && e2 == nil && g == pgid {
			pids = append(pids, p)
		}
	}
	return pids
}
func nativeCleanup(t *testing.T, root string, pid, port int) {
	t.Helper()
	if pid <= 1 {
		t.Error("no game ownership PID available for cleanup")
		return
	}
	report := map[string]any{"owned_pgid": pid, "before": nativeGroup(pid)}
	var sent []string
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		if len(nativeGroup(pid)) == 0 {
			break
		}
		// Recheck executable paths before group signals; refuse recycled/unrelated PIDs.
		for _, member := range nativeGroup(pid) {
			command, e := exec.Command("/bin/ps", "-p", strconv.Itoa(member), "-o", "command=").Output()
			if e != nil && !nativeAlive(member) {
				continue
			}
			if e != nil || !strings.HasPrefix(strings.TrimSpace(string(command)), filepath.Join(root, "managed/v0.94.1/nwjs.app")+"/") {
				t.Errorf("refusing cleanup of unrecognized group member %d: %s (%v)", member, command, e)
				return
			}
		}
		if nativeAlive(pid) {
			g, e := syscall.Getpgid(pid)
			if e != nil || g != pid {
				t.Errorf("ownership changed: pid=%d pgid=%d err=%v", pid, g, e)
				return
			}
		}
		err := syscall.Kill(-pid, sig)
		sent = append(sent, fmt.Sprintf("owned PGID %d signal=%s error=%v", pid, sig, err))
		if nativeWait(8*time.Second, func() bool { return len(nativeGroup(pid)) == 0 }) {
			break
		}
	}
	stopped := nativeWait(5*time.Second, func() bool { return !nativeAlive(pid) && len(nativeGroup(pid)) == 0 && nativePortClosed(port) })
	report["signals"] = sent
	report["pid_dead_group_empty_port_closed"] = stopped
	report["after"] = nativeGroup(pid)
	release, err := AcquireManagedLock(filepath.Join(root, "managed"))
	report["lock_released"] = err == nil
	report["lock_error"] = fmt.Sprint(err)
	if release != nil {
		release()
	}
	nativeJSON(t, filepath.Join(root, "cleanup.json"), report)
	if !stopped || err != nil {
		t.Errorf("cleanup incomplete: %v", report)
	}
}

type nativeCDP struct {
	conn   *websocket.Conn
	serial int
}

func (c *nativeCDP) call(method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c.serial++
	b, e := json.Marshal(map[string]any{"id": c.serial, "method": method, "params": params})
	if e != nil {
		return nil, e
	}
	if e = c.conn.Write(ctx, websocket.MessageText, b); e != nil {
		return nil, e
	}
	for {
		_, b, e = c.conn.Read(ctx)
		if e != nil {
			return nil, e
		}
		var v struct {
			ID     int
			Result json.RawMessage
			Error  json.RawMessage
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		if v.ID == c.serial {
			if len(v.Error) != 0 {
				return nil, fmt.Errorf("CDP: %s", v.Error)
			}
			return v.Result, nil
		}
	}
}
func nativeConnect(port int, game string) (*nativeCDP, error) {
	address := "127.0.0.1:" + strconv.Itoa(port)
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("no redirects") }}
	defer client.CloseIdleConnections()
	resp, e := client.Get("http://" + address + "/json/list")
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	var pages []struct {
		URL       string `json:"url"`
		WebSocket string `json:"webSocketDebuggerUrl"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&pages); e != nil {
		return nil, e
	}
	for _, p := range pages {
		u, e := url.Parse(p.URL)
		localFile := e == nil && u.Scheme == "file" && u.Host == "" && u.Path == filepath.Join(game, "index.html") && u.RawQuery == "" && u.Fragment == ""
		reviewedOrigin := p.URL == "chrome-extension://njgcanhfjdabfmnlmpmdedalocpafnhl/index.html"
		if !localFile && !reviewedOrigin {
			continue
		}
		ws, e := url.Parse(p.WebSocket)
		if e != nil || ws.Scheme != "ws" || ws.Host != address || ws.User != nil || !strings.HasPrefix(ws.Path, "/devtools/page/") {
			return nil, errors.New("unsafe CDP URL")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		conn, _, e := websocket.Dial(ctx, p.WebSocket, &websocket.DialOptions{HTTPClient: client})
		if e != nil {
			return nil, e
		}
		conn.SetReadLimit(16 << 20)
		return &nativeCDP{conn: conn}, nil
	}
	return nil, errors.New("owned game page not found")
}
func nativeTitle(c *nativeCDP) (json.RawMessage, bool, error) {
	b, e := c.call("Runtime.evaluate", map[string]any{"expression": `({scene:typeof SceneManager==='undefined'?'':SceneManager._scene?.constructor.name,ready:typeof SceneManager!=='undefined'&&SceneManager._scene?.constructor.name==='Scene_Title'&&SceneManager._scene._commandWindow?.active&&Graphics.frameCount>30&&SceneManager._scene._fadeDuration===0,title:document.title,href:location.href,arch:process.arch,nw:process.versions.nw,execPath:process.execPath,cwd:process.cwd(),pid:process.pid,error:document.getElementById('errorPrinter')?.innerText||''})`, "returnByValue": true})
	if e != nil {
		return b, false, e
	}
	var v struct {
		Result struct {
			Value struct {
				Ready bool
				Arch  string
				NW    string
				Error string
			}
		}
		ExceptionDetails json.RawMessage
	}
	e = json.Unmarshal(b, &v)
	return b, e == nil && len(v.ExceptionDetails) == 0 && v.Result.Value.Ready && v.Result.Value.Arch == "arm64" && v.Result.Value.NW == "0.94.1" && v.Result.Value.Error == "", e
}

func TestNativeManagerLaunch(t *testing.T) {
	repo, root := nativeRoot(t)
	fixture := filepath.Join(repo, "build/validation/mz-20260717/fixture.zip")
	runtimeSource := filepath.Join(repo, "build/validation/mz-20260717/runtime/v0.94.1")
	if nativeHash(t, fixture) != nativeFixtureHash {
		t.Fatal("reviewed public fixture ZIP hash mismatch")
	}
	runtimeBefore := nativeTree(t, runtimeSource)
	report := map[string]any{"fixture_sha256": nativeFixtureHash, "fixture_source": fixture, "runtime_source": runtimeSource, "runtime_tree_before": runtimeBefore, "tree_hash_basis": "sorted relative path, mode, file SHA256 or symlink target; not xattrs", "passed": false}
	defer func() {
		report["fixture_unchanged"] = nativeHash(t, fixture) == nativeFixtureHash
		report["runtime_tree_after"] = nativeTree(t, runtimeSource)
		report["runtime_unchanged"] = report["runtime_tree_after"] == runtimeBefore
		if report["fixture_unchanged"] != true || report["runtime_unchanged"] != true {
			t.Error("original inputs changed")
		}
		report["passed"] = !t.Failed()
		nativeJSON(t, filepath.Join(root, "result.json"), report)
	}()
	for _, d := range []string{"home", "cache", "tmp", "profile", "managed", "game"} {
		nativeMust(t, os.Mkdir(filepath.Join(root, d), 0700))
	}
	// ditto copies preserve official bundle links and signatures; no resigning or xattr edits.
	nativeCommand(t, "/usr/bin/ditto", runtimeSource, filepath.Join(root, "managed/v0.94.1"))
	if nativeTree(t, filepath.Join(root, "managed/v0.94.1")) != runtimeBefore {
		t.Fatal("runtime copy differs")
	}
	signature, sigErr := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", "--verbose=2", filepath.Join(root, "managed/v0.94.1/nwjs.app")).CombinedOutput()
	report["signature_verified"] = sigErr == nil
	report["signature_error"] = fmt.Sprint(sigErr)
	nativeMust(t, os.WriteFile(filepath.Join(root, "codesign.log"), signature, 0600))
	copiedZip := filepath.Join(root, "fixture.zip")
	nativeCommand(t, "/bin/cp", fixture, copiedZip)
	if nativeHash(t, copiedZip) != nativeFixtureHash {
		t.Fatal("copied archive hash mismatch")
	}
	nativeCommand(t, "/usr/bin/ditto", "-x", "-k", copiedZip, filepath.Join(root, "game"))
	game := filepath.Join(root, "game", nativeFixtureTop)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	nativeMust(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	nativeMust(t, listener.Close())
	report["port"] = port
	profile := filepath.Join(root, "profile")
	if strings.ContainsAny(profile, " \t\n\r'\"\\") {
		t.Fatal("profile path must not need shell quoting")
	}
	packagePath := filepath.Join(game, "package.json")
	raw, err := os.ReadFile(packagePath)
	nativeMust(t, err)
	var pkg map[string]any
	nativeMust(t, json.Unmarshal(raw, &pkg))
	pkg["chromium-args"] = fmt.Sprintf("--force-color-profile=srgb --remote-debugging-address=127.0.0.1 --remote-debugging-port=%d --user-data-dir=%s", port, profile)
	nativeJSON(t, packagePath, pkg)
	command := exec.Command(os.Args[0], "-test.run=^TestNativeLaunchCaller$", "-test.v")
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "RPGM_NATIVE_CALLER=1", "RPGM_NATIVE_WORK=" + root, "HOME=" + filepath.Join(root, "home"), "CFFIXED_USER_HOME=" + filepath.Join(root, "home"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "TMPDIR=" + filepath.Join(root, "tmp")}
	input, err := command.StdinPipe()
	nativeMust(t, err)
	log, err := os.OpenFile(filepath.Join(root, "caller.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	nativeMust(t, err)
	defer log.Close()
	command.Stdout = log
	command.Stderr = log
	nativeMust(t, command.Start())
	report["caller_pid"] = command.Process.Pid
	report["caller_environment"] = command.Env
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	callerDone := false
	defer func() {
		input.Close()
		if !callerDone {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = command.Process.Kill()
				<-done
			}
		}
		s, e := nativeReadState(filepath.Join(root, "pid.json"))
		if e == nil {
			nativeCleanup(t, root, s.PID, port)
		} else {
			t.Errorf("caller never published PID: %v", e)
		}
	}()
	var state nativeLaunchState
	if !nativeWait(20*time.Second, func() bool {
		var e error
		state, e = nativeReadState(filepath.Join(root, "caller-state.json"))
		return e == nil
	}) {
		t.Fatal("caller did not publish launch state; inspect caller.log/game.log")
	}
	report["caller_state"] = state
	if !state.ContextCanceled || !state.WaitCanceled || !state.LockRefused {
		t.Error("context or held-caller lock contract failed", state)
	}
	pgid, err := syscall.Getpgid(state.PID)
	nativeMust(t, err)
	report["game_pgid"] = pgid
	processBefore := nativeCommand(t, "/bin/ps", "-p", fmt.Sprintf("%d,%d", state.PID, command.Process.Pid), "-o", "pid=,ppid=,pgid=,sess=,command=")
	nativeMust(t, os.WriteFile(filepath.Join(root, "process-caller-alive.txt"), processBefore, 0600))
	if pgid != state.PID {
		t.Fatal("game did not establish its own process group")
	}
	var c *nativeCDP
	if !nativeWait(25*time.Second, func() bool { c, err = nativeConnect(port, game); return err == nil }) {
		t.Fatalf("no owned CDP target: %v", err)
	}
	defer c.conn.CloseNow()
	// MZ pauses in the background; foreground the owned page as a user would.
	_, err = c.call("Page.bringToFront", nil)
	nativeMust(t, err)
	var title json.RawMessage
	if !nativeWait(25*time.Second, func() bool { var ready bool; title, ready, err = nativeTitle(c); return err == nil && ready }) {
		nativeJSON(t, filepath.Join(root, "failed-title.json"), title)
		t.Fatalf("ARM64 NW 0.94.1 title did not become ready: %v", err)
	}
	nativeJSON(t, filepath.Join(root, "title-caller-alive.json"), title)
	var identity struct {
		Result struct {
			Value struct {
				CWD      string
				ExecPath string
			}
		}
	}
	nativeMust(t, json.Unmarshal(title, &identity))
	if identity.Result.Value.CWD != game || !strings.HasPrefix(identity.Result.Value.ExecPath, filepath.Join(root, "managed")+"/") {
		t.Fatal("live renderer identity differs from copied game/runtime", identity)
	}
	image, err := c.call("Page.captureScreenshot", map[string]any{"format": "png"})
	nativeMust(t, err)
	var screenshot struct{ Data string }
	nativeMust(t, json.Unmarshal(image, &screenshot))
	png, err := base64.StdEncoding.DecodeString(screenshot.Data)
	nativeMust(t, err)
	nativeMust(t, os.WriteFile(filepath.Join(root, "title.png"), png, 0600))
	fds, _ := exec.Command("/usr/sbin/lsof", "-nP", "-p", strconv.Itoa(state.PID)).CombinedOutput()
	nativeMust(t, os.WriteFile(filepath.Join(root, "game-fds-caller-alive.txt"), fds, 0600))
	sockets, err := exec.Command("/usr/sbin/lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN").CombinedOutput()
	nativeMust(t, err)
	nativeMust(t, os.WriteFile(filepath.Join(root, "listeners.txt"), sockets, 0600))
	if !strings.Contains(string(sockets), "127.0.0.1:"+strconv.Itoa(port)) || strings.Contains(string(sockets), "*:"+strconv.Itoa(port)) {
		t.Fatal("debug listener not restricted to loopback")
	}
	release, err := AcquireManagedLock(filepath.Join(root, "managed"))
	heldRefused := errors.Is(err, syscall.EWOULDBLOCK)
	if release != nil {
		release()
	}
	report["parent_probe_caller_alive_lock_refused"] = heldRefused
	if !heldRefused {
		t.Error("caller-alive lock not refused", err)
	}
	nativeMust(t, input.Close())
	select {
	case err = <-done:
		callerDone = true
		nativeMust(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("caller failed to exit normally")
	}
	report["caller_normal_exit"] = true
	time.Sleep(time.Second)
	alive := nativeAlive(state.PID)
	report["game_alive_after_caller_exit"] = alive
	if !alive {
		t.Error("game died after caller exit")
	}
	processAfter := nativeCommand(t, "/bin/ps", "-p", strconv.Itoa(state.PID), "-o", "pid=,ppid=,pgid=,sess=,command=")
	nativeMust(t, os.WriteFile(filepath.Join(root, "process-after-caller-exit.txt"), processAfter, 0600))
	title, ready, err := nativeTitle(c)
	nativeMust(t, err)
	nativeJSON(t, filepath.Join(root, "title-after-caller-exit.json"), title)
	if !ready {
		t.Error("title not alive after caller exit")
	}
	fds, _ = exec.Command("/usr/sbin/lsof", "-nP", "-p", strconv.Itoa(state.PID)).CombinedOutput()
	nativeMust(t, os.WriteFile(filepath.Join(root, "game-fds-after-caller-exit.txt"), fds, 0600))
	release, err = AcquireManagedLock(filepath.Join(root, "managed"))
	refused := errors.Is(err, syscall.EWOULDBLOCK)
	if release != nil {
		release()
	}
	report["caller_exited_game_alive_lock_refused"] = refused
	report["post_caller_lock_error"] = fmt.Sprint(err)
	if !refused {
		t.Errorf("REAL NW.js lock contract failed: game PID %d alive=%v but AcquireManagedLock returned %v after normal Go caller exit", state.PID, alive, err)
	}
}

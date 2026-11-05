#!/usr/bin/env python3
"""Opt-in real renderer check for the reviewed unacro MZ fixture; see docs/validation.

Requires websocket-client in a disposable venv. Does not download, patch, sign,
install, or delete anything. Saves slot 1 in a dedicated fixture copy only.
"""
import argparse
import base64
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import time
import urllib.request

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT / "src/python"))
from runtime_arch import ensure_supported_host, require_nwjs_arm64

import websocket

COMMIT = "81abd876e3c339fbd7d74323ad8817ad7011cade"
STATE = """({scene:SceneManager._scene?.constructor.name,
 map:$gameMap?.mapId(),x:$gamePlayer?.x,y:$gamePlayer?.y,
 direction:$gamePlayer?.direction(),switch2:$gameSwitches?.value(2),
 moving:$gamePlayer?.isMoving(),eventRunning:$gameMap?.isEventRunning(),
 error:document.getElementById('errorPrinter')?.innerText || null})"""


class Game:
    def __init__(self, root, evidence, run):
        self.root, self.evidence, self.run = root, evidence, run
        self.process = self.ws = None
        self.serial = 0
        self.log = (evidence / f"cdp-{run}.jsonl").open("w")

    def call(self, method, **params):
        self.serial += 1
        request = dict(id=self.serial, method=method, params=params)
        self.log.write(json.dumps(request) + "\n")
        self.log.flush()
        self.ws.send(json.dumps(request))
        while True:
            result = json.loads(self.ws.recv())
            if result.get("id") == self.serial:
                assert "error" not in result, result
                return result.get("result", {})
            if result.get("method") in ("Runtime.exceptionThrown", "Log.entryAdded", "Network.loadingFailed"):
                self.log.write(json.dumps(result) + "\n")

    def evaluate(self, expression):
        result = self.call("Runtime.evaluate", expression=expression, returnByValue=True, awaitPromise=True)
        assert "exceptionDetails" not in result, result
        return result["result"].get("value")

    def wait(self, expression, timeout=30):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if self.evaluate(expression):
                return
            time.sleep(0.1)
        raise AssertionError(f"Timed out: {expression}; state={self.evaluate(STATE)}")

    def key(self, key, code, hold=0.07):
        self.call("Input.dispatchKeyEvent", type="keyDown", key=key, code=key,
                  windowsVirtualKeyCode=code, nativeVirtualKeyCode=code)
        time.sleep(hold)
        self.call("Input.dispatchKeyEvent", type="keyUp", key=key, code=key,
                  windowsVirtualKeyCode=code, nativeVirtualKeyCode=code)
        time.sleep(0.25)

    def capture(self, name):
        time.sleep(0.7)
        state = self.evaluate(STATE)
        assert not state["error"], state
        (self.evidence / f"{name}.json").write_text(json.dumps(state, indent=2))
        image = self.call("Page.captureScreenshot", format="png")
        (self.evidence / f"{name}.png").write_bytes(base64.b64decode(image["data"]))
        print(name, json.dumps(state), flush=True)
        return state

    def start(self):
        ensure_supported_host()
        require_nwjs_arm64(self.root / "runtime/v0.94.1")
        for name in ("home", "profile", "cache", "tmp"):
            (self.root / name).mkdir(exist_ok=True)
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        game = self.root / "game" / f"rpgmaker-mz-demo-{COMMIT}"
        command = [str(self.root / "runtime/v0.94.1/nwjs.app/Contents/MacOS/nwjs"),
                   f"--user-data-dir={self.root / 'profile'}",
                   "--remote-debugging-address=127.0.0.1", f"--remote-debugging-port={port}",
                   str(game)]
        env = dict(PATH="/usr/bin:/bin:/usr/sbin:/sbin", HOME=str(self.root / "home"),
                   TMPDIR=str(self.root / "tmp"), XDG_CACHE_HOME=str(self.root / "cache"))
        self.output = (self.evidence / f"nwjs-{self.run}.log").open("w")
        self.process = subprocess.Popen(command, cwd=game, env=env, stdout=self.output, stderr=subprocess.STDOUT)
        (self.evidence / f"launch-{self.run}.json").write_text(json.dumps(dict(command=command, env=env, pid=self.process.pid), indent=2))
        deadline = time.monotonic() + 30
        targets = []
        while time.monotonic() < deadline:
            assert self.process.poll() is None, f"NW.js exited {self.process.returncode}; inspect log"
            try:
                with urllib.request.urlopen(f"http://127.0.0.1:{port}/json/list", timeout=1) as response:
                    targets = [t for t in json.load(response) if t.get("type") == "page" and t.get("url", "").endswith("index.html")]
                if targets:
                    break
            except OSError:
                pass
            time.sleep(0.2)
        assert targets, "No real game renderer CDP target"
        listeners = subprocess.check_output(["/usr/sbin/lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN"], text=True)
        (self.evidence / f"listeners-{self.run}.txt").write_text(listeners)
        assert f"127.0.0.1:{port}" in listeners and f"*:{port}" not in listeners, listeners
        self.ws = websocket.create_connection(targets[0]["webSocketDebuggerUrl"], timeout=20, suppress_origin=True)
        self.call("Runtime.enable")
        self.call("Log.enable")
        self.call("Network.enable")
        self.call("Page.bringToFront")
        self.wait("typeof SceneManager !== 'undefined' && SceneManager._scene?.constructor.name === 'Scene_Title' && SceneManager._scene._commandWindow?.active && SceneManager._scene._commandWindow.isOpen()")
        versions = self.evaluate("({engine:Utils.RPGMAKER_NAME,engineVersion:Utils.RPGMAKER_VERSION,arch:process.arch,versions:process.versions,savePath:StorageManager.fileDirectoryPath(),plugins:$plugins.filter(p=>p.status).map(p=>p.name),webgl:Graphics.app.renderer.type,audioContext:WebAudio._context?.state})")
        (self.evidence / f"versions-{self.run}.json").write_text(json.dumps(versions, indent=2))
        assert versions["engine"] == "MZ" and versions["engineVersion"] == "1.8.0"
        assert versions["arch"] == "arm64" and versions["versions"]["nw"] == "0.94.1"
        assert Path(versions["savePath"]).resolve().is_relative_to(game.resolve())
        return self

    def close(self):
        if self.ws:
            try:
                self.evaluate("setTimeout(()=>nw.App.quit(),50); true")
            except (OSError, websocket.WebSocketException, AssertionError):
                pass
            self.ws.close()
        if self.process:
            try:
                self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.terminate()
                try:
                    self.process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    self.process.kill()
                    self.process.wait()
            self.output.close()
        self.log.close()


def main():
    ensure_supported_host()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path)
    parser.add_argument("--reviewed-fixture", action="store_true", required=True)
    args = parser.parse_args()
    root = args.root.resolve()
    require_nwjs_arm64(root / "runtime/v0.94.1")
    game = root / "game" / f"rpgmaker-mz-demo-{COMMIT}"
    assert (root / "runtime/v0.94.1/.nwjs-install.json").is_file(), "Install with the Go helper first"
    assert (game / "package.json").is_file()
    assert not (game / "save/file1.rmmzsave").exists(), "Use a fresh dedicated fixture copy, not existing saves"
    evidence = root / "evidence" / f"renderer-{time.time_ns()}"
    evidence.mkdir(parents=True)
    start = time.monotonic()
    first = Game(root, evidence, 1)
    try:
        first.start()
        first.capture("01-title")
        first.key("Enter", 13)
        first.wait("SceneManager._scene.constructor.name === 'Scene_Map' && $gamePlayer.canMove()")
        initial = first.capture("02-new-game")
        assert (initial["map"], initial["x"], initial["y"]) == (1, 8, 7)
        first.key("ArrowLeft", 37)
        first.key("ArrowUp", 38)
        first.wait("!$gamePlayer.isMoving()")
        moved = first.capture("03-movement")
        assert (moved["x"], moved["y"]) == (7, 6), moved
        first.key("Enter", 13)
        first.wait("$gameSwitches.value(2) && !$gameMap.isEventRunning()")
        saved = first.capture("04-switch-event")
        first.key("Escape", 27)
        first.wait("SceneManager._scene.constructor.name === 'Scene_Menu' && SceneManager._scene._commandWindow.active")
        # Navigate actual menu commands with key events, not engine save APIs.
        for _ in range(10):
            if first.evaluate("SceneManager._scene._commandWindow.currentSymbol() === 'save'"):
                break
            first.key("ArrowDown", 40)
        assert first.evaluate("SceneManager._scene._commandWindow.currentSymbol() === 'save'")
        first.key("Enter", 13)
        first.wait("SceneManager._scene.constructor.name === 'Scene_Save' && SceneManager._scene._listWindow.active")
        first.capture("05-save-menu")
        assert first.evaluate("SceneManager._scene.savefileId()") == 1
        first.key("Enter", 13)
        first.wait("SceneManager._scene.constructor.name === 'Scene_Menu'")
        assert (game / "save/file1.rmmzsave").stat().st_size > 0
        first.capture("06-saved")
    finally:
        first.close()
    second = Game(root, evidence, 2)
    try:
        second.start()
        second.capture("07-relaunch-title")
        assert second.evaluate("SceneManager._scene._commandWindow.currentSymbol() === 'continue'")
        second.key("Enter", 13)
        second.wait("SceneManager._scene.constructor.name === 'Scene_Load' && SceneManager._scene._listWindow.active")
        second.capture("08-load-menu")
        assert second.evaluate("SceneManager._scene.savefileId()") == 1
        second.key("Enter", 13)
        second.wait("SceneManager._scene.constructor.name === 'Scene_Map' && $gamePlayer.canMove()")
        loaded = second.capture("09-loaded")
        for key in ("map", "x", "y", "direction", "switch2"):
            assert loaded[key] == saved[key], (key, saved, loaded)
        (evidence / "result.json").write_text(json.dumps(dict(passed=True, elapsed_seconds=time.monotonic()-start, saved=saved, loaded=loaded, input="CDP key events; no engine save/load API calls", audibility="not tested"), indent=2))
    finally:
        second.close()
    print(f"PASS: {evidence}")


if __name__ == "__main__":
    main()

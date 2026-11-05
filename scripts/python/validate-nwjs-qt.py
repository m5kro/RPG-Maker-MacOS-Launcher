#!/usr/bin/env python3
"""Offscreen real launcher -> QProcess -> Go helper no-op installation smoke.

Use only a dedicated validation root containing a helper-installed runtime.
Only modal dialog answers are automated; acquisition, signals, refresh and
settings persistence use production code. No legacy installers/game launches.
"""
import json
import os
from pathlib import Path
import shutil
import sys
import time
from unittest.mock import patch

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT / "src/python"))
from runtime_arch import ensure_supported_host, require_arm64, require_nwjs_arm64

ensure_supported_host()
root = Path(sys.argv[1]).resolve()
require_nwjs_arm64(root / "runtime/v0.94.1")
require_arm64(root / "nwjs-helper")
assert (root / "runtime/v0.94.1/.nwjs-install.json").is_file()
home = root / "qt-home"
home.mkdir(mode=0o700)  # Refuse to reuse an existing profile/settings directory.
managed = home / "Library/Application Support/RPGM-Launcher"
managed.mkdir(parents=True, mode=0o700)
shutil.copytree(root / "runtime/v0.94.1", managed / "v0.94.1", symlinks=True)
os.environ.update(HOME=str(home), QT_QPA_PLATFORM="offscreen", XDG_CACHE_HOME=str(root / "cache"))
from PySide6.QtCore import QTimer
from PySide6.QtTest import QTest
from PySide6.QtWidgets import QApplication, QMessageBox
import main

app = QApplication([])
main.check_appdir()
window = main.RPGMLauncher()
window.nwjs_process.executable = str(root / "nwjs-helper")
calls, events, progress, notices, ticks = [], [], [], [], []
original_start = window.nwjs_process.start

def start(arguments, expected):
    calls.append(arguments)
    original_start(arguments, expected)

def select(versions):
    assert "v0.94.1" in versions
    return "v0.94.1", True

def notice(parent, title, message):
    notices.append(dict(title=title, message=message))
    return QMessageBox.Ok

window.nwjs_process.done.connect(lambda result, error, cancelled: events.append(dict(result=result, error=error, cancelled=cancelled)))
window.nwjs_process.progress.connect(progress.append)
timer = QTimer()
timer.timeout.connect(lambda: ticks.append(time.monotonic()))
timer.start(10)
started = time.monotonic()
try:
    with patch.object(window.nwjs_process, "start", side_effect=start), patch.object(window, "show_version_selection_dialog", side_effect=select), patch.object(QMessageBox, "information", side_effect=notice), patch.object(QMessageBox, "critical", side_effect=notice):
        window.show()
        window.install_button.click()
        assert window._nwjs_busy and not window.central_widget.isEnabled()
        deadline = time.monotonic() + 60
        while window._nwjs_busy and time.monotonic() < deadline:
            QTest.qWait(20)
        assert not window._nwjs_busy, "Timed out waiting for real helper"
        assert len(events) == 2 and all(not e["error"] and not e["cancelled"] for e in events), events
        assert all(v["arches"] == ["arm64"] for v in events[0]["result"]["versions"])
        assert calls == [["versions"], ["install", "--root", str(managed), "--version", "v0.94.1"]], calls
        assert events[1]["result"]["path"] == str(managed / "v0.94.1/nwjs.app")
        assert not progress, "Matching existing install should not download/extract"
        assert window.version_selector.currentText() == "v0.94.1"
        settings = json.loads(Path(window.SETTINGS_FILE).read_text())
        assert settings["last_selected_version"] == "v0.94.1"
        assert window.central_widget.isEnabled() and window.install_button.isEnabled()
        assert not window.optimize_space_checkbox.isEnabled()
        assert ticks and "successfully" in notices[-1]["message"], notices
        window.grab().save(str(root / "evidence/qt-offscreen.png"))
        result = dict(passed=True, elapsed_seconds=time.monotonic()-started, calls=calls, events=events, progress=progress, timer_ticks=len(ticks), notices=notices, settings=settings, selected=window.version_selector.currentText())
        (root / "evidence/qt-smoke.json").write_text(json.dumps(result, indent=2))
        print("PASS real QProcess GUI no-op", result["elapsed_seconds"], "seconds", len(events[0]["result"]["versions"]), "official versions", len(ticks), "event-loop ticks")
finally:
    timer.stop()
    if window.nwjs_process.running:
        window.nwjs_process.cancel()
        deadline = time.monotonic() + 5
        while window.nwjs_process.running and time.monotonic() < deadline:
            QTest.qWait(20)
    window.close()
    app.processEvents()

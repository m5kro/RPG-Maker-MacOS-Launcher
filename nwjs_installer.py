"""NW.js helper protocol and the shared Darwin install lock; no GUI dialogs."""
from contextlib import contextmanager
import fcntl
import json
import os
from pathlib import Path
import platform
import re
import stat
import sys

from PySide6.QtCore import QObject, QProcess, QTimer, Signal

VERSION = re.compile(r"v\d+\.\d+\.\d+", re.ASCII)
LOCK_NAME = ".nwjs-install.lock"


def helper_path():
    base = Path(__file__).resolve().parent
    if getattr(sys, "frozen", False):
        return str(base / "nwjs-helper")
    arch = "arm64" if platform.machine() == "arm64" else "x64"
    return str(base / "build" / "helpers" / arch / "nwjs-helper")


def installed_versions(root):
    root = Path(root)
    if not root.is_dir():
        return []
    versions = []
    for directory in root.iterdir():
        executable = directory / "nwjs.app/Contents/MacOS/nwjs"
        if (VERSION.fullmatch(directory.name) and not directory.is_symlink()
                and executable.is_file() and os.access(executable, os.X_OK)):
            versions.append(directory.name)
    return sorted(versions, key=lambda v: tuple(map(int, v[1:].split('.'))), reverse=True)


@contextmanager
def installation_lock(root):
    """Match Go's nonblocking flock; never unlink the persistent lock inode."""
    root = Path(root)
    info = root.lstat()
    if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid()
            or info.st_mode & 0o022):
        raise OSError("Unsafe managed directory")
    name = root / LOCK_NAME
    fd = os.open(name, os.O_RDWR | os.O_CREAT | os.O_CLOEXEC | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600)
    try:
        info = os.fstat(fd)
        if (not stat.S_ISREG(info.st_mode) or info.st_nlink != 1
                or info.st_uid != os.geteuid() or info.st_mode & 0o077):
            raise OSError("Unsafe NW.js install lock")
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if not os.path.samestat(info, name.lstat()):
            raise OSError("NW.js install lock changed")
        yield
    finally:
        os.close(fd)


class HelperProcess(QObject):
    progress = Signal(dict)
    # A result is emitted only after a normal zero exit and the expected event.
    done = Signal(object, str, bool)  # result, diagnostic, cancelled

    def __init__(self, executable=None, parent=None):
        super().__init__(parent)
        self.executable = executable or helper_path()
        self.process = None
        self.running = False
        self.kill_timeout_ms = 3000
        self._kill_timer = QTimer(self)
        self._kill_timer.setSingleShot(True)
        self._kill_timer.timeout.connect(self._kill)

    def start(self, arguments, expected):
        if self.running:
            raise RuntimeError("NW.js helper is already running")
        self.running = True
        self._expected = expected
        self._buffer = b""
        self._stderr = b""
        self._result = None
        self._error = ""
        self._cancelled = False
        process = self.process = QProcess(self)
        process.readyReadStandardOutput.connect(self._read_stdout)
        process.readyReadStandardError.connect(self._read_stderr)
        process.errorOccurred.connect(self._process_error)
        process.finished.connect(self._finished)
        process.started.connect(self._started)
        process.start(self.executable, arguments)

    def _started(self):
        if self._cancelled:
            self._stop()

    def cancel(self):
        if self.running and not self._cancelled:
            self._cancelled = True
            self._stop()

    def _stop(self):
        self.process.terminate()
        if not self._kill_timer.isActive():
            self._kill_timer.start(self.kill_timeout_ms)

    def _kill(self):
        if self.running:
            self.process.kill()

    def _read_stderr(self):
        # Keep the diagnostic tail bounded even if a broken helper floods stderr.
        self._stderr = (self._stderr + bytes(self.process.readAllStandardError()))[-65536:]

    def _read_stdout(self):
        data = bytes(self.process.readAllStandardOutput())
        if self._error or self._cancelled:
            return
        self._buffer += data
        while b"\n" in self._buffer:
            line, self._buffer = self._buffer.split(b"\n", 1)
            self._line(line)
            if self._error:
                self._buffer = b""
                break
        if len(self._buffer) > 8 * 1024 * 1024:
            self._protocol_error("Oversized helper JSON line")

    def _protocol_error(self, message):
        self._error = message
        self._stop()

    def _line(self, line):
        if not line.strip():
            return
        try:
            if len(line) > 8 * 1024 * 1024:
                raise ValueError("oversized JSON line")
            event = json.loads(line)
            if not isinstance(event, dict):
                raise ValueError("expected JSON object")
            kind = event.get("event")
            if kind == "progress":
                if (event.get("phase") not in ("download", "verify", "extract")
                        or any(type(event.get(k)) is not int or event[k] < 0 for k in ("completed", "total"))):
                    raise ValueError("invalid progress")
                self.progress.emit(event)
                return
            if kind != self._expected or self._result is not None:
                raise ValueError("unexpected or duplicate success event")
            if kind == "versions":
                versions = event.get("versions")
                if not isinstance(versions, list):
                    raise ValueError("invalid versions list")
                seen = set()
                for item in versions:
                    if (not isinstance(item, dict) or not isinstance(item.get("version"), str)
                            or not VERSION.fullmatch(item["version"])
                            or item["version"] in seen or not isinstance(item.get("arches"), list)
                            or not item["arches"] or any(a not in ("arm64", "x64") for a in item["arches"])):
                        raise ValueError("invalid version or architecture")
                    seen.add(item["version"])
            elif kind == "complete":
                if (not isinstance(event.get("version"), str) or not VERSION.fullmatch(event["version"])
                        or not isinstance(event.get("path"), str) or not os.path.isabs(event["path"])):
                    raise ValueError("invalid installation result")
            else:
                raise ValueError("unknown helper event")
            self._result = event
        except (ValueError, TypeError) as exc:
            self._protocol_error(f"Invalid NW.js helper output: {exc}")

    def _process_error(self, error):
        if not self.running:
            return
        if error == QProcess.FailedToStart:
            self._error = (f"Cannot start NW.js helper: {self.executable}\n{self.process.errorString()}\n"
                           "For source builds, run: sh scripts/build-nwjs-helper.sh\n"
                           "For packaged apps, reinstall a complete launcher build.")
            self._finish(None, self._error)
        elif not self._cancelled:
            self._error = self._error or self.process.errorString()

    def _finished(self, code, status):
        if not self.running:
            return
        self._read_stdout()
        self._read_stderr()
        if self._buffer and not self._error and not self._cancelled:
            self._line(self._buffer)
        error = self._error
        if status != QProcess.NormalExit or code != 0:
            error = error or f"NW.js helper exited with code {code}"
        if not error and self._result is None:
            error = f"NW.js helper exited without a {self._expected} event"
        if error and self._stderr:
            error += "\n" + self._stderr.decode("utf-8", errors="replace")
        self._finish(None if error else self._result, error)

    def _finish(self, result, error):
        self._kill_timer.stop()
        self.running = False
        process, self.process = self.process, None
        process.deleteLater()
        if self._cancelled:
            self.done.emit(None, "", True)
        else:
            self.done.emit(result, error, False)

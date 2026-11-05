"""Native host and executable-header checks; no Qt or developer tools required.

This checks the main Mach-O executable, not signatures, dylibs or game compatibility.
"""
import json
import os
from pathlib import Path
import platform
import re
import stat
import struct
import sys

VERSION = re.compile(r"v\d+\.\d+\.\d+", re.ASCII)
ARM64 = 0x0100000C


def ensure_supported_host():
    if sys.platform != "darwin" or platform.machine() != "arm64":
        raise RuntimeError("This launcher requires macOS Apple Silicon arm64. Intel/Rosetta is unsupported; use native arm64 Python (not an Intel Python under Rosetta).")


def _thin_cpu(file, offset, size):
    file.seek(offset)
    magic = file.read(4)
    formats = {b"\xcf\xfa\xed\xfe": ("<", 32), b"\xfe\xed\xfa\xcf": (">", 32),
               b"\xce\xfa\xed\xfe": ("<", 28), b"\xfe\xed\xfa\xce": (">", 28)}
    endian, header_size = formats[magic]
    if size < header_size:
        raise ValueError("Truncated Mach-O header")
    header = file.read(header_size - 4)
    cpu, _, kind, commands, command_size, _ = struct.unpack(endian + "6I", header[:24])
    if (len(header) != header_size - 4 or kind != 2 or command_size > size - header_size
            or commands > command_size // 8 or (cpu == ARM64 and header_size != 32)):
        raise ValueError("Invalid Mach-O executable header")
    return cpu


def is_arm64_executable(path):
    """Accept thin arm64 or an intact FAT table containing an arm64 executable slice."""
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, "rb") as file:
            info = os.fstat(file.fileno())
            if not stat.S_ISREG(info.st_mode) or not os.access(path, os.X_OK):
                return False
            magic = file.read(4)
            formats = {b"\xca\xfe\xba\xbe": (">", "5I"), b"\xbe\xba\xfe\xca": ("<", "5I"),
                       b"\xca\xfe\xba\xbf": (">", "IIQQII"), b"\xbf\xba\xfe\xca": ("<", "IIQQII")}
            if magic not in formats:
                return _thin_cpu(file, 0, info.st_size) == ARM64
            endian, layout = formats[magic]
            count, = struct.unpack(endian + "I", file.read(4))
            entry_size = struct.calcsize(endian + layout)
            table_end = 8 + count * entry_size
            if not count or table_end > info.st_size:
                return False
            ranges, found = [], False
            for index in range(count):
                file.seek(8 + index * entry_size)
                cpu, _, offset, size, *_ = struct.unpack(endian + layout, file.read(entry_size))
                if offset < table_end or size < 28 or offset > info.st_size - size:
                    return False
                if any(offset < end and start < offset + size for start, end in ranges):
                    return False
                ranges.append((offset, offset + size))
                if _thin_cpu(file, offset, size) != cpu:
                    return False
                found |= cpu == ARM64
            return found
    except (OSError, ValueError, KeyError, struct.error, OverflowError):
        return False


def require_arm64(path):
    if not is_arm64_executable(path):
        raise ValueError(f"Unsupported runtime: an executable Mach-O with arm64 support is required: {path}. Intel-only, missing or invalid runtimes cannot be used; existing files were not removed.")


def require_nwjs_arm64(directory):
    directory = Path(directory)
    if not VERSION.fullmatch(directory.name):
        raise ValueError("Select a valid arm64 NW.js version (vX.Y.Z).")
    binary = directory / "nwjs.app/Contents/MacOS/nwjs"
    for parent in (directory, binary.parents[2], binary.parents[1], binary.parent):
        if parent.is_symlink() or not parent.is_dir():
            raise ValueError(f"Unsafe or missing arm64 NW.js directory: {parent}")
    if os.path.lexists(directory / "run-with-rosetta"):
        raise ValueError("Legacy Rosetta NW.js is unsupported; its files have been preserved. Install an arm64 release separately.")
    receipt = directory / ".nwjs-install.json"
    if os.path.lexists(receipt):
        if receipt.is_symlink() or not receipt.is_file() or receipt.stat().st_size > 4096:
            raise ValueError("Invalid NW.js install receipt; existing files have been preserved.")
        with receipt.open() as file:
            data = json.load(file)
        if (not isinstance(data, dict) or data.get("version") != directory.name
                or data.get("arch") != "arm64" or data.get("rosetta", False) is not False):
            raise ValueError("Legacy or invalid NW.js receipt: arm64 is required; existing files have been preserved.")
    require_arm64(binary)
    return binary

"""ARM-only runtime boundary tests, without Qt/network or real executables."""
import os
from pathlib import Path
import struct
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from macho_fixture import ARM64, X64, executable, fat, thin
from runtime_arch import ensure_supported_host, is_arm64_executable, require_arm64


class RuntimeArchTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / "runtime"

    def test_arm_thin_and_external_universal_are_supported(self):
        for data in [thin(), thin(endian=">")] + [fat(endian=e, wide=w) for e in ("<", ">") for w in (False, True)]:
            with self.subTest(data=data):
                executable(self.path, data)
                self.assertTrue(is_arm64_executable(self.path))
                require_arm64(self.path)

    def test_intel_missing_non_executable_symlink_and_directory_are_rejected(self):
        self.assertFalse(is_arm64_executable(self.path))
        for data in (thin(X64), fat((X64,))):
            executable(self.path, data)
            self.assertFalse(is_arm64_executable(self.path))
        executable(self.path)
        self.path.chmod(0o600)
        self.assertFalse(is_arm64_executable(self.path))
        link = self.path.with_name("link")
        link.symlink_to(self.path)
        self.path.chmod(0o700)
        self.assertFalse(is_arm64_executable(link))
        self.assertFalse(is_arm64_executable(self.path.parent))
        fifo = self.path.with_name("fifo")
        os.mkfifo(fifo)
        self.assertFalse(is_arm64_executable(fifo))
        with patch("runtime_arch.os.open", side_effect=PermissionError):
            self.assertFalse(is_arm64_executable(self.path))

    def test_malformed_headers_and_fat_ranges_are_rejected(self):
        malformed = [b"", b"fake", thin()[:31], thin()[:4], fat()[:47], fat()[:-1],
                     struct.pack(">II", 0xCAFEBABE, 0),
                     struct.pack(">II", 0xCAFEBABE, 0xFFFFFFFF)]
        for offset, size in [(0, 32), (8, 32), (28, 0), (28, 31), (29, 32), (0xFFFFFFFF, 32), (28, 0xFFFFFFFF)]:
            malformed.append(struct.pack(">7I", 0xCAFEBABE, 1, ARM64, 0, offset, size, 0) + thin())
        for offset, size in [(0, 32), (40, 0), (40, 31), (41, 32), (2**64 - 1, 32), (40, 2**64 - 1)]:
            malformed.append(struct.pack(">IIIIQQII", 0xCAFEBABF, 1, ARM64, 0, offset, size, 0, 0) + thin())
        overlapping = bytearray(fat())
        struct.pack_into(">I", overlapping, 36, 48)
        malformed.append(overlapping)
        malformed += [struct.pack(">7I", 0xCAFEBABE, 1, ARM64, 0, 28, 32, 0) + thin(X64),
                      struct.pack("<8I", 0xFEEDFACF, ARM64, 0, 6, 0, 0, 0, 0),
                      struct.pack("<8I", 0xFEEDFACF, ARM64, 0, 2, 1, 16, 0, 0)]
        for data in malformed:
            with self.subTest(data=data):
                executable(self.path, data)
                self.assertFalse(is_arm64_executable(self.path))
                with self.assertRaisesRegex(ValueError, "arm64"):
                    require_arm64(self.path)

    def test_host_requires_native_darwin_arm64(self):
        for system, machine in [("linux", "arm64"), ("darwin", "x86_64"), ("win32", "AMD64")]:
            with self.subTest(system=system, machine=machine), patch("runtime_arch.sys.platform", system), patch("runtime_arch.platform.machine", return_value=machine):
                with self.assertRaisesRegex(RuntimeError, "native.*Python"):
                    ensure_supported_host()
        with patch("runtime_arch.sys.platform", "darwin"), patch("runtime_arch.platform.machine", return_value="arm64"):
            ensure_supported_host()

    def test_source_rejects_intel_before_qt_import_or_home_changes(self):
        root = Path(self.temp.name)
        log = root / "Library/Application Support/RPGM-Launcher/log.txt"
        log.parent.mkdir(parents=True)
        log.write_text("keep log")
        settings = log.with_name("settings.json")
        settings.write_text("keep settings")
        code = "import platform,runpy,sys; platform.machine=lambda:'x86_64'; sys.modules['PySide6']=None; runpy.run_path('main.py',run_name='__main__')"
        result = subprocess.run([sys.executable, "-c", code], cwd=Path(__file__).resolve().parents[1], env={**os.environ, "HOME": str(root)}, capture_output=True, text=True, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("native", result.stderr)
        self.assertNotIn("ModuleNotFoundError", result.stderr)
        self.assertEqual(log.read_text(), "keep log")
        self.assertEqual(settings.read_text(), "keep settings")

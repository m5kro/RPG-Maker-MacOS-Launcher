"""Keep distribution entry points in sync with the ARM-only launcher contract."""
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]


class DistributionTests(unittest.TestCase):
    def test_only_arm64_packaging_is_offered(self):
        self.assertEqual(list(ROOT.glob("*.spec")), [])
        self.assertEqual(sorted(p.relative_to(ROOT).as_posix() for p in (ROOT / "packaging").rglob("*.spec")), ["packaging/macos/arm64/arm64.spec"])
        spec = (ROOT / "packaging/macos/arm64/arm64.spec").read_text()
        self.assertIn("target_arch='arm64'", spec)
        self.assertIn("build/helpers/arm64/nwjs-helper", spec)
        build = (ROOT / "scripts/shell/build-nwjs-helper.sh").read_text()
        self.assertIn("GOARCH=arm64", build)
        for removed in ("amd64", "x64", "universal2", "-create"):
            self.assertNotIn(removed, build)
        self.assertNotIn("rm ", build)

    def test_standalone_uses_arm64_without_rosetta(self):
        standalone = (ROOT / "packaging/macos/arm64/EasyRPG-Standalone").read_text()
        self.assertIn('/usr/bin/arch -arm64 "${0%/*}/EasyRPG Player.app/Contents/MacOS/EasyRPG Player"', standalone)
        self.assertNotIn("x86", standalone)

    def test_live_validation_scripts_check_host_and_runtime_and_use_current_cli(self):
        for filename in ("validate-mz-runtime.py", "validate-nwjs-qt.py"):
            source = (ROOT / "scripts/python" / filename).read_text()
            with self.subTest(filename=filename):
                self.assertIn("ensure_supported_host()", source)
                self.assertIn("require_nwjs_arm64(", source)
                self.assertNotIn('"--arch"', source)
                self.assertNotIn('"--rosetta"', source)

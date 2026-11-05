"""Exercise relocated source/frozen resources and PyInstaller's actual inputs."""
import os
from pathlib import Path
import runpy
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

import main
import nwjs_installer

ROOT = Path(__file__).resolve().parents[2]
JS_NAMES = ("bg.js", "disable-child.js", "disable-net.js")
STANDALONE = ROOT / "packaging/macos/arm64/EasyRPG-Standalone"


class LayoutTests(unittest.TestCase):
    def test_language_and_distribution_layout(self):
        for relative in (
            "src/python/main.py", "src/python/nwjs_installer.py", "src/python/runtime_arch.py",
            "src/python/requirements-dev.txt", "src/go/go.mod", "src/go/cmd/nwjs-helper/main.go",
            "tests/python/macho_fixture.py", "scripts/shell/build-nwjs-helper.sh",
            "scripts/python/validate-mz-runtime.py", "scripts/python/validate-nwjs-qt.py",
            "assets/icons/icon.png", "assets/icons/icon.icns", "assets/icons/colorless-icon.png",
            "assets/icons/colorless-icon.icns", "assets/icons/old-icon.icns",
        ):
            with self.subTest(path=relative):
                self.assertTrue((ROOT / relative).is_file(), relative)
        for old in ("main.py", "nwjs_installer.py", "runtime_arch.py", "requirements-dev.txt",
                    "go.mod", "go.work", "cmd", "bg.js", "EasyRPG-Standalone", "icon.png",
                    "RPG-Maker-Launcher-arm64.spec", "packaging/windows"):
            self.assertFalse((ROOT / old).exists(), old)
        self.assertEqual(Path(main.__file__).resolve(), ROOT / "src/python/main.py")

    def check_resources(self, frozen):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            bundle = root / "bundle"
            bundle.mkdir()
            if frozen:
                # Distinct bytes prove reads use _MEIPASS, not the source checkout.
                for name in JS_NAMES + ("EasyRPG-Standalone",):
                    (bundle / name).write_text("// bundled " + name)
            old_cwd = Path.cwd()
            self.addCleanup(os.chdir, old_cwd)
            os.chdir(root)
            with patch.object(sys, "frozen", frozen, create=True), patch.object(sys, "_MEIPASS", str(bundle), create=True):
                env = runpy.run_path(main.__file__, run_name="layout_resources")
                env = env["RPGMLauncher"].prepare_package_json.__globals__
                expected_js = bundle if frozen else ROOT / "src/javascript"
                expected_standalone = bundle / "EasyRPG-Standalone" if frozen else STANDALONE
                self.assertEqual(env["JS_DIR"], expected_js)
                self.assertEqual(env["EASYRPG_STANDALONE"], expected_standalone)
                self.assertEqual(Path(nwjs_installer.helper_path()),
                                 bundle / "nwjs-helper" if frozen else ROOT / "build/helpers/arm64/nwjs-helper")
                cls = env["RPGMLauncher"]
                window = Mock(_nwjs_busy=False)
                window.disable_protection_checkbox.isChecked.return_value = False
                window.add_protection_script.side_effect = lambda folder: cls.add_protection_script(window, folder)
                game = root / "game"
                game.mkdir()
                package = game / "package.json"
                original = b'{"name":"game"}'
                package.write_bytes(original)
                with patch.dict(env, QMessageBox=Mock()):
                    for name in JS_NAMES:
                        collision = game / name
                        collision.write_text("user script")
                        self.assertFalse(cls.prepare_package_json(window, game))
                        self.assertEqual(package.read_bytes(), original)
                        self.assertEqual(collision.read_text(), "user script")
                        self.assertEqual(set(p.name for p in game.iterdir()), {"package.json", name})
                        collision.unlink()
                    link = game / "bg.js"
                    link.symlink_to(expected_js / "bg.js")
                    self.assertFalse(cls.prepare_package_json(window, game))
                    self.assertTrue(link.is_symlink())
                    link.unlink()
                    self.assertTrue(cls.prepare_package_json(window, game))
                    for name in JS_NAMES:
                        self.assertEqual((game / name).read_bytes(), (expected_js / name).read_bytes())
                    window.disable_protection_checkbox.isChecked.return_value = True
                    self.assertTrue(cls.prepare_package_json(window, game))
                    self.assertNotIn("bg-script", env["json"].loads(package.read_text()))
                    package.write_text('{"name":"game","bg-script":"bg.js"}')
                    (game / "bg.js").write_text("user background")
                    self.assertTrue(cls.prepare_package_json(window, game))
                    self.assertEqual(env["json"].loads(package.read_text())["bg-script"], "bg.js")
                    self.assertEqual((game / "bg.js").read_text(), "user background")

                home = root / "home"
                runtime = home / "Library/Application Support/RPGM-Launcher/EasyRPG Player.app"
                runtime.mkdir(parents=True)
                (runtime / "sentinel").write_text("runtime")
                destination = root / "export"
                destination.mkdir()
                window.get_app_name.return_value = ("Example", True)
                window.last_selected_folder = str(game)
                window.check_package_json.return_value = False
                window.check_game_ini.return_value = False
                window.check_RTP_RT.return_value = True
                window.check_easyrpg_installed.return_value = True
                progress = Mock()
                progress.wasCanceled.return_value = False
                message = Mock()
                with patch.dict(os.environ, HOME=str(home)), patch.dict(env,
                        QFileDialog=Mock(getExistingDirectory=Mock(return_value=str(destination))),
                        QProgressDialog=Mock(return_value=progress), QApplication=Mock(),
                        QMessageBox=message, subprocess=Mock()):
                    cls.export_standalone_app(window)
                message.critical.assert_not_called()
                self.assertEqual((destination / "Example.app/Example").read_bytes(), expected_standalone.read_bytes())
                self.assertEqual((destination / "Example.app/EasyRPG Player.app/sentinel").read_text(), "runtime")

    def test_source_resources_and_helper_from_another_cwd(self):
        self.check_resources(False)

    def test_frozen_resources_and_helper_use_meipass(self):
        self.check_resources(True)

    def test_spec_passes_absolute_inputs_and_flat_bundle_destinations(self):
        spec = ROOT / "packaging/macos/arm64/arm64.spec"
        analysis = Mock(return_value=SimpleNamespace(pure=[], scripts=[], binaries=[], datas=[]))
        exe, bundle = Mock(), Mock()
        with tempfile.TemporaryDirectory() as temporary:
            old_cwd = Path.cwd()
            try:
                os.chdir(temporary)
                runpy.run_path(str(spec), init_globals=dict(
                    SPECPATH=str(spec.parent), Analysis=analysis, PYZ=Mock(), EXE=exe,
                    COLLECT=Mock(), BUNDLE=bundle))
            finally:
                os.chdir(old_cwd)
        args, kwargs = analysis.call_args
        self.assertEqual(args[0], [str(ROOT / "src/python/main.py")])
        self.assertEqual(kwargs["pathex"], [str(ROOT / "src/python")])
        self.assertEqual(kwargs["binaries"], [(str(ROOT / "build/helpers/arm64/nwjs-helper"), ".")])
        self.assertEqual(set(kwargs["datas"]), {(str(STANDALONE), ".")} |
                         {(str(ROOT / "src/javascript" / name), ".") for name in JS_NAMES})
        for source, _ in kwargs["datas"] + kwargs["binaries"]:
            self.assertTrue(Path(source).is_file(), source)
        self.assertEqual(exe.call_args.kwargs["icon"], [str(ROOT / "assets/icons/icon.icns")])
        self.assertEqual(bundle.call_args.kwargs["icon"], str(ROOT / "assets/icons/icon.icns"))
        self.assertEqual(exe.call_args.kwargs["target_arch"], "arm64")
        self.assertEqual(exe.call_args.kwargs["name"], "RPG-Maker-Launcher")
        self.assertEqual(bundle.call_args.kwargs["name"], "RPG-Maker-Launcher.app")

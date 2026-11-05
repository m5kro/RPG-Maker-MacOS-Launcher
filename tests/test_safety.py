"""Behavior tests without importing/starting Qt or reading real user settings.

Only the targeted launcher methods are compiled from AST. All game/home files
are temporary and subprocess/UI calls are fakes. Extraction is NOT transactional:
these tests cover staging cleanup and EXE retention, not rollback of overwritten
assets. Real-game smoke tests must leave extraction disabled.
"""
import ast
import json
import os
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import Mock, patch


SOURCE = Path(__file__).resolve().parents[1] / "main.py"
METHODS = {
    "__init__", "load_settings", "select_folder", "check_package_json",
    "prepare_package_json", "add_protection_script", "start_game",
    "launch_nwjs_game", "export_standalone_app", "optimize_space",
    "check_and_unpack_game_en",
}


class FakeWindow:
    def __init__(self):
        pass

    def __getattr__(self, name):
        value = Mock()
        setattr(self, name, value)
        return value


def widget(*args):
    result = Mock()
    result.isChecked.return_value = False
    result.setChecked.side_effect = lambda value: setattr(result.isChecked, "return_value", value)
    result.wasCanceled.return_value = False
    return result


class SafetyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="rpgm-safety-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.game = self.root / "game"
        self.game.mkdir()
        self.assets = self.root / "launcher"
        self.assets.mkdir()
        for name in ("bg.js", "disable-child.js", "disable-net.js"):
            (self.assets / name).write_text("// launcher " + name)
        self.addCleanup(patch.stopall)
        patch.dict(os.environ, {"HOME": str(self.root)}).start()
        message = Mock(Yes=1, No=2)
        message.question.return_value = message.Yes
        process = Mock()
        process.Popen.side_effect = lambda *args, **kwargs: kwargs["stdout"].close()
        self.env = {
            "__file__": str(self.assets / "main.py"), "os": os, "json": json,
            "shutil": shutil, "tempfile": tempfile, "Path": Path,
            "logging": Mock(), "subprocess": process,
            "chardet": Mock(detect=Mock(return_value={"encoding": "utf-8"})),
            "QMainWindow": FakeWindow, "QMessageBox": message, "HelperProcess": Mock(),
            "QFileDialog": Mock(), "QApplication": Mock(), "Qt": Mock(),
            "unpack_files": Mock(), "LOG_FILE": str(self.root / "log.txt"),
            "current_version": "new", "config_version": "old",
        }
        for name in ("QWidget", "QVBoxLayout", "QHBoxLayout", "QPushButton",
                     "QLabel", "QComboBox", "QCheckBox", "QProgressDialog"):
            self.env[name] = widget
        tree = ast.parse(SOURCE.read_text())
        original = next(n for n in tree.body if isinstance(n, ast.ClassDef) and n.name == "RPGMLauncher")
        original.body = [n for n in original.body if isinstance(n, ast.FunctionDef) and n.name in METHODS]
        exec(compile(ast.Module(body=[original], type_ignores=[]), str(SOURCE), "exec"), self.env)
        cls = self.env["RPGMLauncher"]
        self.app = cls.__new__(cls)
        self.app._nwjs_busy = False
        self.app.last_selected_folder = str(self.game)
        self.app.SETTINGS_FILE = str(self.root / "settings.json")
        for name in ("extract", "cheat_menu", "optimize_space", "disable_protection", "getpixel_patch"):
            setattr(self.app, name + "_checkbox", widget())
        self.app.version_selector = Mock()
        self.app.version_selector.currentText.return_value = "v1"
        self.app.patch_rpgm_getpixel = Mock(return_value={"ok": True})

    def package(self, data):
        path = self.game / "package.json"
        path.write_text(json.dumps(data))
        return path

    def prepare(self):
        # Fail clearly before the new method exists; never silently use FakeWindow's mock.
        self.assertTrue(hasattr(type(self.app), "prepare_package_json"))
        return self.app.prepare_package_json(str(self.game))

    def test_selection_is_read_only_and_preserves_named_profiles(self):
        profile = self.root / "Library/Application Support/valuable"
        profile.mkdir(parents=True)
        sentinel = profile / "save.dat"
        sentinel.write_text("save")
        package = self.package({"name": "valuable", "bg-script": "custom.js"})
        before = package.read_bytes()
        self.env["QFileDialog"].getExistingDirectory.return_value = str(self.game)
        self.app.select_folder()
        self.assertEqual(before, package.read_bytes())
        self.assertEqual(sentinel.read_text(), "save")
        self.assertEqual(list(self.game.iterdir()), [package])

    def test_package_repair_preserves_profiles_for_untrusted_names(self):
        for name in ("", "../../valuable", str(self.root / "absolute")):
            with self.subTest(name=name):
                self.package({"name": name})
                with patch.object(shutil, "rmtree") as remove:
                    self.assertTrue(self.prepare())
                    remove.assert_not_called()
                self.assertEqual(json.loads((self.game / "package.json").read_text())["name"], name or "tempname")

    def test_missing_name_and_launcher_protection_can_be_added_and_disabled(self):
        package = self.package({"main": "index.html"})
        self.assertTrue(self.prepare())
        self.assertEqual(json.loads(package.read_text()), {"main": "index.html", "name": "tempname", "bg-script": "bg.js"})
        self.assertEqual((self.game / "bg.js").read_bytes(), (self.assets / "bg.js").read_bytes())
        self.app.disable_protection_checkbox.setChecked(True)
        self.assertTrue(self.prepare())
        self.assertNotIn("bg-script", json.loads(package.read_text()))

    def test_user_background_scripts_survive_both_protection_settings(self):
        for bg in ("custom.js", "bg.js"):
            for disabled in (False, True):
                with self.subTest(bg=bg, disabled=disabled):
                    package = self.package({"name": "game", "bg-script": bg})
                    script = self.game / bg
                    script.write_text("// user script")
                    self.app.disable_protection_checkbox.setChecked(disabled)
                    self.assertTrue(self.prepare())
                    self.assertEqual(json.loads(package.read_text())["bg-script"], bg)
                    self.assertEqual(script.read_text(), "// user script")

    def test_protection_script_collision_aborts_without_overwriting(self):
        package = self.package({"name": "game"})
        before = package.read_bytes()
        (self.game / "disable-net.js").write_text("// user networking")
        self.assertFalse(self.prepare())
        self.assertEqual(package.read_bytes(), before)
        self.assertEqual((self.game / "disable-net.js").read_text(), "// user networking")
        self.assertFalse((self.game / "bg.js").exists())

    def test_invalid_json_fails_without_launching_or_rewriting(self):
        package = self.game / "package.json"
        for raw in ("{bad", "[]"):
            with self.subTest(raw=raw):
                package.write_text(raw)
                self.app.launch_nwjs_game = Mock()
                self.app.start_game()
                self.app.launch_nwjs_game.assert_not_called()
                self.assertEqual(package.read_text(), raw)

    def test_start_repairs_package_without_permissions_or_optimization(self):
        package = self.package({"name": ""})
        self.app.optimize_space_checkbox.setChecked(True)
        self.app.optimize_space = Mock()
        self.app.launch_nwjs_game = Mock()
        self.app.start_game()
        self.env["subprocess"].run.assert_not_called()
        self.app.optimize_space.assert_not_called()
        self.app.launch_nwjs_game.assert_called_once_with(str(self.game))
        self.assertEqual(json.loads(package.read_text())["name"], "tempname")
        self.assertTrue((self.game / "bg.js").is_file())

    def test_launch_preserves_shared_nwjs_data(self):
        shared = self.root / "Library/Application Support/nwjs"
        shared.mkdir(parents=True)
        (shared / "save").write_text("valuable")
        self.app.launch_nwjs_game(str(self.game))
        self.assertEqual((shared / "save").read_text(), "valuable")
        self.env["subprocess"].Popen.assert_called_once()

    def test_optimize_space_is_inert_even_if_called_directly(self):
        for name in ("game.exe", "nw.dll", "locales/user-data"):
            path = self.game / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("keep")
        self.app.optimize_space()
        for name in ("game.exe", "nw.dll", "locales/user-data"):
            self.assertEqual((self.game / name).read_text(), "keep")

    def test_saved_optimization_enabled_is_ignored(self):
        Path(self.app.SETTINGS_FILE).write_text(json.dumps({"optimize_space": True}))
        self.app.load_settings()
        self.assertFalse(self.app.optimize_space_checkbox.isChecked())

    def test_initialization_preserves_old_settings_and_disables_optimization_ui(self):
        self.app.load_settings = Mock()
        self.app.__init__()
        self.app.remove_configs.assert_not_called()
        self.app.optimize_space_checkbox.setEnabled.assert_called_with(False)

    def test_export_repairs_copy_not_original_and_never_optimizes(self):
        package = self.package({"name": ""})
        before = package.read_bytes()
        runtime = self.root / "Library/Application Support/RPGM-Launcher/v1/nwjs.app/Contents/Resources"
        runtime.mkdir(parents=True)
        destination = self.root / "export"
        destination.mkdir()
        self.app.get_app_name.return_value = ("Example", True)
        self.env["QFileDialog"].getExistingDirectory.return_value = str(destination)
        self.app.optimize_space_checkbox.setChecked(True)
        self.app.optimize_space = Mock()
        self.app.export_standalone_app()
        self.app.optimize_space.assert_not_called()
        self.assertEqual(package.read_bytes(), before)
        exported = destination / "Example.app/Contents/Resources/app.nw"
        self.assertEqual(json.loads((exported / "package.json").read_text())["name"], "tempname")
        self.assertTrue((exported / "bg.js").exists())
        self.env["QMessageBox"].critical.assert_not_called()

    def test_extraction_uses_unique_staging_keeps_exe_and_existing_extracted_folder(self):
        exe = self.game / "game_en.exe"
        exe.write_text("original exe")
        existing = self.game / "extracted"
        existing.mkdir()
        (existing / "user-save").write_text("keep")
        staging = []

        def unpack(source, target, *_):
            staging.append(Path(target))
            Path(target).mkdir(exist_ok=True)
            (Path(target) / "translation.txt").write_text("translated")

        self.env["unpack_files"].side_effect = unpack
        self.app.check_and_unpack_game_en(str(self.game))
        self.assertEqual(exe.read_text(), "original exe")
        self.assertEqual((existing / "user-save").read_text(), "keep")
        self.assertNotEqual(staging[0], existing)
        self.assertFalse(staging[0].exists())
        self.assertEqual((self.game / "translation.txt").read_text(), "translated")
        self.env["QMessageBox"].question.assert_called_once()

    def test_extracted_payload_cannot_replace_original_exe(self):
        exe = self.game / "nested/game_en.exe"
        exe.parent.mkdir()
        exe.write_text("original")

        def unpack(source, target, *_):
            payload = Path(target) / "nested/game_en.exe"
            payload.parent.mkdir()
            payload.write_text("replacement")

        self.env["unpack_files"].side_effect = unpack
        self.app.check_and_unpack_game_en(str(self.game))
        self.assertEqual(exe.read_text(), "original")

    def test_extraction_failure_cleans_staging_but_does_not_promise_asset_rollback(self):
        exe = self.game / "game_en.exe"
        exe.write_text("original")
        staging = []

        def fail(source, target, *_):
            staging.append(Path(target))
            Path(target).mkdir(exist_ok=True)
            raise RuntimeError("unpack failed")

        self.env["unpack_files"].side_effect = fail
        with self.assertRaises(RuntimeError):
            self.app.check_and_unpack_game_en(str(self.game))
        self.assertEqual(exe.read_text(), "original")
        self.assertFalse(staging[0].exists())

    def test_extraction_cancel_does_not_unpack(self):
        (self.game / "game_en.exe").write_text("original")
        self.env["QMessageBox"].question.return_value = self.env["QMessageBox"].No
        self.assertFalse(self.app.check_and_unpack_game_en(str(self.game)))
        self.env["unpack_files"].assert_not_called()


if __name__ == "__main__":
    unittest.main()

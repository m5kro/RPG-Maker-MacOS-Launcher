"""Behavior tests without importing/starting Qt or reading real user settings.

Only the targeted launcher methods are compiled from AST. All game/home files
are temporary and subprocess/UI calls are fakes. Removed features have explicit
spies so FakeWindow cannot hide a remaining cheat or extraction call.
"""
import ast
import json
import os
from pathlib import Path
import re
import shutil
import tempfile
import unittest
from unittest.mock import Mock, patch
from macho_fixture import executable, thin, X64
from runtime_arch import VERSION, ensure_supported_host, require_arm64, require_nwjs_arm64


SOURCE = Path(__file__).resolve().parents[1] / "main.py"
METHODS = {
    "__init__", "load_settings", "select_folder", "check_package_json",
    "prepare_package_json", "add_protection_script", "start_game",
    "launch_nwjs_game", "export_standalone_app", "optimize_space",
    "patch_rpgm_getpixel", "_check_runtime", "_nwjs_executable",
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
            "shutil": shutil, "re": re, "Path": Path,
            "logging": Mock(), "subprocess": process,
            "chardet": Mock(detect=Mock(return_value={"encoding": "utf-8"})),
            "QMainWindow": FakeWindow, "QMessageBox": message, "HelperProcess": Mock(),
            "QFileDialog": Mock(), "QApplication": Mock(), "Qt": Mock(),
            "LOG_FILE": str(self.root / "log.txt"),
            "current_version": "new", "config_version": "old",
            "VERSION": VERSION, "ensure_supported_host": ensure_supported_host, "require_arm64": require_arm64,
            "require_nwjs_arm64": require_nwjs_arm64,
        }
        for name in ("QWidget", "QVBoxLayout", "QHBoxLayout", "QPushButton",
                     "QLabel", "QComboBox", "QCheckBox", "QProgressDialog"):
            self.env[name] = widget
        tree = ast.parse(SOURCE.read_text())
        original = next(n for n in tree.body if isinstance(n, ast.ClassDef) and n.name == "RPGMLauncher")
        original.body = [n for n in original.body if isinstance(n, ast.FunctionDef) and n.name in METHODS]
        self.assertEqual({n.name for n in original.body}, METHODS)
        exec(compile(ast.Module(body=[original], type_ignores=[]), str(SOURCE), "exec"), self.env)
        cls = self.env["RPGMLauncher"]
        self.app = cls.__new__(cls)
        self.app._nwjs_busy = False
        self.app.last_selected_folder = str(self.game)
        self.app.SETTINGS_FILE = str(self.root / "settings.json")
        for name in ("extract", "cheat_menu", "optimize_space", "disable_protection", "getpixel_patch"):
            setattr(self.app, name + "_checkbox", widget())
        self.app.version_selector = Mock()
        self.app.version_selector.currentText.return_value = "v1.0.0"
        self.runtime = executable(self.root / "Library/Application Support/RPGM-Launcher/v1.0.0/nwjs.app/Contents/MacOS/nwjs")
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

    def test_unsupported_nwjs_is_rejected_before_game_repair_launch_and_export(self):
        package = self.package({"name": ""})
        before = package.read_bytes()
        destination = self.root / "export"
        destination.mkdir()
        self.app.get_app_name.return_value = ("Example", True)
        self.env["QFileDialog"].getExistingDirectory.return_value = str(destination)
        marker = self.runtime.parents[3] / "run-with-rosetta"
        for mode in ("intel", "marker", "missing"):
            with self.subTest(mode=mode):
                executable(self.runtime, thin(X64) if mode == "intel" else thin())
                if mode == "marker":
                    marker.touch()
                if mode == "missing":
                    self.runtime.unlink()
                self.app.start_game()
                self.app.launch_nwjs_game(str(self.game))
                self.app.export_standalone_app()
                self.env["subprocess"].Popen.assert_not_called()
                self.app.patch_rpgm_getpixel.assert_not_called()
                self.assertEqual(package.read_bytes(), before)
                self.assertEqual(list(self.game.iterdir()), [package])
                self.assertEqual(list(destination.iterdir()), [])
                if mode == "marker":
                    self.assertTrue(marker.exists())
                    marker.unlink()

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
        runtime = self.root / "Library/Application Support/RPGM-Launcher/v1.0.0/nwjs.app/Contents/Resources"
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

    def game_fixture(self, engine, injected, legacy_enabled):
        self.game = self.root / f"{engine}-{injected}-{legacy_enabled}"
        self.game.mkdir()
        self.app.last_selected_folder = str(self.game)
        self.package({"name": "", "main": "index.html"})
        prefix = "www/" if engine == "MV" else ""
        if engine == "MV":
            main_js = "PluginManager.setup($plugins);\n"
            if injected:
                main_js += "PluginManager._path= 'js/plugins/';\nPluginManager.loadScript('Cheat_Menu.js');\n"
        else:
            main_js = 'const scriptUrls = [\n    "js/rmmz_managers.js",\n'
            if injected:
                main_js += '    "js/plugins/Cheat_Menu.js",\n'
            main_js += "];\n"
        files = {
            prefix + "js/main.js": main_js.encode(),
            "game_en.exe": b"original exe\x00\xff",
            "nested/GAME_EN.EXE": b"nested original exe",
            prefix + "data/English.json": b'{"text":"Keep English assets"}',
            prefix + "save/file1.rmmzsave": b"valuable save",
        }
        if injected:
            for name in ("Cheat_Menu.js", "Cheat_Menu.css"):
                files[prefix + "js/plugins/" + name] = b"existing user asset: " + name.encode()
        for name, data in files.items():
            path = self.game / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
        core = self.game / prefix / "js" / ("rpg_core.js" if engine == "MV" else "rmmz_core.js")
        core.write_text("Bitmap.prototype.getPixel = function(x, y) {\n    return '#ffffff';\n};\n")
        Path(self.app.SETTINGS_FILE).write_text(json.dumps({
            "extract_game_en": legacy_enabled, "add_cheat_menu": legacy_enabled,
            "last_selected_folder": str(self.game), "patch_getpixel": True,
        }))
        type(self.app).load_settings(self.app)
        spies = []
        for name in ("check_and_unpack_game_en", "add_cheat_menu", "remove_cheat_menu"):
            spy = Mock(name=name, return_value=True)
            setattr(self.app, name, spy)
            spies.append(spy)
        return files, core, spies

    def test_start_ignores_removed_features_and_preserves_existing_game_assets(self):
        for engine in ("MV", "MZ"):
            for injected in (False, True):
                for enabled in (False, True):
                    with self.subTest(engine=engine, injected=injected, legacy_enabled=enabled):
                        files, core, spies = self.game_fixture(engine, injected, enabled)
                        self.app.patch_rpgm_getpixel = type(self.app).patch_rpgm_getpixel.__get__(self.app)
                        self.app.launch_nwjs_game = Mock()
                        type(self.app).start_game(self.app)
                        self.app.launch_nwjs_game.assert_called_once_with(str(self.game))
                        for spy in spies:
                            spy.assert_not_called()
                        for name, data in files.items():
                            self.assertEqual((self.game / name).read_bytes(), data, name)
                        self.assertEqual(json.loads((self.game / "package.json").read_text())["name"], "tempname")
                        self.assertIn("RPGM-Launcher getPixel finite patch begin", core.read_text())
                        if not injected:
                            self.assertFalse(list(self.game.rglob("Cheat_Menu.*")))

    def test_export_copies_existing_assets_without_cheats_or_extraction(self):
        runtime = self.root / "Library/Application Support/RPGM-Launcher/v1.0.0/nwjs.app"
        (runtime / "Contents/Resources").mkdir(parents=True)
        binary = runtime / "Contents/MacOS/nwjs"
        executable(binary)
        destination = self.root / "export"
        destination.mkdir()
        self.env["QFileDialog"].getExistingDirectory.return_value = str(destination)
        for engine in ("MV", "MZ"):
            for injected in (False, True):
                with self.subTest(engine=engine, injected=injected):
                    _, _, spies = self.game_fixture(engine, injected, True)
                    before = {p.relative_to(self.game): p.read_bytes() for p in self.game.rglob("*") if p.is_file()}
                    app_name = self.game.name
                    self.app.get_app_name = Mock(return_value=(app_name, True))
                    type(self.app).export_standalone_app(self.app)
                    self.env["QMessageBox"].critical.assert_not_called()
                    for spy in spies:
                        spy.assert_not_called()
                    self.assertEqual({p.relative_to(self.game): p.read_bytes() for p in self.game.rglob("*") if p.is_file()}, before)
                    bundle = destination / (app_name + ".app")
                    exported = bundle / "Contents/Resources/app.nw"
                    self.assertEqual((bundle / "Contents/MacOS/nwjs").read_bytes(), binary.read_bytes())
                    for name, data in before.items():
                        if name != Path("package.json"):
                            self.assertEqual((exported / name).read_bytes(), data, str(name))
                    self.assertEqual(json.loads((exported / "package.json").read_text())["name"], "tempname")
                    self.assertTrue((exported / "bg.js").is_file())
                    if not injected:
                        self.assertFalse(list(exported.rglob("Cheat_Menu.*")))


if __name__ == "__main__":
    unittest.main()

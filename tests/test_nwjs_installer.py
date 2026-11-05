"""Real Qt event-loop/process tests; no network, games, or user settings."""
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
import zipfile
from unittest.mock import Mock, patch

os.environ.setdefault("QT_QPA_PLATFORM", "offscreen")
from PySide6.QtCore import QProcess, QTimer
from PySide6.QtTest import QTest
from PySide6.QtWidgets import QApplication, QAbstractButton, QDialog, QMessageBox, QPlainTextEdit

from nwjs_installer import HelperProcess, helper_path, installation_lock, installed_versions
import main
from macho_fixture import executable, fat, thin, X64

APP = QApplication.instance() or QApplication([])
APP.setQuitOnLastWindowClosed(False)


def until(predicate, timeout=4000):
    deadline = time.monotonic() + timeout / 1000
    while not predicate() and time.monotonic() < deadline:
        QTest.qWait(10)
    if not predicate():
        raise AssertionError("Qt operation did not complete in time")


class HelperFixture(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.helper = self.root / "fake-helper"
        self.processes = []

    def tearDown(self):
        for proc in self.processes:
            if proc.running:
                proc.cancel()
                until(lambda: not proc.running)
        self.temp.cleanup()

    def script(self, body):
        self.helper.write_text(f"#!{sys.executable}\nimport sys, os, time, json, signal\n" + body)
        self.helper.chmod(0o700)
        return str(self.helper)

    def run_helper(self, body, expected="versions"):
        proc = HelperProcess(self.script(body))
        proc.kill_timeout_ms = 80
        self.processes.append(proc)
        results = []
        progress = []
        proc.done.connect(lambda *args: results.append(args))
        proc.progress.connect(progress.append)
        proc.start([expected], expected)
        return proc, results, progress


class HelperTests(HelperFixture):
    def test_split_utf8_json_and_final_line_without_newline(self):
        proc, results, _ = self.run_helper(
            "payload = json.dumps({'event':'versions','versions':[{'version':'v1.2.3','arches':['arm64']}], 'note':'日本'}, ensure_ascii=False).encode()\n"
            "for b in payload:\n os.write(1, bytes([b])); time.sleep(.001)\n")
        until(lambda: bool(results))
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0][0]["note"], "日本")
        self.assertEqual(results[0][1:], ("", False))
        self.assertFalse(proc.running)

    def test_progress_and_complete_wait_for_exit(self):
        proc, results, progress = self.run_helper(
            "print(json.dumps({'event':'progress','phase':'extract','completed':8589934592,'total':8589934592}), flush=True)\n"
            "print(json.dumps({'event':'complete','version':'v1.2.3','path':'/tmp/app'}), flush=True)\n"
            "time.sleep(.15)\n", "complete")
        until(lambda: bool(progress))
        self.assertFalse(results)
        until(lambda: bool(results))
        self.assertEqual(results[0][1:], ("", False))

    def test_nonzero_even_after_success_is_failure_with_stderr(self):
        _, results, _ = self.run_helper("print('{\"event\":\"versions\",\"versions\":[]}')\nprint('checksum failed', file=sys.stderr)\nsys.exit(7)\n")
        until(lambda: bool(results))
        self.assertIsNone(results[0][0])
        self.assertIn("checksum failed", results[0][1])
        self.assertEqual(len(results), 1)

    def test_missing_success_and_invalid_json_fail(self):
        for body in ["pass\n", "print('broken json')\n", "print('{\"event\":\"versions\",\"versions\":\"bad\"}')\n"]:
            with self.subTest(body=body):
                _, results, _ = self.run_helper(body)
                until(lambda: bool(results))
                self.assertIsNone(results[0][0])
                self.assertTrue(results[0][1])

    def test_failed_start_notifies_once(self):
        proc = HelperProcess(str(self.root / "missing"))
        self.processes.append(proc)
        results = []
        proc.done.connect(lambda *args: results.append(args))
        proc.start(["versions"], "versions")
        until(lambda: bool(results))
        QTest.qWait(50)
        self.assertEqual(len(results), 1)
        self.assertIn("build", results[0][1].lower())
        self.assertFalse(proc.running)

    def test_cancel_terminates_and_waits_for_cleanup(self):
        proc, results, progress = self.run_helper(
            "def stop(*args):\n time.sleep(.05); sys.exit(1)\n"
            "signal.signal(signal.SIGTERM, stop)\n"
            "print('{\"event\":\"progress\",\"phase\":\"download\",\"completed\":0,\"total\":0}', flush=True)\n"
            "time.sleep(10)\n")
        until(lambda: bool(progress))
        proc.cancel()
        self.assertTrue(proc.running)
        self.assertFalse(results)
        until(lambda: bool(results))
        self.assertEqual(results, [(None, "", True)])

    def test_cancel_kills_unresponsive_helper_and_ui_keeps_ticking(self):
        proc, results, progress = self.run_helper(
            "signal.signal(signal.SIGTERM, signal.SIG_IGN)\n"
            "print('{\"event\":\"progress\",\"phase\":\"download\",\"completed\":0,\"total\":0}', flush=True)\n"
            "time.sleep(10)\n")
        until(lambda: bool(progress))
        ticks = []
        timer = QTimer()
        timer.timeout.connect(lambda: ticks.append(1))
        timer.start(10)
        proc.cancel()
        until(lambda: bool(results))
        timer.stop()
        self.assertGreater(len(ticks), 2)
        self.assertEqual(results, [(None, "", True)])

    def test_crash_notifies_once_and_allows_retry(self):
        proc, results, _ = self.run_helper("os.kill(os.getpid(), signal.SIGKILL)\n")
        until(lambda: bool(results))
        QTest.qWait(50)
        self.assertEqual(len(results), 1)
        self.assertIsNone(results[0][0])
        self.assertTrue(results[0][1])
        self.script("print('{\"event\":\"versions\",\"versions\":[]}')\n")
        proc.start(['versions'], 'versions')
        until(lambda: len(results) == 2)
        self.assertEqual(results[1][1:], ('', False))

    def test_duplicate_event_and_bad_trailing_json_fail(self):
        for tail in ["print('{\"event\":\"versions\",\"versions\":[]}')", "sys.stdout.write('{broken')"]:
            with self.subTest(tail=tail):
                _, results, _ = self.run_helper("print('{\"event\":\"versions\",\"versions\":[]}')\n" + tail + "\n")
                until(lambda: bool(results))
                self.assertIsNone(results[0][0])
                self.assertTrue(results[0][1])

    def test_discovery_requires_executable_and_excludes_staging(self):
        for version, executable in [("v1.2.3", True), ("v2.0.0", False), (".nwjs-stage-abc", True), ("vbroken", True)]:
            binary = self.root / version / "nwjs.app/Contents/MacOS/nwjs"
            binary.parent.mkdir(parents=True)
            binary.write_bytes(thin())
            binary.chmod(0o700 if executable else 0o600)
        (self.root / "v9.0.0").mkdir()
        self.assertEqual(installed_versions(self.root), ["v1.2.3"])

    def test_discovery_hides_legacy_intel_and_marker_without_deleting(self):
        for version, data in [("v1.0.0", thin()), ("v2.0.0", thin(X64)), ("v3.0.0", thin()), ("v4.0.0", thin())]:
            executable(self.root / version / "nwjs.app/Contents/MacOS/nwjs", data)
        (self.root / "v3.0.0/run-with-rosetta").touch()
        (self.root / "v4.0.0/.nwjs-install.json").write_text(json.dumps({"version":"v4.0.0", "arch":"x64", "rosetta":False}))
        before = {p: p.read_bytes() for p in self.root.rglob("*") if p.is_file()}
        self.assertEqual(installed_versions(self.root), ["v1.0.0"])
        self.assertEqual(before, {p: p.read_bytes() for p in self.root.rglob("*") if p.is_file()})

    def test_helper_path_never_falls_back_to_intel(self):
        with patch("platform.machine", return_value="x86_64"):
            self.assertEqual(Path(helper_path()).parts[-3:], ("helpers", "arm64", "nwjs-helper"))

    def test_built_go_helper_respects_python_lock_without_network(self):
        executable = helper_path()
        if not Path(executable).is_file():
            self.skipTest('Build the native Go helper to test the cross-language lock')
        proc = HelperProcess(executable)
        self.processes.append(proc)
        results = []
        proc.done.connect(lambda *args: results.append(args))
        with installation_lock(self.root):
            proc.start(['install', '--root', str(self.root), '--version', 'v1.2.3'], 'complete')
            until(lambda: bool(results))
        self.assertIsNone(results[0][0])
        self.assertIn('another helper may be installing', results[0][1])
        self.assertFalse((self.root / 'v1.2.3').exists())

    def test_lock_is_exclusive_and_persistent(self):
        with installation_lock(self.root):
            inode = (self.root / ".nwjs-install.lock").stat().st_ino
            with self.assertRaises(OSError):
                with installation_lock(self.root):
                    self.fail("lock was not exclusive")
        with installation_lock(self.root):
            self.assertEqual((self.root / ".nwjs-install.lock").stat().st_ino, inode)


class WindowTests(HelperFixture):
    def setUp(self):
        super().setUp()
        self.home = patch.dict(os.environ, {"HOME": str(self.root)})
        self.home.start()
        self.managed = self.root / "Library/Application Support/RPGM-Launcher"
        self.managed.mkdir(parents=True)
        self.patches = [patch.object(main.RPGMLauncher, "SETTINGS_FILE", str(self.managed / "settings.json"))]
        for name in ["critical", "information"]:
            self.patches.append(patch.object(QMessageBox, name, return_value=QMessageBox.Ok))
        for p in self.patches:
            p.start()
        self.window = main.RPGMLauncher()
        self.window.nwjs_process.executable = str(self.helper)
        self.window.nwjs_process.kill_timeout_ms = 150
        self.processes.append(self.window.nwjs_process)
        self.window.show_version_selection_dialog = Mock(return_value=("v1.2.3", True))
        self.window.save_settings = Mock()
        self.window.update_version_selector = Mock(wraps=self.window.update_version_selector)
        self.window.show()

    def tearDown(self):
        if self.window.nwjs_process.running:
            self.window.nwjs_process.cancel()
            until(lambda: not self.window.nwjs_process.running)
        self.window.close()
        self.window.deleteLater()
        APP.processEvents()
        for p in reversed(self.patches):
            p.stop()
        self.home.stop()
        super().tearDown()

    def test_legacy_feature_settings_are_ignored_and_not_saved(self):
        settings = {
            "extract_game_en": True, "add_cheat_menu": True,
            "optimize_space": False, "disable_protection": True, "patch_getpixel": False,
            "last_selected_version": "v1.2.3", "last_selected_folder": str(self.root / "game"),
            "launcher_version": main.current_version, "last_commit_sha": "keep-sha", "last_build_number": 42,
        }
        path = Path(self.window.SETTINGS_FILE)
        path.write_text(json.dumps(settings))
        original = path.read_bytes()
        self.window.version_selector.addItem("v1.2.3")
        with patch.multiple(main, config_version="", last_commit_sha="", last_build_number=0):
            main.RPGMLauncher.load_settings(self.window)
            self.assertEqual(path.read_bytes(), original)
            self.assertEqual(self.window.last_selected_folder, settings["last_selected_folder"])
            self.assertTrue(self.window.disable_protection_checkbox.isChecked())
            self.assertFalse(self.window.getpixel_patch_checkbox.isChecked())
            main.RPGMLauncher.save_settings(self.window)
        expected = {key: value for key, value in settings.items() if key not in ("extract_game_en", "add_cheat_menu")}
        self.assertEqual(json.loads(path.read_text()), expected)
        self.assertFalse(hasattr(self.window, "extract_checkbox"))
        self.assertFalse(hasattr(self.window, "cheat_menu_checkbox"))

    def test_removed_features_have_no_ui_or_callable_entry_points(self):
        texts = [button.text() for button in self.window.findChildren(QAbstractButton)]
        with patch.object(QDialog, "exec", return_value=QDialog.Accepted):
            self.window.show_instructions()
            self.window.show_credits()
        texts += [field.toPlainText() for field in self.window.findChildren(QPlainTextEdit)]
        for text in texts:
            with self.subTest(text=text):
                self.assertNotRegex(text.lower(), r"cheat|game_en\.exe|save editor|saveeditor")
        for name in ("extract_checkbox", "cheat_menu_checkbox", "open_save_editor_button",
                     "check_and_unpack_game_en", "open_save_editor", "add_cheat_menu",
                     "copy_cheat_files", "remove_cheat_menu", "remove_cheat_files",
                     "modify_MV_main_js", "modify_MZ_main_js", "unmodify_MV_main_js", "unmodify_MZ_main_js"):
            with self.subTest(attribute=name):
                self.assertFalse(hasattr(self.window, name))

    def test_launcher_imports_without_evbunpack(self):
        result = subprocess.run(
            [sys.executable, "-c", "import sys,platform; platform.machine=lambda:'x86_64'; sys.modules['evbunpack'] = None; import main"],
            cwd=Path(main.__file__).parent, capture_output=True, text=True, timeout=30,
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def install_script(self, end="success", arches=None):
        if arches is None:
            arches = ["arm64"]
        body = (
            "from pathlib import Path\n"
            f"with open({str(self.root / 'calls')!r}, 'a') as f: f.write(json.dumps(sys.argv[1:])+'\\n')\n"
            "if sys.argv[1] == 'versions':\n"
            f" print(json.dumps({{'event':'versions','versions':[{{'version':'v1.2.3','arches':{arches!r}}}]}}), flush=True)\n"
            " sys.exit(0)\n"
            "time.sleep(.12)\n"
        )
        if end == "success":
            body += (
                "root = Path(sys.argv[sys.argv.index('--root')+1])\n"
                "app = root / 'v1.2.3/nwjs.app'\n"
                "binary = app / 'Contents/MacOS/nwjs'\n"
                f"binary.parent.mkdir(parents=True); binary.write_bytes({thin()!r}); binary.chmod(0o700)\n"
                "print(json.dumps({'event':'complete','version':'v1.2.3','path':str(app.resolve())}), end='', flush=True)\n"
            )
        elif end == "nonzero":
            body += "print('download failed', file=sys.stderr); sys.exit(2)\n"
        elif end == "delay":
            body += "signal.signal(signal.SIGTERM, signal.SIG_IGN)\ntime.sleep(10)\n"
        self.script(body)

    def test_versions_install_refresh_only_after_success(self):
        self.install_script()
        self.window.install_button.click()
        self.assertFalse(self.window.uninstall_button.isEnabled())
        self.assertFalse(self.window.start_game_button.isEnabled())
        self.assertFalse(self.window.export_button.isEnabled())
        until(lambda: self.window.show_version_selection_dialog.called)
        self.window.update_version_selector.assert_not_called()
        self.window.save_settings.assert_not_called()
        until(lambda: not self.window._nwjs_busy)
        self.window.update_version_selector.assert_called_once()
        self.window.save_settings.assert_called_once()
        self.assertEqual(self.window.version_selector.currentText(), "v1.2.3")
        self.assertTrue(self.window.install_button.isEnabled())
        calls = [json.loads(line) for line in (self.root / "calls").read_text().splitlines()]
        self.assertEqual(calls[0], ["versions"])
        self.assertEqual(calls[1], ["install", "--root", str(self.managed), "--version", "v1.2.3"])
        self.assertFalse(hasattr(self.window, "show_rosetta_warning"))

    def test_install_failure_or_missing_complete_never_refresh(self):
        for end in ["nonzero", "missing"]:
            with self.subTest(end=end):
                self.install_script(end)
                self.window.install_nwjs()
                until(lambda: not self.window._nwjs_busy)
                self.window.update_version_selector.assert_not_called()
                self.window.save_settings.assert_not_called()
                self.assertTrue(self.window.install_button.isEnabled())

    def test_close_waits_for_helper_and_preserves_disabled_state(self):
        self.install_script("delay")
        self.window.install_nwjs()
        until(lambda: self.window.show_version_selection_dialog.called)
        QTest.qWait(200)
        self.window.close()
        self.assertTrue(self.window.isVisible())
        self.assertTrue(self.window.nwjs_process.running)
        self.window.save_settings.assert_not_called()
        until(lambda: not self.window.isVisible())
        self.assertFalse(self.window.nwjs_process.running)
        self.window.update_version_selector.assert_not_called()
        self.assertFalse(self.window.optimize_space_checkbox.isEnabled())

    def test_intel_or_mixed_protocol_does_not_offer_install(self):
        for arches in (["x64"], ["arm64", "x64"], ["arm64", "arm64"], []):
            with self.subTest(arches=arches), patch.object(QDialog, "exec", return_value=QDialog.Rejected):
                self.install_script(arches=arches)
                self.window.install_nwjs()
                until(lambda: not self.window._nwjs_busy)
                self.window.show_version_selection_dialog.assert_not_called()
                self.window.update_version_selector.assert_not_called()
                self.assertIn("architecture", QMessageBox.critical.call_args.args[2])
        calls = [json.loads(line) for line in (self.root / "calls").read_text().splitlines()]
        self.assertEqual(calls, [["versions"]] * 4)

    def test_complete_with_intel_binary_never_refreshes(self):
        self.install_script()
        self.helper.write_text(self.helper.read_text().replace(repr(thin()), repr(thin(X64))))
        self.window.install_nwjs()
        until(lambda: not self.window._nwjs_busy)
        self.window.update_version_selector.assert_not_called()
        self.window.save_settings.assert_not_called()

    def test_close_during_version_selection_does_not_start_install(self):
        self.install_script()
        def choose(versions):
            self.window.close()
            return 'v1.2.3', True
        self.window.show_version_selection_dialog.side_effect = choose
        self.window.install_nwjs()
        until(lambda: not self.window.isVisible())
        self.assertFalse(self.window.nwjs_process.running)
        self.assertEqual(len((self.root / 'calls').read_text().splitlines()), 1)
        self.window.update_version_selector.assert_not_called()

    def test_versions_failure_and_missing_helper_do_not_offer_selection(self):
        self.script("print('offline', file=sys.stderr); sys.exit(3)\n")
        self.window.install_nwjs()
        until(lambda: not self.window._nwjs_busy)
        self.window.show_version_selection_dialog.assert_not_called()
        self.window.update_version_selector.assert_not_called()
        self.helper.unlink()
        self.window.install_nwjs()
        until(lambda: not self.window._nwjs_busy)
        self.assertIn('scripts/build-nwjs-helper.sh', QMessageBox.critical.call_args.args[2])
        self.window.save_settings.assert_not_called()

    def test_cancel_button_during_versions_restores_controls_without_refresh(self):
        self.script("time.sleep(10)\n")
        self.window.install_nwjs()
        until(lambda: self.window.nwjs_process.process.state() == QProcess.Running)
        from PySide6.QtWidgets import QPushButton
        self.window._nwjs_dialog.findChild(QPushButton).click()
        until(lambda: not self.window._nwjs_busy)
        self.window.show_version_selection_dialog.assert_not_called()
        self.window.update_version_selector.assert_not_called()
        self.window.save_settings.assert_not_called()
        self.assertTrue(self.window.install_button.isEnabled())
        self.assertFalse(self.window.start_game_button.isEnabled())
        self.assertFalse(self.window.optimize_space_checkbox.isEnabled())

    def test_busy_blocks_direct_start_export_and_uninstall(self):
        self.script("time.sleep(10)\n")
        self.window.install_nwjs()
        with patch.object(main.shutil, 'rmtree') as remove, patch.object(main.subprocess, 'Popen') as launch:
            self.window.start_game()
            self.window.export_standalone_app()
            self.window.uninstall_nwjs()
            self.window.uninstall_all()
            remove.assert_not_called()
            launch.assert_not_called()
        self.window.show_version_selection_dialog.assert_not_called()
        self.window.nwjs_process.cancel()
        until(lambda: not self.window._nwjs_busy)

    def test_unsupported_host_rejects_main_before_log_or_settings_changes(self):
        log = self.managed / "log.txt"
        log.write_text("keep")
        settings = Path(self.window.SETTINGS_FILE)
        settings.write_text("keep settings")
        for system, machine in [("darwin", "x86_64"), ("linux", "arm64")]:
            with self.subTest(system=system), patch("runtime_arch.sys.platform", system), patch("runtime_arch.platform.machine", return_value=machine), patch.object(main, "check_appdir") as initialize:
                with self.assertRaisesRegex(RuntimeError, "native"):
                    main.main()
                initialize.assert_not_called()
                with self.assertRaises(RuntimeError):
                    main.RPGMLauncher()
        self.assertEqual(log.read_text(), "keep")
        self.assertEqual(settings.read_text(), "keep settings")

    def test_external_intel_refuses_launch_and_export_without_game_changes(self):
        game = self.root / "game"
        game.mkdir()
        destination = self.root / "export"
        destination.mkdir()
        self.window.last_selected_folder = str(game)
        for bundle, binary, check, launch, marker in [
            ("Z-universal.app", "Z-universal", self.window.check_mkxpz_installed, self.window.launch_mkxpz_game, "Game.ini"),
            ("EasyRPG Player.app", "EasyRPG Player", self.window.check_easyrpg_installed, self.window.launch_easyrpg_game, "RPG_RT.ldb"),
        ]:
            with self.subTest(bundle=bundle):
                (game / marker).write_text("original game")
                runtime = executable(self.managed / bundle / "Contents/MacOS" / binary, thin(X64))
                before = {p: p.read_bytes() for p in game.rglob("*") if p.is_file()}
                self.assertFalse(check(warn=False))
                with patch.object(main.subprocess, "Popen") as popen, patch.object(main.requests, "get") as download, patch.object(self.window, "get_app_name", return_value=("Export", True)), patch.object(main.QFileDialog, "getExistingDirectory", return_value=str(destination)):
                    launch(str(game))
                    self.window.export_standalone_app()
                    popen.assert_not_called()
                    download.assert_not_called()
                self.assertEqual(before, {p: p.read_bytes() for p in game.rglob("*") if p.is_file()})
                self.assertFalse(list(destination.iterdir()))
                self.assertEqual(runtime.read_bytes(), thin(X64))
                runtime.write_bytes(fat())
                self.assertTrue(check(warn=False))
                runtime.unlink()
                self.assertFalse(check(warn=False))
                (game / marker).unlink()

    def test_external_universal_launches_arm64_explicitly(self):
        game = self.root / "game"
        game.mkdir()
        for bundle, binary in [("Z-universal.app", "Z-universal"), ("EasyRPG Player.app", "EasyRPG Player")]:
            executable(self.managed / bundle / "Contents/MacOS" / binary, fat())
        config = self.managed / "Z-universal.app/Contents/Game/mkxp.json"
        config.parent.mkdir()
        config.write_text("{}")
        with patch.object(main.subprocess, "Popen", side_effect=lambda *a, **k: k["stdout"].close()) as popen, patch.object(main, "LOG_FILE", str(self.root / "game.log")), patch.object(self.window, "check_easyrpg_update", return_value=main.last_build_number), patch.object(self.window, "check_mkxpz_update", return_value=main.last_commit_sha), patch.object(self.window, "check_RTP_installed", return_value=True), patch.object(self.window, "check_soundfont_installed", return_value=True), patch.object(self.window, "check_kawariki_installed", return_value=True), patch.object(self.window, "get_rtp_value", return_value=("RTP", 1)):
            self.window.launch_mkxpz_game(str(game))
            self.window.launch_easyrpg_game(str(game))
        self.assertEqual(popen.call_count, 2)
        for call in popen.call_args_list:
            self.assertEqual(call.args[0][:2], ["/usr/bin/arch", "-arm64"])

    def test_external_install_validates_before_replacing_old_bundle(self):
        for bundle, binary, install in [("EasyRPG Player.app", "EasyRPG Player", self.window.install_easyrpg), ("Z-universal.app", "Z-universal", self.window.install_mkxpz)]:
            runtime = executable(self.managed / bundle / "Contents/MacOS" / binary, fat())
            sentinel = runtime.parents[2] / "keep-save"
            sentinel.write_text("valuable")
            for data in (b"invalid zip", thin(X64), fat()):
                with self.subTest(bundle=bundle, data=data):
                    archive = io.BytesIO()
                    with zipfile.ZipFile(archive, "w") as zip_file:
                        zip_file.writestr(f"{bundle}/Contents/MacOS/{binary}", data)
                    payload = b"invalid zip" if data == b"invalid zip" else archive.getvalue()
                    response = Mock(status_code=200, headers={"content-length": str(len(payload))})
                    response.iter_content.return_value = [payload]
                    with patch.object(main.requests, "get", return_value=response):
                        install()
                    self.assertEqual(runtime.read_bytes(), fat())
                    if data != fat():
                        self.assertEqual(sentinel.read_text(), "valuable")
                    else:
                        self.assertFalse(sentinel.exists())
            self.assertFalse(list(self.managed.glob(".runtime-stage-*")))

    def test_external_switch_failure_restores_previous_bundle(self):
        old = self.managed / "EasyRPG Player.app"
        executable(old / "Contents/MacOS/EasyRPG Player", fat())
        sentinel = old / "keep-save"
        sentinel.write_text("valuable")
        staged = self.root / "staged.app"
        executable(staged / "Contents/MacOS/EasyRPG Player", thin())
        rename = os.rename
        def fail_publish(src, dst):
            if Path(src) == staged:
                raise OSError("simulated publish failure")
            return rename(src, dst)
        with patch.object(main.os, "rename", side_effect=fail_publish), self.assertRaises(OSError):
            self.window._publish_runtime(staged, old, "EasyRPG Player")
        self.assertEqual(sentinel.read_text(), "valuable")
        self.assertEqual((old / "Contents/MacOS/EasyRPG Player").read_bytes(), fat())

    def test_failed_easyrpg_update_keeps_and_launches_old_arm_runtime(self):
        runtime = executable(self.managed / "EasyRPG Player.app/Contents/MacOS/EasyRPG Player", fat())
        sentinel = runtime.parents[2] / "save"
        sentinel.write_text("keep")
        with patch.object(self.window, "check_easyrpg_update", return_value=main.last_build_number + 1), patch.object(QMessageBox, "question", return_value=QMessageBox.Yes), patch.object(main.requests, "get", side_effect=main.requests.ConnectionError("offline")), patch.object(main.subprocess, "Popen", side_effect=lambda *a, **k: k["stdout"].close()) as launch, patch.object(main, "LOG_FILE", str(self.root / "game.log")):
            self.window.launch_easyrpg_game(str(self.root / "game"))
        self.assertEqual(runtime.read_bytes(), fat())
        self.assertEqual(sentinel.read_text(), "keep")
        self.assertEqual(launch.call_args.args[0][:3], ["/usr/bin/arch", "-arm64", str(runtime)])

    def test_failed_restore_retains_recovery_copy_and_refuses_next_install(self):
        old = self.managed / "EasyRPG Player.app"
        executable(old / "Contents/MacOS/EasyRPG Player", fat())
        staged = self.root / "staged.app"
        executable(staged / "Contents/MacOS/EasyRPG Player", thin())
        backup = self.managed / ".EasyRPG Player.app.previous"
        rename = os.rename
        def fail_publish_and_restore(src, dst):
            if Path(src) in (staged, backup):
                raise OSError("simulated failure")
            return rename(src, dst)
        with patch.object(main.os, "rename", side_effect=fail_publish_and_restore), self.assertRaises(OSError):
            self.window._publish_runtime(staged, old, "EasyRPG Player")
        self.assertEqual((backup / "Contents/MacOS/EasyRPG Player").read_bytes(), fat())
        with self.assertRaisesRegex(ValueError, "recovery"):
            self.window._publish_runtime(staged, old, "EasyRPG Player")
        self.assertTrue(backup.is_dir())

    def test_wrong_complete_path_does_not_refresh(self):
        self.install_script()
        self.helper.write_text(self.helper.read_text().replace("str(app.resolve())", "'/wrong/app'"))
        self.window.install_nwjs()
        until(lambda: not self.window._nwjs_busy)
        self.window.update_version_selector.assert_not_called()
        self.window.save_settings.assert_not_called()

    def test_uninstall_respects_lock_and_all_keeps_lock_inode(self):
        self.install_script()
        self.window.install_nwjs()
        until(lambda: not self.window._nwjs_busy)
        with installation_lock(self.managed):
            inode = (self.managed / ".nwjs-install.lock").stat().st_ino
            self.window.uninstall_nwjs()
            self.assertTrue((self.managed / "v1.2.3").exists())
            with patch.object(QMessageBox, "question", return_value=QMessageBox.Yes):
                self.window.uninstall_all()
            self.assertTrue((self.managed / "v1.2.3").exists())
        with patch.object(QMessageBox, "question", return_value=QMessageBox.Yes):
            self.window.uninstall_all()
        self.assertFalse((self.managed / "v1.2.3").exists())
        self.assertEqual((self.managed / ".nwjs-install.lock").stat().st_ino, inode)


if __name__ == "__main__":
    unittest.main()

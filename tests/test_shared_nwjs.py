import os
import json
import sys
from pathlib import Path
import plistlib
import subprocess
import tempfile
import unittest

from shared_nwjs import create_shared_app


class SharedAppTests(unittest.TestCase):
    def test_launch_with_quoted_paths_and_independent_game(self):
        with tempfile.TemporaryDirectory(prefix="quote ' $ ") as temp:
            root = Path(temp)
            runtime = root / "nw ' $ `runtime`"
            output = root / 'arguments'
            runtime.write_text('#!/bin/sh\nprintf "%s" "$1" > ' + __import__('shlex').quote(str(output)) + '\n')
            runtime.chmod(0o755)
            app = root / "Game ' $ `name`.app"
            game = Path(create_shared_app(str(app), "Game ' $ `name`", str(runtime), False))
            (game / 'package.json').write_text('{}')
            launch = app / 'Contents/MacOS/launch'
            subprocess.run([str(launch)], check=True)
            self.assertEqual(Path(output.read_text()).resolve(), game.resolve())
            with (app / 'Contents/Info.plist').open('rb') as f:
                self.assertEqual(plistlib.load(f)['CFBundleExecutable'], 'launch')
            self.assertEqual((game / 'package.json').read_text(), '{}')

    def test_runtime_icon_and_invalid_fallbacks(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            contents = root / 'nwjs.app/Contents'
            runtime = contents / 'MacOS/nwjs'
            runtime.parent.mkdir(parents=True)
            runtime.write_text('')
            runtime.chmod(0o755)
            resources = contents / 'Resources'
            resources.mkdir()
            (resources / 'nw.icns').write_bytes(b'icon')
            (root / 'outside.icns').write_bytes(b'outside')
            (resources / 'link.icns').symlink_to(root / 'outside.icns')
            cases = [('nw', True), ('nw.icns', True), ('missing', False),
                     ('../outside.icns', False), ('/tmp/icon.icns', False),
                     ('link.icns', False), (42, False), ('bad\\name', False)]
            for index, (icon, expected) in enumerate(cases):
                with self.subTest(icon=icon):
                    with (contents / 'Info.plist').open('wb') as f:
                        plistlib.dump({'CFBundleIconFile': icon}, f)
                    app = root / f'Game{index}.app'
                    create_shared_app(str(app), 'Game', str(runtime), False)
                    with (app / 'Contents/Info.plist').open('rb') as f:
                        info = plistlib.load(f)
                    if expected:
                        self.assertEqual((app / 'Contents/Resources' / info['CFBundleIconFile']).read_bytes(), b'icon')
                    else:
                        self.assertNotIn('CFBundleIconFile', info)
                    self.assertFalse((app / 'Contents/MacOS/nwjs').exists())
            for index, data in enumerate((b'not a plist', b'<?xml version="1.0"?><plist><dict>')):
                (contents / 'Info.plist').write_bytes(data)
                app = root / f'Invalid{index}.app'
                create_shared_app(str(app), 'Game', str(runtime), False)
                with (app / 'Contents/Info.plist').open('rb') as f:
                    self.assertNotIn('CFBundleIconFile', plistlib.load(f))
            (contents / 'Info.plist').unlink()
            app = root / 'Missing.app'
            create_shared_app(str(app), 'Game', str(runtime), False)
            with (app / 'Contents/Info.plist').open('rb') as f:
                self.assertNotIn('CFBundleIconFile', plistlib.load(f))

    def test_rosetta_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            runtime = root / 'nwjs'
            runtime.write_text('')
            runtime.chmod(0o755)
            app = root / 'Game.app'
            create_shared_app(str(app), 'Game', str(runtime), True)
            self.assertIn('/usr/bin/arch -x86_64 "$runtime" "$game"', (app / 'Contents/MacOS/launch').read_text())
            with self.assertRaises(FileExistsError):
                create_shared_app(str(app), 'Game', str(runtime), False)
            self.assertTrue((app / 'Contents/Info.plist').exists())

    def test_selection_confirmation_cancel_and_missing(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            def install(version):
                path = root / version / 'nwjs.app/Contents/MacOS/nwjs'
                path.parent.mkdir(parents=True)
                path.write_text('#!/bin/sh\nprintf "%s" "$0" > "$RESULT"\n')
                path.chmod(0o755)
                return path
            original = install('v1')
            replacement = install('v2')
            app = root / 'Game.app'
            create_shared_app(str(app), 'Game', str(original), False)
            launch = app / 'Contents/MacOS/launch'
            defaults = root / 'defaults'
            defaults.write_text('#!/bin/sh\ncase "$3" in runtimeVersion) [ -f "$STATE" ] && command cat "$STATE";; skipConfirmation) printf "%s" "$SKIP";; esac\n')
            defaults.chmod(0o755)
            selector = root / 'osascript'
            selector.write_text('#!/bin/sh\nprintf "%s" "$6" >> "$CALLS"\n[ -n "$CHOOSE" ] || exit 1\nprintf "%s" "$CHOOSE" > "$STATE"\nprintf "%s" "$CHOOSE"\n')
            selector.chmod(0o755)
            arch = root / 'arch'
            arch.write_text('#!/bin/sh\nprintf "%s" "$1" > "$ARCH_RESULT"\nshift\nexec "$@"\n')
            arch.chmod(0o755)
            launch.write_text(launch.read_text().replace('/usr/bin/defaults', str(defaults)).replace('/usr/bin/osascript', str(selector)).replace('/usr/bin/arch', str(arch)))
            env = dict(os.environ, STATE=str(root / 'state'), CALLS=str(root / 'calls'), RESULT=str(root / 'result'), ARCH_RESULT=str(root / 'arch_result'), SKIP='1', CHOOSE='v2')
            subprocess.run([str(launch)], env=env, check=True)
            self.assertEqual((root / 'result').read_text(), str(original))
            self.assertFalse((root / 'calls').exists())
            original.unlink()
            (root / 'v2/run-with-rosetta').touch()
            subprocess.run([str(launch)], env=env, check=True)
            self.assertEqual((root / 'result').read_text(), str(replacement))
            self.assertEqual((root / 'arch_result').read_text(), '-x86_64')
            self.assertEqual((root / 'calls').read_text(), 'missing')
            subprocess.run([str(launch)], env=env, check=True)
            self.assertEqual((root / 'calls').read_text(), 'missing')
            env['SKIP'] = '0'
            subprocess.run([str(launch)], env=env, check=True)
            self.assertEqual((root / 'calls').read_text(), 'missingconfirm')
            env['CHOOSE'] = ''
            replacement.unlink()
            self.assertNotEqual(subprocess.run([str(launch)], env=env).returncode, 0)
            self.assertEqual((root / 'calls').read_text(), 'missingconfirmmissing')

    @unittest.skipUnless(sys.platform == 'darwin', 'requires macOS JavaScript for Automation')
    def test_only_installed_executable_versions_are_listed(self):
        script = (Path(__file__).resolve().parents[1] / 'shared_selector.js').read_text().split('function run(argv)')[0]
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            def listed():
                return json.loads(subprocess.check_output(['/usr/bin/osascript', '-l', 'JavaScript', '-e', script + '\nJSON.stringify(installedVersions(' + json.dumps(temp) + '))'], text=True))
            self.assertEqual(listed(), [])
            for name in ('v1', 'v2', 'other'):
                executable = root / name / 'nwjs.app/Contents/MacOS/nwjs'
                executable.parent.mkdir(parents=True)
                executable.write_text('')
                executable.chmod(0o755 if name != 'v2' else 0o644)
            self.assertEqual(listed(), ['v1'])

    def test_missing_runtime_and_invalid_name_leave_no_destination(self):
        with tempfile.TemporaryDirectory() as temp:
            app = Path(temp) / 'Game.app'
            with self.assertRaises(FileNotFoundError):
                create_shared_app(str(app), 'Game', str(Path(temp) / 'missing'), False)
            with self.assertRaises(ValueError):
                create_shared_app(str(app), '../Game', '/missing', False)
            self.assertFalse(app.exists())


if __name__ == '__main__':
    unittest.main()

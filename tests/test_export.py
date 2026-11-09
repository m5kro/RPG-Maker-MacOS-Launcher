import ast
import logging
import os
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch

from shared_nwjs import create_shared_app

# Load the actual export method without importing the GUI dependencies.
tree = ast.parse((Path(__file__).resolve().parents[1] / 'main.py').read_text())
method = next(n for n in next(n for n in tree.body if isinstance(n, ast.ClassDef) and n.name == 'RPGMLauncher').body if isinstance(n, ast.FunctionDef) and n.name == 'export_standalone_app')
namespace = {'os': os, 'shutil': shutil, 'logging': logging, 'create_shared_app': create_shared_app}
exec(compile(ast.fix_missing_locations(ast.Module(body=[method], type_ignores=[])), 'main.py', 'exec'), namespace)


class Checkbox:
    def __init__(self, checked=False): self.checked = checked
    def isChecked(self): return self.checked


class Progress:
    canceled = False
    def __init__(self, *args): pass
    def setWindowModality(self, *args): pass
    def setMinimumDuration(self, *args): pass
    def show(self): pass
    def close(self): pass
    def setValue(self, *args): pass
    def setLabelText(self, *args): pass
    def wasCanceled(self): return self.canceled


class Dialog:
    @staticmethod
    def information(*args): pass
    @staticmethod
    def warning(*args): pass
    @staticmethod
    def critical(*args): pass


class ExportTests(unittest.TestCase):
    def test_shared_copy_default_standalone_and_cancel(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = root / 'source'
            source.mkdir()
            (source / 'package.json').write_text('{}')
            runtime = root / 'Library/Application Support/RPGM-Launcher/v1/nwjs.app/Contents/MacOS/nwjs'
            runtime.parent.mkdir(parents=True)
            runtime.write_text('runtime')
            runtime.chmod(0o755)
            class Launcher:
                last_selected_folder = str(source)
                version_selector = type('Version', (), {'currentText': lambda self: 'v1'})()
                extract_checkbox = cheat_menu_checkbox = optimize_space_checkbox = Checkbox()
                shared_nwjs_checkbox = Checkbox(True)
                get_app_name = lambda self: ('Game', True)
                check_package_json = lambda self, path: True
                truncate_filename = lambda self, name: name
            launcher = Launcher()
            namespace.update({'QFileDialog': type('Files', (), {'getExistingDirectory': staticmethod(lambda *a: str(source))}),
                              'QProgressDialog': Progress, 'QMessageBox': Dialog,
                              'Qt': type('Qt', (), {'WindowModal': 1}),
                              'QApplication': type('App', (), {'processEvents': staticmethod(lambda: None)})})
            with patch.dict(os.environ, {'HOME': temp}):
                namespace['export_standalone_app'](launcher)
                app = source / 'Game.app'
                self.assertEqual((app / 'Contents/Resources/app.nw/package.json').read_text(), '{}')
                self.assertFalse((app / 'Contents/MacOS/nwjs').exists())
                namespace['export_standalone_app'](launcher)
                self.assertTrue(app.exists())
                shutil.rmtree(app)
                Progress.canceled = True
                namespace['export_standalone_app'](launcher)
                self.assertFalse(app.exists())
                Progress.canceled = False
                launcher.shared_nwjs_checkbox.checked = False
                namespace['export_standalone_app'](launcher)
                self.assertTrue((app / 'Contents/MacOS/nwjs').exists())


if __name__ == '__main__': unittest.main()

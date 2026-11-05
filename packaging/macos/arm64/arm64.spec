# -*- mode: python ; coding: utf-8 -*-
from pathlib import Path

ROOT = Path(SPECPATH).resolve().parents[2]

a = Analysis(
    [str(ROOT / 'src/python/main.py')],
    pathex=[str(ROOT / 'src/python')],
    binaries=[(str(ROOT / 'build/helpers/arm64/nwjs-helper'), '.')],
    datas=[(str(ROOT / 'packaging/macos/arm64/EasyRPG-Standalone'), '.'),
           (str(ROOT / 'src/javascript/bg.js'), '.'),
           (str(ROOT / 'src/javascript/disable-child.js'), '.'),
           (str(ROOT / 'src/javascript/disable-net.js'), '.')],
    hiddenimports=[],
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[],
    noarchive=False,
    optimize=0,
)
pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    [],
    exclude_binaries=True,
    name='RPG-Maker-Launcher',
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=True,
    console=False,
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch='arm64',
    codesign_identity=None,
    entitlements_file=None,
    icon=[str(ROOT / 'assets/icons/icon.icns')],
)
coll = COLLECT(
    exe,
    a.binaries,
    a.datas,
    strip=False,
    upx=True,
    upx_exclude=[],
    name='RPG-Maker-Launcher',
)
app = BUNDLE(
    coll,
    name='RPG-Maker-Launcher.app',
    icon=str(ROOT / 'assets/icons/icon.icns'),
    bundle_identifier=None,
    version='3.3.4',
)

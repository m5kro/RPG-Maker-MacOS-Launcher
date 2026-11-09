# -*- mode: python ; coding: utf-8 -*-


a = Analysis(
    ['main.py'],
    pathex=[],
    binaries=[],
    datas=[('Cheat_Menu.js', '.'), ('Cheat_Menu.css', '.'), ('EasyRPG-Standalone', '.'), ('bg.js', '.'), ('disable-child.js', '.'), ('disable-net.js', '.'), ('shared_selector.js', '.')],
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
    target_arch='x86_64',
    codesign_identity=None,
    entitlements_file=None,
    icon=['icon.icns'],
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
    icon='./icon.icns',
    bundle_identifier=None,
    version='3.3.4',
)

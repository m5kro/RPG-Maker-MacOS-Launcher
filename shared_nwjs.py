"""Create the small NW.js shared-runtime app shell (no Qt dependency)."""
import os
import plistlib
import shlex
import shutil
import uuid
from xml.parsers.expat import ExpatError


def create_shared_app(destination, name, runtime, rosetta):
    if name in ('.', '..') or '/' in name or '\0' in name:
        raise ValueError('Application name must be a filename, not a path.')
    if not os.path.isfile(runtime) or not os.access(runtime, os.X_OK):
        raise FileNotFoundError('The selected NW.js runtime is missing or not executable.')
    os.mkdir(destination)  # Never overwrite an existing destination.
    try:
        contents = os.path.join(destination, 'Contents')
        macos = os.path.join(contents, 'MacOS')
        game = os.path.join(contents, 'Resources', 'app.nw')
        os.makedirs(macos)
        os.makedirs(game)
        bundle_id = 'org.rpgmlauncher.shared.' + uuid.uuid4().hex
        info = {'CFBundleName': name, 'CFBundleDisplayName': name,
                'CFBundleIdentifier': bundle_id,
                'CFBundlePackageType': 'APPL', 'CFBundleExecutable': 'launch'}
        runtime_contents = os.path.dirname(os.path.dirname(runtime))
        resources = os.path.join(runtime_contents, 'Resources')
        icon_data = None
        try:
            with open(os.path.join(runtime_contents, 'Info.plist'), 'rb') as f:
                runtime_info = plistlib.load(f)
            icon = runtime_info.get('CFBundleIconFile') if isinstance(runtime_info, dict) else None
            if isinstance(icon, str) and icon not in ('', '.', '..') and not any(c in icon for c in '/\\\0'):
                if not os.path.splitext(icon)[1] and not os.path.isfile(os.path.join(resources, icon)):
                    icon += '.icns'
                source = os.path.realpath(os.path.join(resources, icon))
                if os.path.commonpath((os.path.realpath(resources), source)) == os.path.realpath(resources):
                    with open(source, 'rb') as f:
                        icon_data = f.read()
        except (OSError, ValueError, TypeError, OverflowError, ExpatError):
            pass
        if icon_data is not None:
            icon_name = 'runtime-icon' + (os.path.splitext(icon)[1] or '.icns')
            with open(os.path.join(contents, 'Resources', icon_name), 'wb') as f:
                f.write(icon_data)
            info['CFBundleIconFile'] = icon_name
        with open(os.path.join(contents, 'Info.plist'), 'wb') as f:
            plistlib.dump(info, f)
        shutil.copyfile(os.path.join(os.path.dirname(__file__), 'shared_selector.js'),
                        os.path.join(contents, 'Resources', 'shared_selector.js'))
        root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(runtime)))))
        script = os.path.join(macos, 'launch')
        with open(script, 'w', encoding='utf-8') as f:
            f.write('#!/bin/sh\n'
                    'root=' + shlex.quote(root) + '\n'
                    'bundle=' + shlex.quote(bundle_id) + '\n'
                    'runtime=' + shlex.quote(runtime) + '\n'
                    'rosetta=' + ('1' if rosetta else '0') + '\n'
                    'version=$(/usr/bin/defaults read "$bundle" runtimeVersion 2>/dev/null)\n'
                    'case "$version" in v* ) case "$version" in */*) version=;; esac;; *) version=;; esac\n'
                    'if [ -n "$version" ]; then\n'
                    '  runtime="$root/$version/nwjs.app/Contents/MacOS/nwjs"\n'
                    '  rosetta=0; [ ! -e "$root/$version/run-with-rosetta" ] || rosetta=1\n'
                    'fi\n'
                    'reason=\n'
                    'if [ ! -f "$runtime" ] || [ ! -x "$runtime" ]; then reason=missing\n'
                    'elif [ -n "$version" ] && [ "$(/usr/bin/defaults read "$bundle" skipConfirmation 2>/dev/null)" != 1 ]; then reason=confirm; fi\n'
                    'if [ -n "$reason" ]; then\n'
                    '  version=$(/usr/bin/osascript -l JavaScript "$(dirname "$0")/../Resources/shared_selector.js" "$root" "$bundle" "$reason") || exit 1\n'
                    '  [ -n "$version" ] || exit 1\n'
                    '  runtime="$root/$version/nwjs.app/Contents/MacOS/nwjs"\n'
                    '  rosetta=0; [ ! -e "$root/$version/run-with-rosetta" ] || rosetta=1\n'
                    'fi\n'
                    'if [ ! -f "$runtime" ] || [ ! -x "$runtime" ]; then\n'
                    '  /usr/bin/osascript -e \'display dialog "The selected NW.js runtime is missing. Install a version in RPG Maker Launcher and try again." with icon stop\'\n'
                    '  exit 1\n'
                    'fi\n'
                    'game="$(dirname "$0")/../Resources/app.nw"\n'
                    'if [ "$rosetta" = 1 ]; then exec /usr/bin/arch -x86_64 "$runtime" "$game"; fi\n'
                    'exec "$runtime" "$game"\n')
        os.chmod(script, 0o755)
        return game
    except BaseException:
        shutil.rmtree(destination, ignore_errors=True)
        raise

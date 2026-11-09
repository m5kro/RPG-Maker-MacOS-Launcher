ObjC.import('Cocoa');

function installedVersions(root) {
    var fm = $.NSFileManager.defaultManager;
    var dirs = ObjC.unwrap(fm.contentsOfDirectoryAtPathError(root, null)) || [];
    return dirs.map(ObjC.unwrap).filter(function (v) {
        var p = root + '/' + v + '/nwjs.app/Contents/MacOS/nwjs';
        var isDir = Ref();
        return /^v/.test(v) && fm.fileExistsAtPathIsDirectory(p, isDir) && !isDir[0] && fm.isExecutableFileAtPath(p);
    }).sort();
}

function run(argv) {
    var root = argv[0], bundle = argv[1], missing = argv[2] === 'missing';
    var versions = installedVersions(root);
    $.NSApplication.sharedApplication;
    $.NSApp.setActivationPolicy($.NSApplicationActivationPolicyAccessory);
    var alert = $.NSAlert.alloc.init;
    alert.messageText = missing ? 'NW.js runtime missing' : 'Confirm NW.js runtime';
    alert.informativeText = versions.length ?
        (missing ? 'The selected NW.js runtime is missing. Choose an installed version.' : 'Choose an installed NW.js version for this game.') :
        'No installed executable NW.js versions were found. Install a version using RPG Maker Launcher and try again.';
    alert.addButtonWithTitle('OK');
    alert.addButtonWithTitle('Cancel');
    var view = $.NSView.alloc.initWithFrame($.NSMakeRect(0, 0, 340, 60));
    var popup = $.NSPopUpButton.alloc.initWithFramePullsDown($.NSMakeRect(0, 30, 340, 26), false);
    versions.forEach(function (v) { popup.addItemWithTitle(v); });
    var defaults = $.NSUserDefaults.alloc.initWithSuiteName(bundle);
    var selected = ObjC.unwrap(defaults.stringForKey('runtimeVersion'));
    if (selected && versions.indexOf(selected) !== -1) popup.selectItemWithTitle(selected);
    var checkbox = $.NSButton.alloc.initWithFrame($.NSMakeRect(0, 0, 340, 24));
    checkbox.setButtonType($.NSSwitchButton);
    checkbox.title = 'Skip future confirmation while this version is installed';
    checkbox.state = defaults.boolForKey('skipConfirmation') ? $.NSControlStateValueOn : $.NSControlStateValueOff;
    view.addSubview(popup); view.addSubview(checkbox); alert.accessoryView = view;
    $.NSApp.activateIgnoringOtherApps(true);
    if (alert.runModal !== $.NSAlertFirstButtonReturn) return '';
    selected = ObjC.unwrap(popup.titleOfSelectedItem);
    if (!selected) return '';
    defaults.setObjectForKey(selected, 'runtimeVersion');
    defaults.setBoolForKey(checkbox.state === $.NSControlStateValueOn, 'skipConfirmation');
    defaults.synchronize;
    return selected;
}

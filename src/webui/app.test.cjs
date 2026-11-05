// Dependency-free bridge/DOM fault injection; native smoke covers real WebKit/DOM.
const {test} = require('node:test');
const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const vm = require('node:vm');
const source = readFileSync(__dirname + '/app.js', 'utf8').replace(/^import .*\n/, '');

for (const button of ['install-nw', 'install-component']) {
  for (const fault of ['refresh-error', 'missing-installed', 'valid']) {
    test(`${button}: ${fault}`, async () => {
      const nodes = new Map(), calls = [];
      const node = id => {
        if (!nodes.has(id)) nodes.set(id, {
          value: '', textContent: '', options: [], dataset: {}, open: false,
          querySelectorAll: () => [], setAttribute() {},
          replaceChildren(...items) { this.options = items; },
          add(item) { this.options.push(item); },
          showModal() { this.open = true; this.returnValue = 'ok'; },
          addEventListener(name, callback) { assert.equal(name, 'close'); this.open = false; callback(); },
        });
        return nodes.get(id);
      };
      let ran = false;
      const snapshot = () => ({Preferences: {Version: 'v0.93.0', Folder: '/old-game'}, Installed: ['v0.93.0'], State: {Busy: false}, Game: null, SettingsError: ''});
      const context = vm.createContext({
        document: {getElementById: node, querySelectorAll: () => [], addEventListener() {}, body: {setAttribute() {}}, documentElement: {dataset: {}}},
        window: {addEventListener() {}},
        Option: function(text, value) { this.text = text; this.value = value; this.dataset = {}; },
        Events: {On() {}},
        Call: {async ByName(name, ...args) {
          const method = name.split('.').pop(); calls.push(method);
          if (method === 'Run') { assert.equal(args[0].Component, 'v0.94.1'); ran = true; return {Path: '/root/v0.94.1/nwjs.app'}; }
          if (method === 'Snapshot') {
            if (ran && fault === 'refresh-error') throw new Error('snapshot failed');
            const view = snapshot();
            if (ran && fault === 'valid') { view.Installed.push('v0.94.1'); view.Preferences.Version = 'v0.94.1'; }
            return view;
          }
          if (method === 'SavePreferences') return;
          throw new Error('unexpected bridge call: ' + method);
        }},
      });
      vm.runInContext(source, context);
      await context.window.rpgmReady;
      node('version').value = node('component').value = 'v0.94.1';
      await node(button).onclick();
      assert.equal(calls.filter(x => x === 'Run').length, 1);
      assert(!calls.includes('SavePreferences'), 'install must not save selection before verification');
      if (fault === 'valid') {
        assert.match(node('status').textContent, /導入が完了しました/);
        assert.equal(node('version').value, 'v0.94.1');
      } else {
        assert.doesNotMatch(node('status').textContent, /導入が完了しました/);
        assert.match(node('status').textContent, fault === 'refresh-error' ? /snapshot failed/ : /導入済み一覧/);
        assert.equal(vm.runInContext('preferences.Version', context), 'v0.93.0', 'failed refresh keeps old preferences');
        assert.equal(node('version').value, 'v0.94.1', 'failed refresh keeps the current UI selection');
      }
    });
  }
}

// Dependency-free bridge/DOM fault injection; native smoke covers real WebKit/DOM.
const {test} = require('node:test');
const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const vm = require('node:vm');
const source = readFileSync(__dirname + '/app.js', 'utf8').replace(/^import .*\n/, '');

test('advanced string arrays accept unambiguous newline lists without weakening JSON validation', async () => {
  const nodes = new Map();
  const node = id => {
    if (!nodes.has(id)) nodes.set(id, {value: '', dataset: {}, options: [], querySelectorAll: () => [], setAttribute() {}, replaceChildren() {}, add() {}});
    return nodes.get(id);
  };
  const context = vm.createContext({
    document: {getElementById: node, querySelectorAll: () => [], addEventListener() {}, body: {setAttribute() {}}, documentElement: {dataset: {}}},
    window: {addEventListener() {}}, Option: function() {}, Events: {On() {}},
    Call: {async ByName() { return {Preferences: {}, Installed: [], State: {Busy: false}}; }},
  });
  vm.runInContext(source, context); await context.window.rpgmReady;
  const cases = [
    ['[]', ' alpha \r\n\r\nbeta gamma\n', '["alpha","beta gamma"]'],
    ['["old"]', '["new",\n"other"]', '["new",\n"other"]'],
    ['[]', '["broken",\n"other"', '["broken",\n"other"'],
    ['[]', 'true\nfalse', 'true\nfalse'],
    ['[1,2]', 'alpha\nbeta', 'alpha\nbeta'],
    ['[]', 'alpha', 'alpha'],
  ];
  for (const [original, input, expected] of cases) {
    context.field = {Type: 'array', ValueJSON: original, EnabledJSON: 'true'};
    vm.runInContext('fields = [field]', context);
    node('value-0').value = input;
    assert.equal(vm.runInContext('formValues()[0].ValueJSON', context), expected);
  }
});

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

for (const engine of ['MKXPZ', 'EasyRPG', 'NWJS']) {
  for (const scenario of ['current', 'accept', 'decline', 'offline', 'cancel', 'install-fail', 'missing', 'missing-decline', 'export']) {
    test(`launch update ${engine}: ${scenario}`, async () => {
      const nodes = new Map(), actions = [], prompts = [];
      const node = id => {
        if (!nodes.has(id)) nodes.set(id, {
          value: '', textContent: '', options: [], dataset: {}, open: false,
          querySelectorAll: () => [], setAttribute() {},
          replaceChildren(...items) { this.options = items; }, add(item) { this.options.push(item); },
          showModal() { this.open = true; if (id === 'confirm-dialog') { prompts.push(node('confirm-message').textContent); this.returnValue = ['decline', 'missing-decline'].includes(scenario) ? 'cancel' : 'ok'; } },
          addEventListener(name, callback) { assert.equal(name, 'close'); this.open = false; callback(); },
        });
        return nodes.get(id);
      };
      const context = vm.createContext({
        document: {getElementById: node, querySelectorAll: () => [], addEventListener() {}, body: {setAttribute() {}}, documentElement: {dataset: {}}},
        window: {addEventListener() {}},
        Option: function(text, value) { this.text = text; this.value = value; this.dataset = {}; },
        Events: {On() {}},
        Call: {async ByName(name, ...args) {
          const method = name.split('.').pop();
          if (method === 'Snapshot') return {Preferences: {Version: 'v0.93.0'}, Installed: ['v0.93.0'], State: {Busy: false}, Game: {Engine: engine, Path: '/game'}};
          if (method === 'SavePreferences' || method === 'CancelOperation') return;
          if (method !== 'Run') throw new Error('unexpected bridge call: ' + method);
          const request = args[0]; actions.push(request.Action);
          if (request.Action === 'missing') return {Missing: scenario.startsWith('missing') ? [engine] : []};
          if (request.Action === 'update') {
            assert.equal(request.Component, engine);
            if (scenario === 'cancel') { await node('cancel').onclick(); throw new Error('cancelled'); }
            if (scenario === 'offline') throw new Error('offline');
            return {Update: {Available: ['accept', 'decline', 'install-fail'].includes(scenario), Current: 'old', Latest: 'new'}};
          }
          if (request.Action === 'install') { assert.equal(request.Approved, true); if (scenario === 'install-fail') throw new Error('install failed'); return {}; }
          if (request.Action === 'launch') return {PID: 42};
          throw new Error('unexpected action: ' + request.Action);
        }},
      });
      vm.runInContext(source, context); await context.window.rpgmReady;
      await node(scenario === 'export' ? 'export' : 'launch').onclick();
      const checks = engine !== 'NWJS' && !scenario.startsWith('missing') && scenario !== 'export';
      assert.equal(actions.includes('update'), checks);
      const installed = scenario === 'missing' || (checks && ['accept', 'install-fail'].includes(scenario));
      assert.equal(actions.includes('install'), installed);
      assert.equal(prompts.length, scenario.startsWith('missing') || (checks && ['accept', 'decline', 'install-fail'].includes(scenario)) ? 1 : 0);
      const launched = scenario !== 'export' && scenario !== 'missing-decline' && !(checks && ['cancel', 'install-fail'].includes(scenario));
      assert.equal(actions.includes('launch'), launched);
      if (checks && scenario === 'offline') assert.match(node('status').textContent, /更新確認に失敗しました: offline.*起動しました/);
      if (checks && scenario === 'install-fail') assert.match(node('status').textContent, /install failed/);
    });
  }
}

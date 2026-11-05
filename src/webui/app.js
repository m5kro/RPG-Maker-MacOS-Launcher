import {Call, Events} from '/wails/runtime.js';

const $ = id => document.getElementById(id);
const call = (method, ...args) => Call.ByName(`rpgm-launcher/nwjs-helper/internal/ui.UI.${method}`, ...args);
let preferences = {}, installed = [], remote = [], game = null;
let state = {Busy: false}, pending = false, activeEngine = '', fields = [], destination = '';
const message = text => { $('status').textContent = text; };
const fail = error => {
  const text = String(error?.message || error);
  message(text);
  for (const id of ['advanced', 'export']) if ($(id + '-dialog').open) $(id + '-message').textContent = text;
};
function controls() {
  const busy = state.Busy || pending || state.Closing;
  document.querySelectorAll('[data-work]').forEach(el => { el.disabled = busy || el.dataset.readonly === 'true'; });
  $('launch').disabled = busy || !game;
  $('export').disabled = busy || !game;
  $('cancel').disabled = !state.Busy || state.Cancelling;
  $('progress').hidden = !state.Busy;
  document.body.setAttribute('aria-busy', String(busy));
}
async function perform(work) {
  if (pending) return;
  pending = true; controls();
  try { await work(); } catch (error) { fail(error); }
  finally { pending = false; controls(); }
}
function versionOptions() {
  const selected = $('version').value || preferences.Version || '';
  $('version').replaceChildren(new Option('版を選択', ''));
  const versions = [...new Set([...installed, selected, ...remote.map(v => v.version)])].filter(Boolean);
  for (const v of versions) $('version').add(new Option(v + (installed.includes(v) ? '（導入済み）' : ''), v));
  $('version').value = selected;
  const old = $('component').value;
  $('component').querySelectorAll('[data-nw]').forEach(el => el.remove());
  for (const v of installed) { const option = new Option('NW.js ' + v, v); option.dataset.nw = 'true'; $('component').add(option); }
  if ([...$('component').options].some(o => o.value === old)) $('component').value = old;
}
async function refresh(expectedNW = '') {
  const view = await call('Snapshot');
  if (expectedNW && !view.Installed?.includes(expectedNW)) throw new Error(`${expectedNW} が導入済み一覧にありません。導入結果を確認してください。`);
  preferences = view.Preferences; installed = view.Installed || []; game = view.Game; state = view.State;
  $('version').value = preferences.Version;
  versionOptions(); $('version').value = preferences.Version;
  $('disable-protection').checked = preferences.DisableProtection;
  $('patch-getpixel').checked = preferences.PatchGetPixel;
  $('folder').textContent = game?.Path || preferences.Folder || 'フォルダ未選択';
  $('engine').textContent = game ? 'エンジン: ' + game.Engine : 'エンジン未判定';
  $('recovery').textContent = view.SettingsError;
  controls();
}
async function savePreferences() {
  const p = {...preferences, Version: $('version').value, DisableProtection: $('disable-protection').checked, PatchGetPixel: $('patch-getpixel').checked};
  await call('SavePreferences', p); preferences = p;
}
function confirm(text) {
  const dialog = $('confirm-dialog');
  $('confirm-message').textContent = text;
  dialog.returnValue = 'cancel'; dialog.showModal();
  return new Promise(resolve => dialog.addEventListener('close', () => resolve(dialog.returnValue === 'ok'), {once: true}));
}
const run = request => call('Run', request);
async function install(component) {
  await run({Action: 'install', Component: component, Approved: true});
  await refresh(/^v\d+\.\d+\.\d+$/.test(component) ? component : '');
}
async function readyGame() {
  await savePreferences();
  const result = await run({Action: 'missing'});
  if (result.Missing?.length) {
    if (!await confirm('次のコンポーネントが必要です。ダウンロード / 修復しますか？\n' + result.Missing.join('\n'))) return false;
    for (const component of result.Missing) await install(component);
  }
  return true;
}
async function logs() { $('log').textContent = await call('Logs'); }

// Do not allow dropped URLs, links or forms to navigate the privileged WebView.
document.addEventListener('click', e => { if (e.target.closest('a')) e.preventDefault(); });
document.addEventListener('dragover', e => e.preventDefault());
document.addEventListener('drop', e => e.preventDefault());
document.addEventListener('submit', e => { if (e.target.method !== 'dialog') e.preventDefault(); });
Events.On('launcher:state', e => {
  state = e.data; controls();
  if (state.Busy) message(state.Cancelling ? 'キャンセル中 — 後始末を待っています…' : '処理中: ' + state.Operation);
  else if (/^(処理中: |キャンセル中 — )/.test($('status').textContent)) message('待機中');
});
Events.On('launcher:progress', e => {
  const p = e.data; message(`${p.Phase}: ${p.Completed} / ${p.Total || '不明'}`);
  if (p.Total > 0) { $('progress').max = p.Total; $('progress').value = p.Completed; } else $('progress').removeAttribute('value');
});
Events.On('launcher:log', () => { if ($('log-panel').open) logs().catch(fail); });
Events.On('launcher:game-exit', e => { message(`ゲーム終了 PID ${e.data.PID} / code ${e.data.Code}`); logs().catch(fail); });
$('cancel').onclick = () => call('CancelOperation').catch(fail);
$('quit').onclick = () => call('RequestQuit').catch(fail);
$('pick-game').onclick = () => perform(async () => {
  const path = await call('PickFolder');
  if (path) { await call('SelectFolder', path); await refresh(); message('ゲームフォルダを選択しました'); }
});
for (const id of ['version', 'disable-protection', 'patch-getpixel']) $(id).onchange = () => perform(async () => { await savePreferences(); message('設定を保存しました'); });
$('versions').onclick = () => perform(async () => { remote = (await run({Action: 'versions'})).Versions || []; versionOptions(); message('ARM 対応の公開版を取得しました'); });
$('install-nw').onclick = () => perform(async () => {
  const version = $('version').value; if (!version) throw new Error('NW.js の版を選択してください');
  if (await confirm(`${version} を導入しますか？ 未導入の場合は公式配布からダウンロードします。`)) { await install(version); message('NW.js の導入が完了しました'); }
});
$('install-component').onclick = () => perform(async () => {
  const component = $('component').value;
  if (await confirm(`${component} をダウンロードして導入 / 再導入しますか？`)) { await install(component); message('導入が完了しました'); }
});
$('update').onclick = () => perform(async () => {
  const component = $('component').value;
  if (!['MKXPZ', 'EasyRPG'].includes(component)) { message('NW.js は「公開版を取得」で確認してください。RTP / SoundFont / Kawariki は必要なときに再導入できます。'); return; }
  const update = (await run({Action: 'update', Component: component})).Update;
  if (!update.Available) { message('最新版です: ' + update.Current); return; }
  if (await confirm(`${component}\n現在: ${update.Current || '未記録'}\n新しい版: ${update.Latest}\nダウンロードして更新しますか？`)) { await install(component); message('更新が完了しました'); }
});
$('uninstall').onclick = () => perform(async () => {
  const component = $('component').value;
  const targets = component === 'MKXPZ' ? 'MKXP-Z と関連する RTP / SoundFont / Kawariki' : component;
  if (await confirm(`${targets} を削除しますか？ ゲーム原本・セーブは削除しません。`)) { await run({Action: 'uninstall', Component: component, Approved: true}); await refresh(); message('削除しました'); }
});
$('uninstall-all').onclick = () => perform(async () => {
  if (!await confirm('管理対象のすべてのコンポーネントと設定を削除しますか？ ゲーム原本・セーブ・未知のデータは削除しません。')) return;
  try { await run({Action: 'uninstall-all', Approved: true}); message('削除しました'); }
  finally { await refresh(); }
});
$('launch').onclick = () => perform(async () => {
  if (!await readyGame()) return;
  const result = await run({Action: 'launch'}); message(`起動しました PID ${result.PID}。ランチャーを終了してもゲームは継続します。`);
});
$('export').onclick = () => perform(async () => {
  if (!await readyGame()) return;
  $('app-name').value = game.Path.split('/').pop(); destination = ''; $('destination').textContent = '出力先未選択'; $('export-message').textContent = '';
  $('export-dialog').showModal();
});
$('pick-export').onclick = () => perform(async () => { const path = await call('PickFolder'); if (path) { destination = path; $('destination').textContent = path; } });
$('close-export').onclick = () => $('export-dialog').close();
$('export-form').onsubmit = e => { e.preventDefault(); perform(async () => {
  if (!destination) throw new Error('出力先を選択してください');
  const result = await run({Action: 'export', Destination: destination, Name: $('app-name').value, Approved: true});
  $('export-dialog').close(); message('書き出しました: ' + result.Path);
}); };

function renderFields(next) {
  fields = next; $('fields').replaceChildren();
  fields.forEach((field, i) => {
    const group = document.createElement('fieldset'); group.dataset.key = field.Key;
    const legend = document.createElement('legend'); legend.textContent = field.Key + (field.Forced ? ' — Force（必須）' : '') + (field.ReadOnly ? ' — ゲーム選択で管理' : ''); group.append(legend);
    const enabled = document.createElement('input'); enabled.type = 'checkbox'; enabled.id = 'enabled-' + i;
    enabled.checked = ['true', '"True"', '"Force"'].includes(field.EnabledJSON);
    enabled.dataset.work = ''; enabled.dataset.readonly = String(field.Forced || field.ReadOnly);
    const elabel = document.createElement('label'); elabel.htmlFor = enabled.id; elabel.textContent = 'Enabled（有効）'; elabel.prepend(enabled); group.append(elabel);
    const value = document.createElement(['array', 'object'].includes(field.Type) ? 'textarea' : 'input');
    value.id = 'value-' + i; value.dataset.work = ''; value.dataset.readonly = String(field.ReadOnly);
    if (field.Type === 'boolean') { value.type = 'checkbox'; value.checked = field.ValueJSON === 'true'; }
    else if (field.Type === 'string') { value.value = JSON.parse(field.ValueJSON); }
    else { value.value = field.ValueJSON; }
    const label = document.createElement('label'); label.htmlFor = value.id; label.textContent = '値（' + field.Type + '）'; group.append(label, value); $('fields').append(group);
  }); controls();
}
function formValues() {
  return fields.map((field, i) => {
    const value = $('value-' + i), enabled = $('enabled-' + i);
    let enabledJSON = field.EnabledJSON;
    if (!field.Forced && !field.ReadOnly) {
      if (enabledJSON === '"True"' || enabledJSON === '"False"') enabledJSON = enabled.checked ? '"True"' : '"False"';
      else if (enabledJSON !== 'null' || enabled.checked) enabledJSON = String(enabled.checked);
    }
    // Carry Go's per-field Reset intent; ordinary saves never infer a type change.
    return {...field, Reset: field.Reset === true, EnabledJSON: enabledJSON, ValueJSON: field.ReadOnly ? field.ValueJSON : field.Type === 'string' ? JSON.stringify(value.value) : field.Type === 'boolean' ? String(value.checked) : value.value};
  });
}
async function openAdvanced(engine) {
  activeEngine = engine; renderFields(await call('Advanced', engine, false)); $('advanced-title').textContent = engine + ' 詳細設定'; $('advanced-message').textContent = ''; $('advanced-dialog').showModal();
}
$('mkxp-settings').onclick = () => perform(() => openAdvanced('MKXPZ'));
$('easy-settings').onclick = () => perform(() => openAdvanced('EasyRPG'));
$('close-advanced').onclick = () => $('advanced-dialog').close();
$('reset-advanced').onclick = () => perform(async () => { renderFields(await call('Advanced', activeEngine, true)); $('advanced-message').textContent = '初期値に戻しました。保存するまで反映されません。'; });
$('advanced-form').onsubmit = e => { e.preventDefault(); perform(async () => { await call('SaveAdvanced', activeEngine, formValues()); fields.forEach(field => { field.Reset = false; }); $('advanced-message').textContent = '保存しました'; }); };
$('refresh-log').onclick = () => logs().catch(fail);
$('log-panel').ontoggle = () => { if ($('log-panel').open) logs().catch(fail); };
window.addEventListener('error', e => fail(e.error || e.message));
window.addEventListener('unhandledrejection', e => fail(e.reason));
window.rpgmReady = refresh().then(() => { message('待機中 — 導入済みの版はオフラインで利用できます'); document.documentElement.dataset.ready = 'true'; }).catch(error => { fail(error); throw error; });

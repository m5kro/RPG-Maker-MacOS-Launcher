// Run with: node src/go/cmd/benchmark-mz/benchmark_test.js
const vm = require('vm');
const fs = require('fs');
const path = require('path');
const assert = require('assert');
let tick, result, focused = true, quit = false, focusCalls = 0;
const context = {
  window: {__mzBenchmark: {warmup: 1, duration: 1, result: 'result'}, SceneManager: {}, Graphics: {}},
  SceneManager: {_scene: {isStarted: () => true, constructor: {name: 'Scene_Map'}}, isGameActive: () => focused},
  Graphics: {app: {}},
  nw: {Window: {get: () => ({focus: () => focusCalls++})}, App: {quit: () => quit = true}},
  requestAnimationFrame: f => tick = f,
  process: {versions: {}, platform: 'test', arch: 'test'}, navigator: {userAgent: ''},
  screen: {width: 1, height: 1}, devicePixelRatio: 1,
  require: () => ({writeFileSync: (p, s) => result = JSON.parse(s)})
};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'benchmark.js'), 'utf8'), context);
tick(0); tick(1000); tick(1500);
focused = false; tick(1600);
focused = true; tick(1700); tick(2700); tick(3200);
assert(!result);
tick(3700);
assert(quit);
assert.equal(result.frame_count, 2);
assert.equal(result.elapsed_seconds, 1);
assert.equal(focusCalls, 1);

// Fixed workloads must not start timing on a title or other non-map scene.
result = undefined; quit = false;
context.window.__mzBenchmark.fixed = true;
context.SceneManager._scene.constructor.name = 'Scene_Title';
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'benchmark.js'), 'utf8'), context);
tick(0); tick(2000); tick(4000);
assert(!result);
context.SceneManager._scene.constructor.name = 'Scene_Map';
tick(5000); tick(6000); tick(7000);
assert(result);
assert(quit);

// Nested timings are inclusive and reset after focus loss.
let clock = 0;
context.performance = {now: () => clock};
context.window.Game_Map = function() {};
context.window.Game_Map.prototype.update = function(value) { clock += 2; return value; };
const map = new context.window.Game_Map();
context.window.SceneManager = context.SceneManager;
context.SceneManager.updateScene = function(value) { clock++; return map.update(value); };
context.Graphics.app.render = function() { clock += 4; };
context.window.__mzBenchmark.fixed = false;
result = undefined;
const sceneManager = context.window.SceneManager;
const gameMap = context.window.Game_Map;
delete context.window.SceneManager;
delete context.window.Game_Map;
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'benchmark.js'), 'utf8'), context);
context.window.SceneManager = sceneManager;
context.window.Game_Map = gameMap;
tick(0); tick(1000);
assert.equal(context.SceneManager.updateScene(42), 42);
focused = false; tick(1100);
focused = true; tick(1200); tick(2200);
context.SceneManager.updateScene(7); context.Graphics.app.render();
tick(3200);
assert.equal(result.cpu_sections.scene_update.count, 1);
assert.equal(result.cpu_sections.scene_update.total_ms, 3);
assert.equal(result.cpu_sections.map_update.mean_ms, 2);
assert.equal(result.cpu_sections.map_update.p95_ms, 2);
assert.equal(result.cpu_sections.render_call.total_ms, 4);

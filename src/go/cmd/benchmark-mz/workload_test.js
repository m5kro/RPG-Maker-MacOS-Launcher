// Run with: node src/go/cmd/benchmark-mz/workload_test.js
const vm = require('vm');
const fs = require('fs');
const path = require('path');
const assert = require('assert');
const context = {
  DataManager: {onLoad: map => { map.meta = {}; }, setupNewGame: () => {}},
  Scene_Boot: function() {}, Scene_Map: function() {},
  SceneManager: {goto: scene => { context.scene = scene; }},
  $dataSystem: {}, $dataMap: null
};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'workload.js'), 'utf8'), context);
context.Scene_Boot.prototype.startNormalGame();
assert.equal(context.scene, context.Scene_Map);
assert.equal(context.$dataSystem.startMapId, 1);
context.DataManager.loadMapData(1);
assert.equal(context.$dataMap.data.length, 40 * 30 * 6);
assert.equal(context.$dataMap.events.length, 201);
assert.equal(context.$dataMap.events[1].pages[0].moveType, 3);

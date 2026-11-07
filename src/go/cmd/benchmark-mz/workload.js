/* Fixed synthetic MZ map; tile graphics come from the supplied game's tileset 1. */
(() => {
  const width = 40, height = 30;
  const data = Array(width * height * 6).fill(0);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) data[y * width + x] = 2048 + (x + y) % 4;
  }
  const events = [null];
  for (let i = 1; i <= 200; i++) {
    events.push({id: i, name: `Benchmark ${i}`, note: '', x: i % 40, y: Math.floor(i / 40), pages: [{
      conditions: {actorId: 1, actorValid: false, itemId: 1, itemValid: false, selfSwitchCh: 'A', selfSwitchValid: false, switch1Id: 1, switch1Valid: false, switch2Id: 1, switch2Valid: false, variableId: 1, variableValid: false, variableValue: 0},
      directionFix: false, image: {tileId: 0, characterName: '', characterIndex: 0, direction: 2, pattern: 1},
      list: [{code: 0, indent: 0, parameters: []}], moveFrequency: 5, moveRoute: {list: [{code: 1, parameters: []}, {code: 2, parameters: []}, {code: 0, parameters: []}], repeat: true, skippable: true, wait: false},
      moveSpeed: 4, moveType: 3, priorityType: 1, stepAnime: false, through: true, trigger: 0, walkAnime: true
    }]});
  }
  DataManager.loadMapData = function() {
    $dataMap = {autoplayBgm: false, autoplayBgs: false, battleback1Name: '', battleback2Name: '', bgm: {name: '', pan: 0, pitch: 100, volume: 90}, bgs: {name: '', pan: 0, pitch: 100, volume: 90}, disableDashing: false, displayName: '', encounterList: [], encounterStep: 30, height, note: '', parallaxLoopX: false, parallaxLoopY: false, parallaxName: '', parallaxShow: false, parallaxSx: 0, parallaxSy: 0, scrollType: 0, specifyBattleback: false, tilesetId: 1, width, data, events};
    this.onLoad($dataMap);
  };
  Scene_Boot.prototype.startNormalGame = function() {
    $dataSystem.startMapId = 1;
    $dataSystem.startX = 20;
    $dataSystem.startY = 15;
    DataManager.setupNewGame();
    SceneManager.goto(Scene_Map);
  };
})();

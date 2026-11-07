(() => {
  const config = window.__mzBenchmark;
  let readyAt, start, previous, sceneStart;
  const frames = [];
  const samples = {scene_update: [], map_update: [], render_call: []};
  function wrap(object, method, name) {
    if (!object || typeof object[method] !== 'function') return false;
    const original = object[method];
    object[method] = function(...args) {
      if (start === undefined) return original.apply(this, args);
      const began = performance.now();
      try { return original.apply(this, args); }
      finally { samples[name].push(performance.now() - began); }
    };
    return true;
  }
  const available = {scene_update: false, map_update: false, render_call: false};
  let renderApp;
  nw.Window.get().focus();
  function tick(now) {
    if (!window.SceneManager || !SceneManager._scene || !window.Graphics || !Graphics.app ||
        ['Scene_Boot', 'Scene_Splash'].includes(SceneManager._scene.constructor.name) ||
        !SceneManager._scene.isStarted() || !SceneManager.isGameActive() ||
        (config.fixed && SceneManager._scene.constructor.name !== 'Scene_Map')) {
      readyAt = start = previous = undefined;
      frames.length = 0;
      for (const values of Object.values(samples)) values.length = 0;
      requestAnimationFrame(tick);
      return;
    }
    if (!available.scene_update) available.scene_update = wrap(window.SceneManager, 'updateScene', 'scene_update');
    if (!available.map_update) available.map_update = wrap(window.Game_Map && window.Game_Map.prototype, 'update', 'map_update');
    if (renderApp !== Graphics.app) {
      renderApp = Graphics.app;
      available.render_call = wrap(renderApp, 'render', 'render_call');
    }
    if (readyAt === undefined) readyAt = now;
    if (start === undefined && now - readyAt >= config.warmup * 1000) {
      start = previous = now;
      sceneStart = SceneManager._scene.constructor.name;
    } else if (start !== undefined) {
      frames.push(now - previous);
      previous = now;
      if (now - start >= config.duration * 1000) {
        const sorted = frames.slice().sort((a, b) => a - b);
        const percentile = p => sorted[Math.ceil(sorted.length * p) - 1];
        const elapsed = now - start;
        const result = {
          measurement: 'requestAnimationFrame intervals; includes scheduling and vsync',
          warmup_seconds: config.warmup,
          requested_seconds: config.duration,
          elapsed_seconds: elapsed / 1000,
          frame_count: frames.length,
          fps: frames.length * 1000 / elapsed,
          frame_time_ms: {mean: elapsed / frames.length, p50: percentile(0.50), p95: percentile(0.95), p99: percentile(0.99), max: sorted[sorted.length - 1]},
          cpu_sections: Object.fromEntries(Object.entries(samples).map(([name, values]) => {
            const sorted = values.slice().sort((a, b) => a - b);
            return [name, {
              available: available[name], count: values.length,
              total_ms: values.reduce((sum, value) => sum + value, 0),
              mean_ms: values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : null,
              p95_ms: values.length ? sorted[Math.ceil(values.length * 0.95) - 1] : null
            }];
          })),
          cpu_sections_note: 'Synchronous wall-clock durations around calls, not CPU accounting; nested sections overlap. Instrumentation adds overhead; GPU work and asynchronous completion are not measured.',
          scene_start: sceneStart,
          scene_end: SceneManager._scene.constructor.name,
          environment: {versions: process.versions, platform: process.platform, arch: process.arch, user_agent: navigator.userAgent, mz: window.Utils && Utils.RPGMAKER_VERSION, screen: {width: screen.width, height: screen.height, device_pixel_ratio: devicePixelRatio}}
        };
        require('fs').writeFileSync(config.result, JSON.stringify(result, null, 2) + '\n', {flag: 'wx'});
        nw.App.quit();
        return;
      }
    }
    requestAnimationFrame(tick);
  }
  requestAnimationFrame(tick);
})();

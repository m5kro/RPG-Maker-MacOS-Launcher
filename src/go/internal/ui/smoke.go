//go:build smoke

package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"rpgm-launcher/nwjs-helper/internal/launcher"
)

// Smoke is registered only by a smoke build, against an explicitly owned root.
// No production binding can write an evidence file or create a test job.
type Smoke struct {
	ui      *UI
	app     *application.App
	window  *application.WebviewWindow
	path    string
	mu      sync.Mutex
	stage   string
	release chan struct{}
	result  error
	once    sync.Once
	done    chan struct{}
}

func configureSmoke(u *UI, opts *application.Options) (func(*application.App, *application.WebviewWindow), func() error, error) {
	path := os.Getenv("RPGM_GUI_SMOKE_RESULT")
	if path == "" {
		return func(*application.App, *application.WebviewWindow) {}, func() error { return nil }, nil
	}
	if !filepath.IsAbs(path) {
		return nil, nil, errors.New("smoke result must be absolute")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return nil, nil, errors.New("smoke result must not exist")
	}
	marker, err := os.ReadFile(filepath.Join(u.manager.Root, ".gui-smoke-owned"))
	if err != nil || string(marker) != "owned\n" {
		return nil, nil, errors.New("smoke root lacks ownership marker")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	parent := filepath.Dir(u.manager.Root)
	rel, err := filepath.Rel(parent, home)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return nil, nil, errors.New("smoke requires an isolated sibling HOME")
	}
	if _, err := launcher.RequireNWArm(filepath.Join(u.manager.Root, "v0.94.1")); err != nil {
		return nil, nil, fmt.Errorf("smoke requires a copied ARM v0.94.1: %w", err)
	}
	s := &Smoke{ui: u, path: path, done: make(chan struct{}), result: errors.New("smoke did not complete")}
	opts.Services = append(opts.Services, application.NewService(s))
	// Cocoa terminate exits without returning from App.Run; enforce failure here.
	opts.PostShutdown = func() {
		s.complete(`{"ok":false,"error":"window closed before smoke completed"}`)
		s.mu.Lock()
		err := s.result
		s.mu.Unlock()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	return func(app *application.App, w *application.WebviewWindow) {
		s.app = app
		s.window = w
		w.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) { w.ExecJS(nativeSmokeJS) })
		go func() {
			select {
			case <-s.done:
				return
			case <-time.After(50 * time.Second):
				s.complete(`{"ok":false,"error":"native WebKit smoke timeout (no ready/bridge/result)"}`)
				os.Exit(1)
			}
		}()
	}, func() error { s.mu.Lock(); defer s.mu.Unlock(); return s.result }, nil
}
func (s *Smoke) Foreground() {
	s.window.Show()
	s.window.Focus()
}
func (s *Smoke) Seed() error {
	return s.ui.operation("smoke seed", func(context.Context) error {
		a := launcher.DefaultAdvancedSettings()
		a.MKXPZ["JITMaxCache"] = json.Number("9007199254740993123456789")
		a.MKXPZ["windowTitle"] = "日本語 001984 <img src=x onerror=alert(1)>"
		a.MKXPZ["solidFonts"] = []any{"123", json.Number("9007199254740993123")}
		a.MKXPZ["bindingNames"] = map[string]any{"日本語": json.Number("9007199254740993123")}
		a.MKXPZEnabled["windowTitle"] = "True"
		a.MKXPZEnabled["JITMaxCache"] = true
		if err := launcher.SaveAdvancedSettings(s.ui.manager.Root, a); err != nil {
			return err
		}
		s.ui.log("日本語ログ: 型を丸めず保存")
		return nil
	})
}
func (s *Smoke) SeedLegacy() error {
	return s.ui.operation("smoke legacy seed", func(context.Context) error {
		a := launcher.DefaultAdvancedSettings()
		a.MKXPZ["windowTitle"] = json.Number("1984")
		a.EasyRPG["--seed"] = json.Number("123")
		a.EasyRPG["--project-path"] = "/日本語/game"
		for _, engine := range []string{"MKXPZ", "EasyRPG"} {
			values, enabled, _ := formMaps(a, engine)
			values["future"] = map[string]any{"huge": json.Number("9007199254740993123456789"), "string": "00123"}
			enabled["future"] = "Force"
		}
		return launcher.SaveAdvancedSettings(s.ui.manager.Root, a)
	})
}
func (s *Smoke) Work() error {
	return s.ui.operation("smoke cleanup", func(ctx context.Context) error {
		dir, err := os.MkdirTemp(s.ui.manager.Root, ".gui-smoke-stage-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		release := make(chan struct{})
		s.mu.Lock()
		s.stage = dir
		s.release = release
		s.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-time.After(15 * time.Second):
			return errors.New("smoke job cancel handshake timeout")
		}
		select {
		case <-release:
		case <-time.After(5 * time.Second):
			return errors.New("smoke job cleanup handshake timeout")
		}
		s.ui.log("キャンセル後始末完了")
		return ctx.Err()
	})
}
func (s *Smoke) CleanupPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stage == "" {
		return false
	}
	_, err := os.Stat(s.stage)
	return err == nil
}
func (s *Smoke) ReleaseCleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.release != nil {
		close(s.release)
		s.release = nil
	}
}
func (s *Smoke) RuntimeHash() (string, error) {
	root := filepath.Join(s.ui.manager.Root, "v0.94.1")
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(h, rel)
		if d.Type()&os.ModeSymlink != 0 {
			target, e := os.Readlink(p)
			if e != nil {
				return e
			}
			_, _ = fmt.Fprintln(h, target)
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		f, e := os.Open(p)
		if e != nil {
			return e
		}
		_, e = io.Copy(h, f)
		return errors.Join(e, f.Close())
	})
	return hex.EncodeToString(h.Sum(nil)), err
}
func (s *Smoke) Report(text string) {
	s.complete(text)
	s.ReleaseCleanup()
	s.ui.closeAfterCancel(s.app.Quit)
}
func (s *Smoke) complete(text string) {
	s.once.Do(func() {
		var report struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		err := json.Unmarshal([]byte(text), &report)
		if err == nil && !report.OK {
			err = fmt.Errorf("native smoke failed: %s", report.Error)
		}
		f, e := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e == nil {
			_, e = f.WriteString(text + "\n")
			e = errors.Join(e, f.Close())
		}
		s.mu.Lock()
		s.result = errors.Join(err, e)
		s.mu.Unlock()
		close(s.done)
	})
}

const nativeSmokeJS = `(async () => {
 const {Call, Events} = await import('/wails/runtime.js');
 const ui = (name,...args) => Call.ByName('rpgm-launcher/nwjs-helper/internal/ui.UI.' + name,...args);
 const test = (name,...args) => Call.ByName('rpgm-launcher/nwjs-helper/internal/ui.Smoke.' + name,...args);
 const evidence = {ok:false, checks:[], errors:[]};
 const assert = (ok,name) => { if (!ok) throw new Error(name); evidence.checks.push(name); };
 const sleep = ms => new Promise(resolve=>setTimeout(resolve,ms));
 const until = async (predicate,name='DOM/bridge condition') => {
  const deadline=performance.now()+4500;
  while(performance.now()<deadline){if(await predicate())return;await sleep(30)}
  throw new Error(name+' timeout (4500ms)');
 };
 window.addEventListener('error',e=>evidence.errors.push(String(e.message)));
 window.addEventListener('unhandledrejection',e=>evidence.errors.push(String(e.reason)));
 const sample = () => ({at:performance.now(),wall:new Date().toISOString(),visibility:document.visibilityState,focus:document.hasFocus()});
 evidence.timing=[];
 let beats=0;const timer=setInterval(()=>{beats++;evidence.timing.push(sample());},20);
 try {
  await until(()=>document.documentElement.dataset.ready==='true');
  await window.rpgmReady;
  assert(document.documentElement.lang==='ja','Japanese document ready');
  assert(document.querySelector('h1').textContent==='RPG Maker Launcher','semantic heading');
  assert(document.querySelector('#status[aria-live="polite"]'),'live status');
  assert(document.querySelectorAll('button').length>=20,'all standard actions present');
  assert(document.querySelector('label[for="version"]') && document.querySelector('#version').tagName==='SELECT','labelled native select');
  const inline=document.createElement('script');inline.textContent='window.__rpgmCSPProbe = true;';document.body.append(inline);await sleep(50);inline.remove();
  assert(window.__rpgmCSPProbe!==true,'CSP blocks inline script');
  const link=document.createElement('a');link.href='https://example.invalid/';document.body.append(link);
  const navigates=link.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true}));link.remove();
  assert(!navigates && !document.body.dispatchEvent(new Event('drop',{bubbles:true,cancelable:true})),'external link and dropped URL navigation prevented');
  await test('Seed');
  const before=await ui('Advanced','MKXPZ',false);
  document.getElementById('mkxp-settings').click();
  await until(()=>document.getElementById('advanced-dialog').open && !document.querySelector('#advanced-form button[type=submit]').disabled);
  assert(document.querySelectorAll('#fields fieldset').length===61,'MKXP-Z 61 fields');
  const value = key => document.querySelector('#fields fieldset[data-key="'+key+'"] input:not([id^=enabled]), #fields fieldset[data-key="'+key+'"] textarea');
  assert(value('windowTitle').value==='日本語 001984 <img src=x onerror=alert(1)>','Japanese and XSS text stays text');
  assert(!document.querySelector('#fields img'),'no dynamic HTML injection');
  assert(value('JITMaxCache').value==='9007199254740993123456789','huge integer DOM text');
  assert(value('solidFonts').value.includes('9007199254740993123'),'nested huge integer DOM text');
  assert(document.querySelector('fieldset[data-key=rgssVersion] input[id^=enabled]').disabled,'Force is not togglable');
  assert([...document.querySelectorAll('#fields input,#fields textarea')].every(el=>document.querySelector('label[for="'+el.id+'"]')),'every advanced control labelled');
  document.querySelector('#advanced-form button[type=submit]').click();
  await until(()=>document.getElementById('advanced-message').textContent==='保存しました');
  const after=await ui('Advanced','MKXPZ',false);
  assert(JSON.stringify(before)===JSON.stringify(after),'exact mixed JSON type roundtrip through DOM and service');
  document.getElementById('close-advanced').click();
  document.getElementById('easy-settings').click();
  await until(()=>document.getElementById('advanced-dialog').open && document.querySelectorAll('#fields fieldset').length===56);
  assert(document.querySelector('fieldset[data-key="--project-path"] input:not([id^=enabled])').disabled,'managed project path read-only');
  document.getElementById('close-advanced').click();
  await until(()=>!document.getElementById('install-component').disabled);
  let workFinished=false,workError='';
  const work=test('Work').then(()=>{workFinished=true;},error=>{workError=String(error);workFinished=true;});
  await until(()=>document.getElementById('cancel').disabled===false);
  await until(()=>test('CleanupPending'));
  await test('Foreground');
  // Hidden WebKit throttles intervals to ~1s: hold the job until observed, not for a guessed sleep.
  const count=beats;evidence.busyStart={...sample(),beats,state:await ui('State')};
  await until(()=>beats>count+3,'busy heartbeat');
  evidence.busyEnd={...sample(),beats,state:await ui('State')};
  assert(beats>count+3 && !workFinished && evidence.busyStart.state.Busy && evidence.busyEnd.state.Busy && !evidence.busyEnd.state.Cancelling,'WebKit heartbeat while backend busy');
  assert((await ui('State')).Busy && document.getElementById('launch').disabled,'Go busy and disabled controls');
  let rejected=false;try {await ui('Run',{Action:'missing'})}catch(_){rejected=true}
  assert(rejected,'Go rejects concurrent actions');
  document.getElementById('cancel').click();
  await until(async()=> (await ui('State')).Cancelling);
  evidence.cancelPending={...sample(),state:await ui('State'),stage:await test('CleanupPending')};
  assert(evidence.cancelPending.state.Busy && evidence.cancelPending.state.Cancelling && evidence.cancelPending.stage && !workFinished,'cancel keeps busy through backend cleanup');
  await test('ReleaseCleanup');await until(()=>workFinished,'job cleanup');await work;
  evidence.idle={...sample(),state:await ui('State'),stage:await test('CleanupPending'),workError};
  assert(!evidence.idle.state.Busy && !evidence.idle.stage && workError.includes('context canceled'),'idle only after owned stage cleanup');
  const hashBefore=await test('RuntimeHash');
  let progress=0;const off=Events.On('launcher:progress',()=>progress++);
  const installed=await ui('Run',{Action:'install',Component:'v0.94.1',Approved:true});off();
  const hashAfter=await test('RuntimeHash');
  assert(installed.Path.endsWith('/v0.94.1/nwjs.app') && hashBefore===hashAfter && progress===0,'real ARM NW install no-op through GUI service: unchanged tree, no download progress');
  evidence.runtimeHash=hashAfter;
  const snap=await ui('Snapshot');assert(snap.Installed.includes('v0.94.1'),'offline installed NW discovery');
  const log=await ui('Logs');assert(log.includes('日本語ログ') && log.includes('キャンセル後始末完了'),'Japanese bounded log');
  document.getElementById('log-panel').open=true;await until(()=>document.getElementById('log').textContent.includes('日本語ログ'));
  assert(document.getElementById('log').textContent.length<=131072,'bounded DOM log');
  assert(evidence.errors.length===0,'no JS exceptions');
  await test('SeedLegacy');
  evidence.resetFailures=[];
  for (const [engine,button,key,expected] of [['MKXPZ','mkxp-settings','windowTitle','"Custom Title"'],['EasyRPG','easy-settings','--seed','""']]) {
   try {
    const original=await ui('Advanced',engine,false);
    document.getElementById(button).click();
    await until(()=>document.getElementById('advanced-dialog').open && !document.querySelector('#advanced-form button[type=submit]').disabled);
    document.querySelector('#advanced-form button[type=submit]').click();
    await until(()=>document.getElementById('advanced-message').textContent==='保存しました');
    assert(JSON.stringify(original)===JSON.stringify(await ui('Advanced',engine,false)),engine+' legacy mixed types ordinary DOM save unchanged');
    document.getElementById('reset-advanced').click();
    await until(()=>document.getElementById('advanced-message').textContent.includes('初期値に戻しました'));
    document.querySelector('#advanced-form button[type=submit]').click();
    await until(()=>document.getElementById('advanced-message').textContent==='保存しました');
    const saved=await ui('Advanced',engine,false), field=saved.find(f=>f.Key===key);
    assert(field.Type==='string' && field.ValueJSON===expected && original.filter(f=>f.Key==='future'||f.ReadOnly).every(f=>JSON.stringify(f)===JSON.stringify(saved.find(g=>g.Key===f.Key))),engine+' legacy Reset DOM save restores default type; unknown/Force/read-only preserved');
   } catch(error) { evidence.resetFailures.push(engine+': '+String(error)); }
   document.getElementById('close-advanced').click();
  }
  assert(evidence.resetFailures.length===0,'both legacy Reset DOM roundtrips pass');
  evidence.beats=beats;evidence.ok=true;
 } catch(error) { evidence.error=String(error)+'\n'+String(error?.stack||''); }
 finally { clearInterval(timer);await test('Report',JSON.stringify(evidence)); }
})().catch(error => { console.error(error); });`

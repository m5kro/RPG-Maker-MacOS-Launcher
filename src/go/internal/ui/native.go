package ui

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type nativeUI struct {
	app        *application.App
	window     *application.WebviewWindow
	mu         sync.Mutex
	confirming bool
}

func RunApplication(u *UI, paths Paths) error {
	// Wails' development override must never replace this trusted local UI.
	if os.Getenv("FRONTEND_DEVSERVER_URL") != "" {
		return errors.New("FRONTEND_DEVSERVER_URL is not supported")
	}
	n := &nativeUI{}
	u.native = n
	opts := application.Options{
		Name: "RPG Maker Launcher", Description: "macOS ARM RPG Maker Launcher",
		Services:   []application.Service{application.NewService(u)},
		Assets:     application.AssetOptions{Handler: AssetHandler(paths.WebUI)},
		Mac:        application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		ShouldQuit: func() bool { return u.shouldQuit() },
	}
	attach, finish, err := configureSmoke(u, &opts)
	if err != nil {
		return err
	}
	n.app = application.New(opts)
	u.emit = func(name string, value any) { n.app.Event.Emit("launcher:"+name, value) }
	menu := n.app.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	n.app.Menu.Set(menu)
	prefs := application.MacWebviewPreferences{}
	prefs.TabFocusesLinks.Set(true)
	prefs.TextInteractionEnabled.Set(true)
	prefs.JavaScriptCanOpenWindowsAutomatically.Set(false)
	n.window = n.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "RPG Maker Launcher", Width: 900, Height: 850, MinWidth: 560, MinHeight: 520, URL: "/",
		Mac: application.MacWindow{WebviewPreferences: prefs},
		Permissions: map[application.PermissionType]application.Permission{
			application.PermissionCamera: application.PermissionDeny, application.PermissionMicrophone: application.PermissionDeny,
			application.PermissionGeolocation: application.PermissionDeny, application.PermissionNotifications: application.PermissionDeny,
		},
	})
	n.window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !u.shouldQuit() {
			e.Cancel()
		}
	})
	attach(n.app, n.window)
	if err = n.app.Run(); err != nil {
		return err
	}
	return finish()
}
func (u *UI) PickFolder() (path string, err error) {
	if u.native == nil {
		return "", errors.New("native window is not ready")
	}
	err = u.operation("フォルダ選択", func(context.Context) error {
		var e error
		path, e = u.native.app.Dialog.OpenFile().AttachToWindow(u.native.window).CanChooseFiles(false).CanChooseDirectories(true).PromptForSingleSelection()
		return e
	})
	return
}
func (u *UI) RequestQuit() {
	if u.native != nil {
		u.native.app.Quit()
	}
}
func (u *UI) shouldQuit() bool {
	u.mu.Lock()
	if !u.state.Busy {
		u.state.Closing = true
		u.mu.Unlock()
		return true
	}
	closing := u.state.Closing
	u.mu.Unlock()
	if closing {
		return false
	}
	n := u.native
	n.mu.Lock()
	if n.confirming {
		n.mu.Unlock()
		return false
	}
	n.confirming = true
	n.mu.Unlock()
	d := n.app.Dialog.Question().SetTitle("処理を中止して終了").SetMessage("進行中の処理をキャンセルし、後始末が完了してから終了しますか？ 起動済みのゲームは終了しません。").AttachToWindow(n.window)
	clear := func() { n.mu.Lock(); n.confirming = false; n.mu.Unlock() }
	d.AddButton("戻る").SetAsCancel().SetAsDefault().OnClick(clear)
	d.AddButton("中止して終了").OnClick(func() { clear(); u.closeAfterCancel(n.app.Quit) })
	d.Show()
	return false
}

//go:build !smoke

package ui

import "github.com/wailsapp/wails/v3/pkg/application"

func configureSmoke(_ *UI, _ *application.Options) (func(*application.App, *application.WebviewWindow), func() error, error) {
	return func(*application.App, *application.WebviewWindow) {}, func() error { return nil }, nil
}

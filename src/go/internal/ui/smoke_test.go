//go:build smoke

package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestSmokeRequiresExplicitOwnedRoot(t *testing.T) {
	u := newTestUI(t)
	t.Setenv("RPGM_GUI_SMOKE_RESULT", filepath.Join(t.TempDir(), "result.json"))
	if _, _, err := configureSmoke(u, &application.Options{}); err == nil {
		t.Fatal("unowned root accepted")
	}
	t.Setenv("RPGM_GUI_SMOKE_RESULT", "")
	opts := application.Options{}
	if _, _, err := configureSmoke(u, &opts); err != nil || len(opts.Services) != 0 {
		t.Fatalf("smoke enabled without opt-in: %v", err)
	}
}
func TestSmokeErrorEvidenceIsFailure(t *testing.T) {
	s := &Smoke{path: filepath.Join(t.TempDir(), "result.json"), done: make(chan struct{})}
	s.complete(`{"ok":false,"error":"JS exception"}`)
	if s.result == nil {
		t.Fatal("failure treated as success")
	}
	data, err := os.ReadFile(s.path)
	if err != nil || len(data) == 0 {
		t.Fatal(err)
	}
}
func TestSmokeJobCleanupHandshake(t *testing.T) {
	u := newTestUI(t)
	s := &Smoke{ui: u}
	finished := make(chan error, 1)
	go func() { finished <- s.Work() }()
	until := time.After(time.Second)
	for !s.CleanupPending() {
		select {
		case <-until:
			t.Fatal("job didn't start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	t.Cleanup(func() { u.CancelOperation(); s.ReleaseCleanup() })
	if !u.State().Busy || u.State().Cancelling {
		t.Fatal("job was not held until explicit cancellation")
	}
	if _, err := u.Run(Request{Action: "missing"}); !errors.Is(err, ErrBusy) {
		t.Fatalf("concurrent action: %v", err)
	}
	u.CancelOperation()
	if !u.State().Busy || !u.State().Cancelling || !s.CleanupPending() {
		t.Fatal("cleanup released too soon")
	}
	select {
	case err := <-finished:
		t.Fatalf("job completed before cleanup release: %v", err)
	default:
	}
	s.ReleaseCleanup()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("job did not finish after cleanup release")
	}
	if u.State().Busy || s.CleanupPending() {
		t.Fatal("cleanup not finished")
	}
}

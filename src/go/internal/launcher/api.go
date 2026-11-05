// Package launcher manages official NW.js SDK bundles on macOS arm64.
// It does not own process signals, start a GUI, or change Intel/Rosetta installs.
package launcher

import (
	"context"
	"fmt"
)

// NWVersion is a stable numeric release with an official macOS ARM distribution.
// Arches currently contains only "arm64".
type NWVersion struct {
	Version string   `json:"version"`
	Arches  []string `json:"arches"`
}

// Progress reports byte counts within download, verify or extract, not overall
// install completion. Total is zero when the download size is unknown.
// Completed == Total does not mean the bundle has been published; only a
// successful return from InstallNW confirms installation.
type Progress struct {
	Phase     string
	Completed int64
	Total     int64
}

// ProgressFunc is called synchronously and serially by InstallNW, while the
// managed-root lock is held. A nil callback discards progress. Returning an error
// aborts installation and cleans owned staging; errors.Is can recover that error.
// Callbacks should return promptly and dispatch GUI updates to the GUI thread.
// A validated existing install emits no progress.
type ProgressFunc func(Progress) error

func (progress ProgressFunc) report(phase string, completed, total int64) error {
	if progress == nil {
		return nil
	}
	return progress(Progress{Phase: phase, Completed: completed, Total: total})
}

// ListNWVersions returns deduplicated stable ARM releases in official metadata
// order (not sorted), or an error. An empty successful result is a non-nil slice.
// Only official NW.js HTTPS hosts are used; ctx controls request cancellation.
func ListNWVersions(ctx context.Context) ([]NWVersion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := officialConfig()
	if err := checkHost(c.hostOS, c.hostArch); err != nil {
		return nil, err
	}
	defer c.client.CloseIdleConnections()
	return listNWVersions(ctx, c)
}

// InstallNW installs the official ARM SDK at root/version/nwjs.app and returns
// its canonical absolute bundle path. root must satisfy AcquireManagedLock and
// version must be vX.Y.Z. It acquires/releases the shared managed-root lock itself.
// Existing targets are reused only with a matching arm64/no-Rosetta receipt,
// no Rosetta marker, and a valid ARM bundle. Receipt-less legacy targets are
// preserved and rejected; this is not a discovery/launch validation API.
// Cancellation or callback errors before publication clean owned staging and
// preserve existing versions. Cancellation racing with the final atomic publish
// may arrive after commit; callers must not assume every error means no bundle.
func InstallNW(ctx context.Context, root, version string, progress ProgressFunc) (bundlePath string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c := officialConfig()
	if err := checkHost(c.hostOS, c.hostArch); err != nil {
		return "", err
	}
	defer c.client.CloseIdleConnections()
	return install(ctx, c, progress, installOptions{root: root, version: version})
}

func checkHost(hostOS, hostArch string) error {
	if hostOS != "darwin" || hostArch != "arm64" {
		return fmt.Errorf("requires macOS arm64; Intel/Rosetta is unsupported (got %s/%s)", hostOS, hostArch)
	}
	return nil
}

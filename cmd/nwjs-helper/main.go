package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"sync"
	"syscall"
	"time"
)

const (
	maxMetadata  = 8 << 20
	maxManifest  = 16 << 20
	maxArchive   = 2 << 30
	maxExtracted = 8 << 30
	maxEntries   = 100000
	maxPath      = 4096
)

var versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`)

// Only tests inject endpoints; the executable always uses official HTTPS URLs.
type config struct {
	client       *http.Client
	versionsURL  string
	downloadBase string
}

func officialConfig() config {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 15 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	return config{
		client:       &http.Client{Transport: transport, Timeout: 20 * time.Minute, CheckRedirect: officialRedirect},
		versionsURL:  "https://nwjs.io/versions.json",
		downloadBase: "https://dl.nwjs.io",
	}
}

func officialRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	host := req.URL.Hostname()
	if req.URL.Scheme != "https" || req.URL.User != nil || (req.URL.Port() != "" && req.URL.Port() != "443") ||
		(host != "nwjs.io" && host != "dl.nwjs.io" && host != "dl.node-webkit.org") {
		return errors.New("refusing redirect outside official HTTPS hosts")
	}
	return nil
}

type emitter struct {
	mu  sync.Mutex
	out io.Writer
}

func (e *emitter) emit(v any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return json.NewEncoder(e.out).Encode(v)
}

func (e *emitter) progress(phase string, completed, total int64) error {
	return e.emit(struct {
		Event     string `json:"event"`
		Phase     string `json:"phase"`
		Completed int64  `json:"completed"`
		Total     int64  `json:"total"`
	}{"progress", phase, completed, total})
}

func main() {
	os.Exit(process(officialConfig(), os.Args[1:]))
}

func process(c config, args []string) int {
	// A disappearing GUI pipe must return EPIPE, not terminate before staging cleanup.
	signal.Ignore(syscall.SIGPIPE)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, args, os.Stdout, os.Stderr, c)
}

func run(ctx context.Context, args []string, out, diag io.Writer, c config) int {
	e := &emitter{out: out}
	err := command(ctx, args, diag, c, e)
	if err != nil {
		fmt.Fprintln(diag, "nwjs-helper:", err)
		return 1
	}
	return 0
}

func command(ctx context.Context, args []string, diag io.Writer, c config, e *emitter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("usage: nwjs-helper versions | install --root <absolute-path> --version vX.Y.Z --arch arm64|x64 [--rosetta]")
	}
	switch args[0] {
	case "versions":
		if len(args) != 1 {
			return errors.New("versions takes no arguments")
		}
		return versions(ctx, c, e)
	case "install":
		f := flag.NewFlagSet("install", flag.ContinueOnError)
		f.SetOutput(diag)
		var o installOptions
		f.StringVar(&o.root, "root", "", "absolute managed root")
		f.StringVar(&o.version, "version", "", "NW.js version vX.Y.Z")
		f.StringVar(&o.arch, "arch", "", "arm64 or x64")
		f.BoolVar(&o.rosetta, "rosetta", false, "run x64 using Rosetta")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return errors.New("unexpected positional arguments")
		}
		app, err := install(ctx, c, e, o)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.emit(struct {
			Event   string `json:"event"`
			Version string `json:"version"`
			Path    string `json:"path"`
		}{"complete", o.version, app})
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func versions(ctx context.Context, c config, e *emitter) error {
	b, err := c.metadata(ctx, c.versionsURL, maxMetadata)
	if err != nil {
		return err
	}
	var data struct {
		Versions []struct {
			Version string
			Files   []string
		}
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return fmt.Errorf("versions JSON: %w", err)
	}
	if data.Versions == nil {
		return errors.New("versions JSON is missing versions array")
	}
	type version struct {
		Version string   `json:"version"`
		Arches  []string `json:"arches"`
	}
	result := make([]version, 0)
	seen := map[string]bool{}
	for _, v := range data.Versions {
		if !versionPattern.MatchString(v.Version) || seen[v.Version] {
			continue
		}
		arches := make([]string, 0, 2)
		for _, a := range []string{"arm64", "x64"} {
			if slices.Contains(v.Files, "osx-"+a) {
				arches = append(arches, a)
			}
		}
		if len(arches) > 0 {
			result = append(result, version{v.Version, arches})
			seen[v.Version] = true
		}
	}
	return e.emit(struct {
		Event    string    `json:"event"`
		Versions []version `json:"versions"`
	}{"versions", result})
}

func (c config) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return resp, nil
}

func (c config) metadata(ctx context.Context, url string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	resp, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.ContentLength > limit {
		return nil, errors.New("metadata size limit exceeded")
	}
	return readBounded(resp.Body, limit)
}

func readBounded(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("size limit exceeded")
	}
	return b, nil
}

// One sequential copy loop handles cancellation and throttles JSONL to avoid GUI flooding.
func copyProgress(ctx context.Context, dst io.Writer, src io.Reader, limit int64, e *emitter, phase string, base, total int64) (int64, error) {
	buf := make([]byte, 256<<10)
	var n int64
	last := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		count, readErr := src.Read(buf)
		if n+int64(count) > limit {
			return n, errors.New("size limit exceeded")
		}
		if count > 0 {
			written, err := dst.Write(buf[:count])
			n += int64(written)
			if err != nil {
				return n, err
			}
			if written != count {
				return n, io.ErrShortWrite
			}
			if e != nil && time.Since(last) >= 100*time.Millisecond {
				if err := e.progress(phase, base+n, total); err != nil {
					return n, err
				}
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			return n, ctx.Err()
		}
		if readErr != nil {
			return n, readErr
		}
	}
}

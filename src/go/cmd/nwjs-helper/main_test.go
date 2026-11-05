package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
)

// The library's subprocess tests invoke the real entrypoint, including signal
// and SIGPIPE handling. Only this test binary can redirect official requests.
func TestProcessWorker(t *testing.T) {
	root := os.Getenv("NWJS_TEST_ROOT")
	if root == "" {
		return
	}
	u, err := url.Parse(os.Getenv("NWJS_TEST_SERVER"))
	if err != nil || u.Host == "" {
		t.Fatalf("invalid fixture URL: %v", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, u.Host)
	}
	http.DefaultTransport = transport
	os.Args = []string{"nwjs-helper", "install", "--root", root, "--version", "v0.100.0"}
	main()
}

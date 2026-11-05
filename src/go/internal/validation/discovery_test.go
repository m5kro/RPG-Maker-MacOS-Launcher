package validation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReviewedNWExtensionTarget(t *testing.T) {
	for _, tc := range []struct {
		target string
		ok     bool
	}{
		{"chrome-extension://njgcanhfjdabfmnlmpmdedalocpafnhl/index.html", true},
		{"chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/index.html", false},
		{"https://example.test/index.html", false},
		{"file:///owned/game/index.html", true},
		{"file:///other/game/index.html", false},
	} {
		t.Run(tc.target, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode([]map[string]string{{"type": "page", "url": tc.target, "webSocketDebuggerUrl": "ws://127.0.0.1:1234/devtools/page/id"}})
			}))
			defer server.Close()
			result, err := discoverTarget(context.Background(), debugHTTPClient(), server.URL, "/owned/game", io.Discard)
			if err != nil || (result != "") != tc.ok {
				t.Fatalf("target %q: result=%q err=%v", tc.target, result, err)
			}
		})
	}
}

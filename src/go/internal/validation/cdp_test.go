package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestDebugURL(t *testing.T) {
	for _, tc := range []struct {
		url, scheme string
		ok          bool
	}{
		{"http://127.0.0.1:1234/json/list", "http", true},
		{"ws://127.0.0.1:1234/devtools/page/abc", "ws", true},
		{"http://localhost:1234/json/list", "http", false},
		{"http://127.0.0.1:9999/json/list", "http", false},
		{"http://user@127.0.0.1:1234/json/list", "http", false},
		{"http://127.0.0.1:1234/json/list?next=x", "http", false},
		{"ws://evil.test:1234/devtools/page/a", "ws", false},
		{"ws://127.0.0.1:1234/devtools/browser/a", "ws", false},
		{"ws://127.0.0.1:1234/devtools/page/../a", "ws", false},
	} {
		if err := checkDebugURL(tc.url, tc.scheme, 1234); (err == nil) != tc.ok {
			t.Errorf("%s: %v", tc.url, err)
		}
	}
}

func TestHTTPAndWSRedirectRefused(t *testing.T) {
	hits := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(200) }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer redirect.Close()
	if _, err := debugHTTPClient().Get(redirect.URL); err == nil {
		t.Fatal("HTTP redirect accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(redirect.URL, "http"), &websocket.DialOptions{HTTPClient: debugHTTPClient()})
	if err == nil || hits != 0 {
		t.Fatalf("redirect followed: hits=%d err=%v", hits, err)
	}
}

func TestCDPIDsErrorsAndLargeScreenshot(t *testing.T) {
	for _, mode := range []string{"large", "error", "wrong-id", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				ctx, cancel := context.WithTimeout(r.Context(), time.Second)
				defer cancel()
				_, b, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var request struct {
					ID int `json:"id"`
				}
				if json.Unmarshal(b, &request) != nil {
					return
				}
				_ = conn.Write(ctx, websocket.MessageText, []byte(`{"method":"Network.requestWillBeSent","params":{}}`))
				var response string
				switch mode {
				case "large":
					response = fmt.Sprintf(`{"id":%d,"result":{"data":"%s"}}`, request.ID, strings.Repeat("A", 128*1024))
				case "error":
					response = fmt.Sprintf(`{"id":%d,"error":{"code":-1,"message":"denied"}}`, request.ID)
				case "wrong-id":
					response = `{"id":999,"result":{}}`
				case "oversize":
					response = fmt.Sprintf(`{"id":%d,"result":{"data":"%s"}}`, request.ID, strings.Repeat("A", int(screenshotReadLimit)+1))
				}
				_ = conn.Write(ctx, websocket.MessageText, []byte(response))
				<-ctx.Done()
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			conn.SetReadLimit(screenshotReadLimit)
			var log bytes.Buffer
			c := &cdp{conn: conn, log: &log}
			result, err := c.call(ctx, "Page.captureScreenshot", nil)
			if mode == "large" {
				if err != nil || len(result) < 128*1024 {
					t.Fatalf("large screenshot: %d %v", len(result), err)
				}
			} else if err == nil {
				t.Fatal("accepted invalid CDP response")
			}
			if !strings.Contains(log.String(), "Network.requestWillBeSent") {
				t.Fatal("request event not retained")
			}
		})
	}
}

func TestStateWaitTimeoutAndRenderFailure(t *testing.T) {
	for _, mode := range []string{"false", "exception"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for {
					_, b, err := conn.Read(r.Context())
					if err != nil {
						return
					}
					var q struct {
						ID int `json:"id"`
					}
					_ = json.Unmarshal(b, &q)
					value := `"result":{"type":"boolean","value":false}`
					if mode == "exception" {
						value = `"exceptionDetails":{"text":"render failed"}`
					}
					_ = conn.Write(r.Context(), websocket.MessageText, []byte(fmt.Sprintf(`{"id":%d,"result":{%s}}`, q.ID, value)))
				}
			}))
			defer server.Close()
			conn, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			c := &cdp{conn: conn, log: &bytes.Buffer{}}
			start := time.Now()
			err = c.wait(context.Background(), "false", 80*time.Millisecond)
			if err == nil || time.Since(start) > time.Second {
				t.Fatalf("wait: %v", err)
			}
		})
	}
}

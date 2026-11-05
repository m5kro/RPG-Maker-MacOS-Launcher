package validation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const screenshotReadLimit = 16 << 20

func checkDebugURL(raw, scheme string, port int) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if port < 1 || port > 65535 || (scheme != "http" && scheme != "ws") || u.Scheme != scheme || u.Host != "127.0.0.1:"+strconv.Itoa(port) || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return fmt.Errorf("unsafe debug URL: %q", raw)
	}
	if scheme == "http" && u.Path == "/json/list" {
		return nil
	}
	if scheme == "ws" && strings.HasPrefix(u.Path, "/devtools/page/") && path.Clean(u.Path) == u.Path {
		id := strings.TrimPrefix(u.Path, "/devtools/page/")
		if id != "" && !strings.ContainsAny(id, "/\\%?#") {
			return nil
		}
	}
	return fmt.Errorf("unexpected debug path: %q", raw)
}

func debugHTTPClient() *http.Client {
	return &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("debug redirects forbidden") }}
}

type cdp struct {
	conn   *websocket.Conn
	serial int
	log    io.Writer
}

// Calls are deliberately serial: an unrelated reply is a protocol failure.
func (c *cdp) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	c.serial++
	request := struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{c.serial, method, params}
	b, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if _, err = fmt.Fprintln(c.log, string(b)); err != nil {
		return nil, err
	}
	if err = c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		return nil, err
	}
	for {
		kind, b, err := c.conn.Read(ctx)
		if err != nil {
			return nil, err
		}
		if kind != websocket.MessageText {
			return nil, errors.New("non-text CDP response")
		}
		if _, err = fmt.Fprintln(c.log, string(b)); err != nil {
			return nil, err
		}
		var response struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Error  json.RawMessage `json:"error"`
			Result json.RawMessage `json:"result"`
		}
		if err = json.Unmarshal(b, &response); err != nil {
			return nil, err
		}
		if response.ID != nil {
			if *response.ID != c.serial {
				return nil, fmt.Errorf("CDP reply id %d, wanted %d", *response.ID, c.serial)
			}
			if len(response.Error) > 0 {
				return nil, fmt.Errorf("CDP %s: %s", method, response.Error)
			}
			if len(response.Result) == 0 {
				return nil, errors.New("missing CDP result")
			}
			return response.Result, nil
		}
		if response.Method == "" {
			return nil, errors.New("CDP event missing method")
		}
		if response.Method == "Runtime.exceptionThrown" || response.Method == "Network.loadingFailed" {
			return nil, fmt.Errorf("renderer failure: %s", b)
		}
	}
}

func (c *cdp) evaluate(ctx context.Context, expression string, into any) error {
	b, err := c.call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true})
	if err != nil {
		return err
	}
	var result struct {
		Exception json.RawMessage `json:"exceptionDetails"`
		Result    struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		return err
	}
	if len(result.Exception) > 0 {
		return fmt.Errorf("JavaScript exception: %s", result.Exception)
	}
	if len(result.Result.Value) == 0 {
		return errors.New("observation returned no value")
	}
	return json.Unmarshal(result.Result.Value, into)
}

func (c *cdp) wait(ctx context.Context, expression string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		var ok bool
		if err := c.evaluate(ctx, expression, &ok); err != nil {
			return fmt.Errorf("wait %s: %w", expression, err)
		}
		if ok {
			return nil
		}
		if err := pause(ctx, 100*time.Millisecond); err != nil {
			return fmt.Errorf("wait timed out: %s: %w", expression, err)
		}
	}
}

func pause(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *cdp) key(ctx context.Context, key string, code int) error {
	for _, kind := range []string{"keyDown", "keyUp"} {
		if _, err := c.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": kind, "key": key, "code": key, "windowsVirtualKeyCode": code, "nativeVirtualKeyCode": code}); err != nil {
			return err
		}
		delay := 70 * time.Millisecond
		if kind == "keyUp" {
			delay = 250 * time.Millisecond
		}
		if err := pause(ctx, delay); err != nil {
			return err
		}
	}
	return nil
}

func screenshotPNG(data string) ([]byte, error) {
	if len(data) > screenshotReadLimit {
		return nil, errors.New("oversized screenshot")
	}
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	config, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if config.Width < 320 || config.Height < 240 || config.Width > 4096 || config.Height > 4096 || config.Width*config.Height > 8_000_000 {
		return nil, fmt.Errorf("unexpected image dimensions: %dx%d", config.Width, config.Height)
	}
	image, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	colors := map[uint32]bool{}
	lit := 0
	for y := 0; y < config.Height; y += 4 {
		for x := 0; x < config.Width; x += 4 {
			r, g, b, a := image.At(x, y).RGBA()
			if a > 0 && r+g+b > 0x1800 {
				lit++
			}
			if len(colors) < 32 {
				colors[(r>>8)<<16|(g>>8)<<8|(b>>8)] = true
			}
		}
	}
	if lit < 1000 || len(colors) < 16 {
		return nil, errors.New("blank/black renderer screenshot")
	}
	return b, nil
}

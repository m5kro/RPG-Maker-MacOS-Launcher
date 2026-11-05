package validation

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestScreenshotRejectsBlackTinyAndOversize(t *testing.T) {
	for _, tc := range []struct {
		name      string
		w, h      int
		paint, ok bool
	}{{"rendered", 816, 624, true, true}, {"black", 816, 624, false, false}, {"tiny", 20, 20, true, false}, {"dimensions", 4097, 1, false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, tc.w, tc.h))
			if tc.paint {
				for y := 0; y < tc.h; y++ {
					for x := 0; x < tc.w; x++ {
						img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
					}
				}
			}
			var b bytes.Buffer
			if err := png.Encode(&b, img); err != nil {
				t.Fatal(err)
			}
			_, err := screenshotPNG(base64.StdEncoding.EncodeToString(b.Bytes()))
			if (err == nil) != tc.ok {
				t.Fatalf("%s: %v", tc.name, err)
			}
		})
	}
	if _, err := screenshotPNG(strings.Repeat("A", screenshotReadLimit+1)); err == nil {
		t.Fatal("oversized image accepted")
	}
	if _, err := screenshotPNG("not base64!"); err == nil {
		t.Fatal("invalid image accepted")
	}
}

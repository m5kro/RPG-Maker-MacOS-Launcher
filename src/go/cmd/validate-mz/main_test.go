package main

import (
	"bytes"
	"context"
	"testing"
)

func TestConsentAndExplicitInputs(t *testing.T) {
	for _, args := range [][]string{nil, {"--root", "/not-created"}, {"--root", "/not-created", "--fixture-zip", "/missing.zip", "--runtime", "/v0.94.1"}, {"--reviewed-fixture", "--unexpected"}} {
		var out bytes.Buffer
		if code := run(context.Background(), args, &out); code != 2 {
			t.Fatalf("%v: exit %d: %s", args, code, out.String())
		}
	}
	var out bytes.Buffer
	if code := run(context.Background(), []string{"--root", "/not-created", "--fixture-zip", "/missing.zip", "--runtime", "/v0.94.1", "--reviewed-fixture"}, &out); code != 1 {
		t.Fatalf("missing fixture: exit %d", code)
	}
}

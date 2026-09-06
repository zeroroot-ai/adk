package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// cassette is a committed request/response fixture (testdata/echo.json). The
// test replays the recorded request through the handler and asserts the
// recorded response, so it runs fully hermetically — no daemon, no network,
// no build tags (ADR-0065 R7). Record new cases by adding a cassette file and
// a sub-test that loads it.
type cassette struct {
	Request  EchoRequest  `json:"request"`
	Response EchoResponse `json:"response"`
}

func loadCassette(t *testing.T, path string) cassette {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %s: %v", path, err)
	}
	var c cassette
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("decode cassette %s: %v", path, err)
	}
	return c
}

func TestEcho(t *testing.T) {
	c := loadCassette(t, "testdata/echo.json")

	got, err := echo(context.Background(), c.Request)
	if err != nil {
		t.Fatalf("echo(%+v): %v", c.Request, err)
	}
	if got != c.Response {
		t.Fatalf("echo(%+v) = %+v, want %+v", c.Request, got, c.Response)
	}
}

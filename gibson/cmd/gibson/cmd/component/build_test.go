// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeTools puts stub buf and go binaries first on PATH. Both append their
// arguments to the returned log file. The buf stub writes api/gen on generate.
func fakeTools(t *testing.T, withBuf bool) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls.log")
	write := func(name, body string) {
		// The stub must be executable, so the 0600 limit does not fit.
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o700); err != nil { //nolint:gosec // executable test stub
			t.Fatal(err)
		}
	}
	write("go", `echo "go $*" >> `+log+"\n")
	if withBuf {
		write("buf", `echo "buf $*" >> `+log+"\n"+
			`if [ "$1" = generate ]; then mkdir -p api/gen/x && echo 'package x' > api/gen/x/x.pb.go; fi`+"\n")
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	} else {
		// Hide any real buf: PATH holds only the stub dir and no buf.
		t.Setenv("PATH", bin)
	}
	return log
}

func scaffoldTool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := runInit("smoke", "tool", dir, nil, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	return filepath.Join(dir, "smoke")
}

func readLog(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(log))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// TestBuildGeneratesProtoBeforeModTidy proves a fresh tool scaffold gets
// its api/gen bindings before go mod tidy runs (adk#62).
func TestBuildGeneratesProtoBeforeModTidy(t *testing.T) {
	log := fakeTools(t, true)
	dir := scaffoldTool(t)

	if err := runBuild(dir); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	calls := readLog(t, log)
	bufAt, tidyAt := strings.Index(calls, "buf generate"), strings.Index(calls, "go mod tidy")
	if bufAt < 0 || tidyAt < 0 || bufAt > tidyAt {
		t.Fatalf("want buf generate before go mod tidy, calls:\n%s", calls)
	}
}

// TestBuildSkipsFreshProtoBindings proves buf generate does not run when
// api/gen is newer than the protos.
func TestBuildSkipsFreshProtoBindings(t *testing.T) {
	log := fakeTools(t, true)
	dir := scaffoldTool(t)
	gen := filepath.Join(dir, "api", "gen", "x")
	if err := os.MkdirAll(gen, 0o750); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(gen, "x.pb.go")
	if err := os.WriteFile(f, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(f, future, future); err != nil {
		t.Fatal(err)
	}

	if err := ensureProtoBindings(dir); err != nil {
		t.Fatalf("ensureProtoBindings: %v", err)
	}
	if calls := readLog(t, log); strings.Contains(calls, "buf generate") {
		t.Fatalf("buf generate ran on fresh bindings:\n%s", calls)
	}
}

// TestBuildRegeneratesStaleProtoBindings proves buf generate runs when a
// proto is newer than api/gen.
func TestBuildRegeneratesStaleProtoBindings(t *testing.T) {
	log := fakeTools(t, true)
	dir := scaffoldTool(t)
	gen := filepath.Join(dir, "api", "gen", "x")
	if err := os.MkdirAll(gen, 0o750); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(gen, "x.pb.go")
	if err := os.WriteFile(f, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}

	if err := ensureProtoBindings(dir); err != nil {
		t.Fatalf("ensureProtoBindings: %v", err)
	}
	if calls := readLog(t, log); !strings.Contains(calls, "buf generate") {
		t.Fatalf("buf generate did not run on stale bindings:\n%s", calls)
	}
}

// TestBuildStopsWithoutBuf proves a missing buf gives a message naming
// make proto, and go mod tidy never runs.
func TestBuildStopsWithoutBuf(t *testing.T) {
	log := fakeTools(t, false)
	dir := scaffoldTool(t)

	err := runBuild(dir)
	if err == nil || !strings.Contains(err.Error(), "make proto") {
		t.Fatalf("err = %v, want a message naming make proto", err)
	}
	if calls := readLog(t, log); strings.Contains(calls, "go mod tidy") {
		t.Fatalf("go mod tidy ran after the buf failure:\n%s", calls)
	}
}

// TestBuildSkipsProtoWithoutBufGen proves a component with no
// buf.gen.yaml needs no bindings.
func TestBuildSkipsProtoWithoutBufGen(t *testing.T) {
	fakeTools(t, false)
	if err := ensureProtoBindings(t.TempDir()); err != nil {
		t.Fatalf("ensureProtoBindings: %v", err)
	}
}

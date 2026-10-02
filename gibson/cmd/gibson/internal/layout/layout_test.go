// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package layout

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		dir     string
		want    string
		wantErr string
	}{
		{dir: "my-tool", want: "my-tool"},
		{dir: "tool1", want: "tool1"},
		{dir: "./nested/debug-tool", want: "debug-tool"},
		// The shape component.yaml's metadata.name enforced, kept so the
		// derived name is as strict as the declared one was.
		{dir: "My Tool", wantErr: "not a usable component name"},
		{dir: "-leading", wantErr: "not a usable component name"},
		{dir: "trailing-", wantErr: "not a usable component name"},
		{dir: "under_score", wantErr: "not a usable component name"},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			t.Parallel()
			got, err := Name(tc.dir)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Name(%q) = %q, want an error", tc.dir, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Name(%q): %v", tc.dir, err)
			}
			if got != tc.want {
				t.Errorf("Name(%q) = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
}

// TestBinaryIsBesideTheSource pins the convention resolveBinaryPath already
// assumed when it read metadata.name out of component.yaml: <dir>/<name>.
func TestBinaryIsBesideTheSource(t *testing.T) {
	t.Parallel()

	got, err := Binary("testdata/my-tool")
	if err != nil {
		t.Fatalf("Binary: %v", err)
	}
	if filepath.Base(got) != "my-tool" || filepath.Base(filepath.Dir(got)) != "my-tool" {
		t.Errorf("Binary = %q, want <dir>/my-tool", got)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("Binary = %q, want an absolute path", got)
	}
}

// TestProtoPkgMatchesTheScaffold. The tool proto path is derived from the
// component name, and validate looks for it at that path. If this and the
// scaffold's ProtoPkg ever disagree, validate reports a missing proto for a
// tool that scaffolded correctly.
func TestProtoPkgMatchesTheScaffold(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"debug-tool": "debugtool",
		"nmap":       "nmap",
		"a-b-c":      "abc",
	} {
		if got := ProtoPkg(in); got != want {
			t.Errorf("ProtoPkg(%q) = %q, want %q", in, got, want)
		}
	}
}

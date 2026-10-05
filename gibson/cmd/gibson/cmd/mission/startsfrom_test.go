// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fork rule of ADR-0169, checked by `gibson mission validate`.
func TestValidateCmd_StartsFrom(t *testing.T) {
	const head = `{"name":"m","version":"1.0.0","nodes":{`
	agent := func(id, extra string) string {
		return `"` + id + `":{"id":"` + id + `","type":"NODE_TYPE_AGENT","agent_config":{"agent_name":"a"}` + extra + `}`
	}
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "a node starts from a dependency",
			body: head + agent("recon", "") + "," + agent("exploit", `,"dependencies":["recon"],"starts_from":"recon"`) + `}}`,
		},
		{
			name: "a node starts from an ancestor two steps back, through an edge",
			body: head + agent("recon", "") + "," + agent("probe", `,"dependencies":["recon"]`) + "," +
				agent("exploit", `,"starts_from":"recon"`) + `},"edges":[{"from":"probe","to":"exploit"}]}`,
		},
		{
			name: "the template of a for_each node starts from an ancestor of the for_each node",
			body: head + agent("recon", "") + `,"each":{"id":"each","type":"NODE_TYPE_FOR_EACH","dependencies":["recon"],` +
				`"for_each_config":{"source":"SOURCE_TARGET_SET","template":{"id":"t","type":"NODE_TYPE_AGENT",` +
				`"agent_config":{"agent_name":"a"},"starts_from":"recon"}}}}}`,
		},
		{
			name: "a node that does not exist is refused",
			body: head + agent("recon", "") + "," +
				agent("exploit", `,"dependencies":["recon"],"starts_from":"rekon"`) + `}}`,
			wantErr: `starts_from "rekon" names no node`,
		},
		{
			name:    "the node itself is refused",
			body:    head + agent("recon", `,"starts_from":"recon"`) + `}}`,
			wantErr: "names the node itself",
		},
		{
			name: "a later node is refused",
			body: head + agent("recon", `,"starts_from":"exploit"`) + "," +
				agent("exploit", `,"dependencies":["recon"]`) + `}}`,
			wantErr: `"exploit" is not an earlier node`,
		},
		{
			name:    "a node with no path between them is refused",
			body:    head + agent("a", "") + "," + agent("b", `,"starts_from":"a"`) + `}}`,
			wantErr: `"a" is not an earlier node`,
		},
		{
			name: "a template that names a node outside the ancestors of its for_each node is refused",
			body: head + agent("recon", "") + `,"each":{"id":"each","type":"NODE_TYPE_FOR_EACH",` +
				`"for_each_config":{"source":"SOURCE_TARGET_SET","template":{"id":"t","type":"NODE_TYPE_AGENT",` +
				`"agent_config":{"agent_name":"a"},"starts_from":"recon"}}}}}`,
			wantErr: `the template of "each"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "m.json")
			if err := os.WriteFile(file, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := validateCmd()
			var out strings.Builder
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{file})
			err := cmd.Execute()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want ok", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want a message that contains %q", err, tc.wantErr)
			}
		})
	}
}

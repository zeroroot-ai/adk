// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package scaffold_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/scaffold"
)

// sdkPathRegex extracts every `core/sdk/<path>` reference an LLM coder
// might grep from a rendered AGENTS.md. The character class is
// deliberately conservative: word chars, slashes, dots, and dashes.
// Trailing punctuation like `,`, `.`, `)` is excluded.
var sdkPathRegex = regexp.MustCompile(`core/sdk/[\w./-]+`)

// TestAgentsMD_LinksResolveAgainstLocalSDK verifies that every
// core/sdk/<path> reference in any rendered AGENTS.md exists on disk in
// the polyrepo's local SDK working tree. This catches drift between
// scaffold prose and the actual SDK contract before it reaches a
// developer.
//
// The test resolves the SDK path by walking up from the test's working
// directory until it finds a sibling core/sdk/ with a go.mod declaring
// `module github.com/zeroroot-ai/sdk`. In a polyrepo checkout that
// directory is at <root>/core/sdk/. The test skips (does not fail) when
// the SDK can't be located, so out-of-tree builds and CI without the
// SDK checked out alongside continue to work.
func TestAgentsMD_LinksResolveAgainstLocalSDK(t *testing.T) {
	sdkRoot := findSDKRoot(t)
	if sdkRoot == "" {
		t.Skip("SDK not found alongside ADK in polyrepo layout — skipping link test")
	}

	cases := []struct {
		name  string
		input scaffold.ScaffoldInput
	}{
		{
			name: "agent",
			input: scaffold.ScaffoldInput{
				Name: "demo-agent", Version: "0.1.0",
				Kind: scaffold.KindAgent, SDKVersion: "v1.2.0",
			},
		},
		{
			name: "tool",
			input: scaffold.ScaffoldInput{
				Name: "demo-tool", Version: "0.1.0",
				Kind: scaffold.KindTool, SDKVersion: "v1.2.0",
			},
		},
		{
			name: "plugin",
			input: scaffold.ScaffoldInput{
				Name: "demo-plugin", Version: "0.1.0",
				Kind: scaffold.KindPlugin,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files, err := scaffold.Render(tc.input)
			require.NoError(t, err)

			// AGENTS.md plus every prompt that may cite SDK paths.
			var sources []string
			for path := range files {
				if path == "AGENTS.md" || strings.HasPrefix(path, "prompts/") {
					sources = append(sources, path)
				}
			}
			sort.Strings(sources)

			seen := map[string]bool{}
			for _, src := range sources {
				for _, m := range sdkPathRegex.FindAllString(string(files[src]), -1) {
					seen[m] = true
				}
			}

			require.NotEmpty(t, seen, "AGENTS.md must cite at least one core/sdk/ path")

			for path := range seen {
				rel := strings.TrimPrefix(path, "core/sdk/")
				abs := filepath.Join(sdkRoot, rel)
				if _, err := os.Stat(abs); err != nil {
					t.Errorf("AGENTS.md cites %q but %s does not exist on disk — update the AGENTS.md template or the SDK to keep them in sync",
						path, abs)
				}
			}
		})
	}
}

// findSDKRoot walks up from the test's working directory looking for a
// sibling core/sdk/ whose go.mod declares the SDK module. Returns "" if
// nothing matches.
func findSDKRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for range 12 {
		candidate := filepath.Join(dir, "core", "sdk", "go.mod")
		if data, err := os.ReadFile(candidate); err == nil {
			if strings.Contains(string(data), "module github.com/zeroroot-ai/sdk") {
				return filepath.Dir(candidate)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

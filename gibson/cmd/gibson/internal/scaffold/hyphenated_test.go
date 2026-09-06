// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package scaffold_test

import (
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/scaffold"
)

// TestScaffoldInput_GoName pins the PascalCase Go/proto identifier derivation.
func TestScaffoldInput_GoName(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"debug-tool", "DebugTool"},
		{"byte-identity", "ByteIdentity"},
		{"my-multi-part-name", "MyMultiPartName"},
		{"scanner", "Scanner"},
	} {
		assert.Equal(t, tc.want, scaffold.ScaffoldInput{Name: tc.name}.GoName(), tc.name)
	}
}

// TestScaffoldInput_ProtoPkg pins the de-hyphenated proto-package token.
func TestScaffoldInput_ProtoPkg(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"debug-tool", "debugtool"},
		{"byte-identity", "byteidentity"},
		{"scanner", "scanner"},
	} {
		assert.Equal(t, tc.want, scaffold.ScaffoldInput{Name: tc.name}.ProtoPkg(), tc.name)
	}
}

// hyphenInProtoIdent matches a hyphen appearing inside a proto package
// declaration or message name — both illegal in protobuf.
var (
	protoPackageLine = regexp.MustCompile(`(?m)^package\s+([^;]+);`)
	protoMessageLine = regexp.MustCompile(`(?m)^message\s+(\S+)\s*\{`)
)

// TestRender_HyphenatedName_Compiles is the regression guard for adk#89:
// a hyphenated component name (the documented DNS-label style, e.g.
// "debug-tool") must render Go that parses and proto whose package and
// message identifiers contain no hyphens. Before the fix the scaffold
// emitted `type debug-toolTool struct{}` and `package gibson.tools.debug-tool.v1`,
// neither of which is valid.
func TestRender_HyphenatedName_Compiles(t *testing.T) {
	for _, kind := range []scaffold.Kind{scaffold.KindTool, scaffold.KindPlugin, scaffold.KindAgent, scaffold.KindConnector} {
		t.Run(string(kind), func(t *testing.T) {
			files, err := scaffold.Render(scaffold.ScaffoldInput{
				Name:       "debug-tool",
				Version:    "0.1.0",
				Kind:       kind,
				SDKVersion: "v1.2.0",
			})
			require.NoError(t, err)

			// Every rendered .go file must parse as valid Go.
			fset := token.NewFileSet()
			for name, content := range files {
				if !strings.HasSuffix(name, ".go") {
					continue
				}
				_, perr := parser.ParseFile(fset, name, content, parser.AllErrors)
				require.NoErrorf(t, perr, "rendered %q must be valid Go", name)
			}

			// Every rendered .proto must have hyphen-free package + message idents.
			for name, content := range files {
				if !strings.HasSuffix(name, ".proto") || strings.Contains(name, "vendor/") {
					continue
				}
				src := string(content)
				for _, m := range protoPackageLine.FindAllStringSubmatch(src, -1) {
					assert.NotContainsf(t, m[1], "-", "%s: proto package %q must not contain a hyphen", name, m[1])
				}
				for _, m := range protoMessageLine.FindAllStringSubmatch(src, -1) {
					assert.NotContainsf(t, m[1], "-", "%s: proto message %q must not contain a hyphen", name, m[1])
				}
			}
		})
	}
}

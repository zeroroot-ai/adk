// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package scaffold

import (
	"fmt"
	"regexp"
	"strings"
)

// secretNameRegex is the broker name shape: a secret name is prefixed with
// a broker kind so the daemon knows where to resolve it. Validating here lets
// `gibson component init` fail fast instead of emitting a plugin that the
// broker refuses at start.
var secretNameRegex = regexp.MustCompile(`^(cred|provider_config):[a-z0-9_/:.-]+$`)

// Kind identifies which Gibson component shape a scaffold renders.
type Kind string

// KindAgent, KindTool, KindPlugin, and KindConnector are the component shapes
// a scaffold renders.
const (
	KindAgent     Kind = "agent"
	KindTool      Kind = "tool"
	KindPlugin    Kind = "plugin"
	KindConnector Kind = "connector"
)

// Valid reports whether k is one of the supported kinds.
func (k Kind) Valid() bool {
	switch k {
	case KindAgent, KindTool, KindPlugin, KindConnector:
		return true
	}
	return false
}

// ScaffoldInput carries everything Render needs to produce a complete
// component directory.
type ScaffoldInput struct {
	// Name is the component's DNS-label identifier (e.g. "my-scanner").
	// Must match ^[a-z][a-z0-9-]{0,61}[a-z0-9]$. Caller validates.
	Name string

	// Version is the initial semver string. Defaults to "0.1.0" when empty.
	Version string

	// Kind selects the template set: agent | tool | plugin.
	Kind Kind

	// Secrets is plugin-only: the broker secrets the plugin resolves at start.
	// Non-nil for other kinds is a caller bug caught by Render with a clear
	// error.
	Secrets []SecretInput

	// SDKVersion pins the SDK in the rendered go.mod. Typically derived
	// from runtime/debug.ReadBuildInfo at the cobra layer.
	SDKVersion string
}

// GoName returns Name converted to a PascalCase Go/proto identifier:
// hyphen-delimited segments are title-cased and concatenated. It is used
// everywhere the name appears as a Go type name or a proto message name,
// where the raw DNS-label Name (which may contain hyphens) is illegal.
//
//	"debug-tool"    → "DebugTool"
//	"byte-identity" → "ByteIdentity"
//	"scanner"       → "Scanner"
func (in ScaffoldInput) GoName() string {
	var b strings.Builder
	for _, seg := range strings.Split(in.Name, "-") {
		if seg == "" {
			continue
		}
		b.WriteString(strings.ToUpper(seg[:1]))
		b.WriteString(seg[1:])
	}
	return b.String()
}

// ProtoPkg returns Name flattened to a single lowercase token with hyphens
// removed, suitable for a proto package segment and the go_package alias
// (the portion after ';'), both of which forbid hyphens.
//
//	"debug-tool"    → "debugtool"
//	"byte-identity" → "byteidentity"
func (in ScaffoldInput) ProtoPkg() string {
	return strings.ToLower(strings.ReplaceAll(in.Name, "-", ""))
}

// SecretInput is one broker secret, parsed from a --with-secret flag, that the
// scaffolded plugin resolves at start. Plugin-only.
type SecretInput struct {
	Name string // e.g. "cred:db_password"
}

// ParseSecretFlag parses a --with-secret flag value, a broker secret name.
//
// Example: ParseSecretFlag("cred:api_key")
func ParseSecretFlag(s string) (SecretInput, error) {
	name := strings.TrimSpace(s)
	if strings.Contains(name, "=") {
		return SecretInput{}, fmt.Errorf("scaffold: --with-secret %q: give the secret name only, for example cred:api_key; "+
			"a secret declares no scope or rotation, because a plugin declares itself in code (ADR-0097)", s)
	}
	if !secretNameRegex.MatchString(name) {
		return SecretInput{}, fmt.Errorf("scaffold: --with-secret %q: name must match %s (e.g. cred:api_key)", s, secretNameRegex.String())
	}
	return SecretInput{Name: name}, nil
}

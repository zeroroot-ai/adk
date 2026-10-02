// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package layout names the on-disk conventions of a component directory.
//
// It replaces component.yaml (ADR-0097, adk#90). That file declared four things
// the CLI actually used — the kind, the component name, the path to main.go and
// the path to the plugin manifest — and of those the kind is the only one a
// directory cannot imply. So the kind becomes a required flag and the rest are
// derived here, in one place, rather than read from a file every command had to
// parse first.
//
// The conventions are not new. They are what the scaffold has always produced
// and what resolveBinaryPath already assumed: the directory is named after the
// component, the binary lands beside the source with that name, main.go sits at
// the root, and a plugin's manifest is plugin.yaml. Writing them down means a
// component no longer has to restate them in a file nobody else reads.
package layout

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// nameRe is the DNS-label shape component.yaml's metadata.name enforced, kept
// so the derived name is as strict as the declared one was. A directory called
// "My Tool" cannot be a component name, and failing here says so rather than
// producing an unusable binary path.
var nameRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Name returns the component name implied by dir: the base name of its absolute
// path. The scaffold creates the directory with the component's name, and
// resolveBinaryPath already looked for a binary of that name inside it.
func Name(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("layout: resolve %s: %w", dir, err)
	}
	name := filepath.Base(abs)
	if !nameRe.MatchString(name) {
		return "", fmt.Errorf("layout: directory name %q is not a usable component name: "+
			"it must be a DNS label (lower-case letters, digits and hyphens, not starting "+
			"or ending with a hyphen). Rename the directory, or pass --dir pointing at one "+
			"that is named after the component", name)
	}
	return name, nil
}

// Binary returns the path the compiled component is expected at: <dir>/<name>.
// This is what the scaffold's Makefile produces.
func Binary(dir string) (string, error) {
	name, err := Name(dir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("layout: resolve %s: %w", dir, err)
	}
	return filepath.Join(abs, name), nil
}

// MainGo returns the path to the component's main.go. component.yaml's
// spec.main_path allowed this to be moved and had exactly one reader; the
// scaffold never emitted anything but the root.
func MainGo(dir string) string { return filepath.Join(dir, "main.go") }

// PluginManifest returns the path to a plugin's manifest. component.yaml's
// spec.manifest_path allowed this to be moved and had exactly one reader.
func PluginManifest(dir string) string { return filepath.Join(dir, "plugin.yaml") }

// ProtoPkg flattens a component name to the hyphen-free token used for its
// proto package segment and file path ("debug-tool" to "debugtool"). It must
// match scaffold.ScaffoldInput.ProtoPkg.
func ProtoPkg(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "-", ""))
}

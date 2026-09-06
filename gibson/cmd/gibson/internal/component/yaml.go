// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package component defines the per-component pin file (component.yaml)
// that every Gibson component scaffolded by `gibson component init`
// ships. The file records the kind (agent | tool | plugin), the
// component's identity (name, version), and a small spec section that
// downstream verbs (validate, register, run) read to dispatch correctly.
//
// The shape mirrors the SDK's plugin.Manifest structure deliberately
// (apiVersion + kind + metadata + spec) so an LLM coder seeing one
// knows how to read the other.
package component

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// APIVersionV1 is the only supported component.yaml apiVersion.
const APIVersionV1 = "component.gibson.zeroroot.ai/v1"

// Kind enumerates the three component shapes.
type Kind string

// KindAgent, KindTool, and KindPlugin are the three supported component kinds.
const (
	KindAgent  Kind = "agent"
	KindTool   Kind = "tool"
	KindPlugin Kind = "plugin"
)

// Valid reports whether k is one of the three supported kinds.
func (k Kind) Valid() bool {
	switch k {
	case KindAgent, KindTool, KindPlugin:
		return true
	}
	return false
}

// nameRegex enforces the DNS-label-style name regex used everywhere
// (scaffold, manifest, dashboard).
var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

// Component is the parsed component.yaml.
type Component struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       Kind              `yaml:"kind"`
	Metadata   ComponentMetadata `yaml:"metadata"`
	Spec       ComponentSpec     `yaml:"spec"`
}

// ComponentMetadata carries identity fields.
type ComponentMetadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
}

// ComponentSpec carries kind-specific configuration.
type ComponentSpec struct {
	// Image is an optional container image reference, populated by the
	// developer when they want `gibson component build` to know the
	// target tag.
	Image string `yaml:"image,omitempty"`

	// MainPath is the path (relative to component.yaml) of the Go main
	// package. Defaults to "./" when empty.
	MainPath string `yaml:"main_path,omitempty"`

	// Runtime is plugin-only. One of "process", "pod", "setec".
	// Defaults to "process" for plugin kind, omitted for others.
	Runtime string `yaml:"runtime,omitempty"`

	// ManifestPath is plugin-only. Defaults to "./plugin.yaml".
	ManifestPath string `yaml:"manifest_path,omitempty"`
}

// Load parses a component.yaml at the given path.
func Load(path string) (*Component, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("component: read %s: %w", path, err)
	}
	var c Component
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("component: parse %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("component: invalid %s: %w", path, err)
	}
	return &c, nil
}

// Validate checks that c is structurally well-formed and rejects any
// kind-specific field used on the wrong kind.
func (c *Component) Validate() error {
	var errs []string
	if c.APIVersion != APIVersionV1 {
		errs = append(errs, fmt.Sprintf("apiVersion must be %q, got %q", APIVersionV1, c.APIVersion))
	}
	if !c.Kind.Valid() {
		errs = append(errs, fmt.Sprintf("kind must be one of agent|tool|plugin, got %q", c.Kind))
	}
	if c.Metadata.Name == "" {
		errs = append(errs, "metadata.name is required")
	} else if !nameRegex.MatchString(c.Metadata.Name) {
		errs = append(errs, fmt.Sprintf("metadata.name %q must match ^[a-z][a-z0-9-]{0,61}[a-z0-9]$", c.Metadata.Name))
	}
	if c.Metadata.Version == "" {
		errs = append(errs, "metadata.version is required")
	}
	// Plugin-only fields on non-plugin kinds:
	if c.Kind != KindPlugin {
		if c.Spec.Runtime != "" {
			errs = append(errs, "spec.runtime is plugin-only")
		}
		if c.Spec.ManifestPath != "" {
			errs = append(errs, "spec.manifest_path is plugin-only")
		}
	} else if c.Spec.Runtime != "" {
		// Plugin: validate runtime if provided.
		switch c.Spec.Runtime {
		case "process", "pod", "setec":
		default:
			errs = append(errs, fmt.Sprintf("spec.runtime %q must be one of process|pod|setec", c.Spec.Runtime))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// EffectiveMainPath returns spec.main_path or "./" if empty.
func (c *Component) EffectiveMainPath() string {
	if c.Spec.MainPath == "" {
		return "./"
	}
	return c.Spec.MainPath
}

// EffectiveManifestPath returns spec.manifest_path or "./plugin.yaml"
// if empty. Only meaningful for plugin kind.
func (c *Component) EffectiveManifestPath() string {
	if c.Spec.ManifestPath == "" {
		return "./plugin.yaml"
	}
	return c.Spec.ManifestPath
}

// EffectiveRuntime returns spec.runtime or the kind-default ("process"
// for plugin, "" for agent/tool).
func (c *Component) EffectiveRuntime() string {
	if c.Spec.Runtime != "" {
		return c.Spec.Runtime
	}
	if c.Kind == KindPlugin {
		return "process"
	}
	return ""
}

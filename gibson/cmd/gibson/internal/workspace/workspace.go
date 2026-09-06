// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package workspace owns the lookup and validation of the `.gibson/
// workspace.yaml` config file. Workspaces pin GIBSON_URL and the
// active tenant reference so subcommands stop re-asking via flags.
//
// The file is non-secret: it MUST NOT contain client_id, client_secret,
// bootstrap_token, host_key, password, secret, or token fields. Load
// rejects any such field and refuses world-writable files.
package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Workspace is the parsed workspace.yaml shape.
type Workspace struct {
	GibsonURL      string `yaml:"gibson_url"`
	DefaultKind    string `yaml:"default_kind,omitempty"`
	DefaultRuntime string `yaml:"default_runtime,omitempty"`
	Comment        string `yaml:"comment,omitempty"`
}

// ErrNoGibsonURL is returned by Resolve when no source yields a
// non-empty gibson_url.
var ErrNoGibsonURL = errors.New("workspace: GIBSON_URL is required (run gibson init or set GIBSON_URL)")

// ErrCredentialField is returned when a workspace.yaml contains a
// field name that hints at a credential.
var ErrCredentialField = errors.New("workspace: credentials must not be stored in workspace.yaml")

// Resolution reports the resolved values and which source they came
// from, so subcommands can include a friendly hint in error messages.
type Resolution struct {
	GibsonURL string
	Source    string // "flag" | "env" | "local-workspace" | "global-workspace"
}

// Resolve walks the precedence chain (flag → env → local workspace
// (parent walk) → global workspace) and returns the first
// non-empty gibson_url, plus the source label.
//
// flagURL is the explicit --gibson-url value from the cobra layer; pass
// "" if not provided.
//
// The CLI does not consume a tenant identifier: tenant context is
// always embedded in the credentials the dashboard issues
// (the OAuth2 client_id is scoped to a tenant, the bootstrap token to
// a tenant install). Workspace config carries no tenant pin.
func Resolve(flagURL string) (*Resolution, error) {
	if flagURL != "" {
		return &Resolution{GibsonURL: flagURL, Source: "flag"}, nil
	}
	if env := os.Getenv("GIBSON_URL"); env != "" {
		return &Resolution{GibsonURL: env, Source: "env"}, nil
	}
	if w, _, err := loadLocal(); err == nil && w != nil && w.GibsonURL != "" {
		return &Resolution{GibsonURL: w.GibsonURL, Source: "local-workspace"}, nil
	}
	if w, err := loadGlobal(); err == nil && w != nil && w.GibsonURL != "" {
		return &Resolution{GibsonURL: w.GibsonURL, Source: "global-workspace"}, nil
	}
	return nil, ErrNoGibsonURL
}

// LocalPath returns the conventional local workspace path
// "./.gibson/workspace.yaml" relative to the caller's cwd.
func LocalPath() string {
	return filepath.Join(".gibson", "workspace.yaml")
}

// GlobalPath returns the conventional global workspace path
// "~/.gibson/workspace.yaml".
func GlobalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("workspace: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".gibson", "workspace.yaml"), nil
}

// loadLocal walks up from cwd looking for a .gibson/workspace.yaml.
// Returns (workspace, path, nil) on success, (nil, "", nil) if none
// exists, or (nil, "", err) on a real error (including credential-
// field violation).
func loadLocal() (*Workspace, string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	dir := wd
	for range 32 {
		candidate := filepath.Join(dir, ".gibson", "workspace.yaml")
		if _, err := os.Stat(candidate); err == nil {
			w, err := Load(candidate)
			return w, candidate, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, "", nil
}

func loadGlobal() (*Workspace, error) {
	path, err := GlobalPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return Load(path)
}

// Load parses a workspace.yaml at path and rejects credential-named
// fields plus world-writable files.
func Load(path string) (*Workspace, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("workspace: stat %s: %w", path, err)
	}
	if info.Mode().Perm()&0o002 != 0 {
		return nil, fmt.Errorf("workspace: refusing to load world-writable file %s (mode %v)", path, info.Mode().Perm())
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workspace: read %s: %w", path, err)
	}

	// Reject credential-named fields by re-parsing into a generic map.
	var generic map[string]any
	if err := yaml.Unmarshal(b, &generic); err != nil {
		return nil, fmt.Errorf("workspace: parse %s: %w", path, err)
	}
	for _, banned := range []string{
		"client_id", "client_secret", "bootstrap_token", "host_key",
		"password", "secret", "token",
	} {
		if _, ok := generic[banned]; ok {
			return nil, fmt.Errorf("%w: field %q is forbidden in %s", ErrCredentialField, banned, path)
		}
	}

	var w Workspace
	if err := yaml.Unmarshal(b, &w); err != nil {
		return nil, fmt.Errorf("workspace: parse %s: %w", path, err)
	}
	if w.GibsonURL == "" {
		return nil, fmt.Errorf("workspace: %s missing required field gibson_url", path)
	}
	return &w, nil
}

// Save writes w to path with mode 0644 (workspace files are
// non-secret; credentials live elsewhere). Creates parent dir if
// needed.
func Save(path string, w *Workspace) error {
	if w.GibsonURL == "" {
		return errors.New("workspace: gibson_url is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("workspace: create parent dir: %w", err)
	}
	b, err := yaml.Marshal(w)
	if err != nil {
		return fmt.Errorf("workspace: marshal: %w", err)
	}
	return os.WriteFile(path, b, 0o644)
}

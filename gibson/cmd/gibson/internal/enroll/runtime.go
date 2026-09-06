// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package enroll

import (
	"path/filepath"

	"github.com/zeroroot-ai/sdk/capabilitygrant"
)

// Runtime credential persistence (ADR-0045 lifecycle A) now lives canonically in
// the SDK's capabilitygrant package so the adk CLI, agent.Connect, and
// gibson inspect read/write byte-identical files. This file is a thin
// delegation layer keeping the adk-facing names (adk#131).

// EnvRuntimeCredential / EnvGibsonURL mirror the SDK env-override contract.
const (
	EnvRuntimeCredential = capabilitygrant.EnvRuntimeCredential
	EnvGibsonURL         = capabilitygrant.EnvGibsonURL
)

// RuntimeInstall is the canonical SDK runtime-install record (re-exported).
type RuntimeInstall = capabilitygrant.RuntimeInstall

// InstallRef is the canonical SDK install reference (re-exported).
type InstallRef = capabilitygrant.InstallRef

// RuntimeInstallPath returns <gibsonDir>/<kind>/<name>.runtime.json.
func RuntimeInstallPath(kind, name string) (string, error) {
	return capabilitygrant.RuntimeInstallPath(kind, name)
}

// CGHostKeyPath returns <gibsonDir>/<kind>/<name>.host_key — the persisted host
// key the SDK CG client uses for idempotent re-registration. The directory
// relocation (GIBSON_HOME) is owned by the SDK's GibsonDir.
func CGHostKeyPath(kind, name string) (string, error) {
	dir, err := capabilitygrant.GibsonDir()
	if err != nil {
		return "", err
	}
	base := name
	if base == "" {
		base = "default"
	}
	return filepath.Join(dir, kind, base+".host_key"), nil
}

// SaveRuntimeInstall writes the install record atomically with 0600 permissions.
func SaveRuntimeInstall(kind, name string, install RuntimeInstall) (string, error) {
	return capabilitygrant.SaveRuntimeInstall(kind, name, install)
}

// ListInstalls scans <gibsonDir>/{agent,tool,plugin}/*.runtime.json.
func ListInstalls() ([]InstallRef, error) {
	return capabilitygrant.ListInstalls()
}

// ResolveRuntimeCredential returns the runtime credential + dial URL for a
// registered component (env override before file; hard error if absent).
func ResolveRuntimeCredential(kind, name string) (capabilitygrant.RuntimeCredential, string, error) {
	install, err := capabilitygrant.ResolveRuntimeInstall(kind, name)
	if err != nil {
		return capabilitygrant.RuntimeCredential{}, "", err
	}
	return install.Credential, install.GibsonURL, nil
}

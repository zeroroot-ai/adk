// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package enroll

import (
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

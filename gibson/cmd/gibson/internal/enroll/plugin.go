// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package enroll runs the Capability-Grant enrollment handshake (Bootstrap →
// Discover → Register) for Gibson components and persists the resulting host
// key and runtime credential (ADR-0045 lifecycle A).
package enroll

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/zeroroot-ai/sdk/capabilitygrant"
)

// CGClient is the subset of capabilitygrant.Client the CG handshake uses.
// Exposed as an interface to allow stub injection in tests.
type CGClient interface {
	Discover(ctx context.Context) error
	Register(ctx context.Context) error
	AgentID() string
	// RuntimeCredential returns the persistable signing material after a
	// successful Register (ADR-0045 lifecycle A).
	RuntimeCredential() (capabilitygrant.RuntimeCredential, error)
}

// cgEnroll runs the Capability-Grant handshake (Bootstrap → Discover → Register)
// for a named component of any kind, persists the host key (via the SDK client)
// AND the RuntimeInstall (ADR-0045 lifecycle A — so a later `gibson inspect` /
// serve loop can re-sign without re-registering), and returns the platform agent
// id. Single core shared by every kind.
func cgEnroll(ctx context.Context, name, kind, bootstrapToken, gibsonURL string, factory CGClientFactory, httpClient *http.Client) (string, error) {
	if name == "" {
		return "", errors.New("enroll: component name is required")
	}
	if gibsonURL == "" {
		gibsonURL = os.Getenv("GIBSON_URL")
	}
	if gibsonURL == "" {
		return "", errors.New("enroll: GIBSON_URL is required (set to the Gibson platform base URL)")
	}
	if factory == nil {
		factory = DefaultCGClientFactory
	}

	// Idempotency (lifecycle A — persist & reuse): if a runtime credential
	// already exists, this install is registered; reuse it. Re-registration is
	// explicit (delete the file, or a future --force).
	runtimePath, err := RuntimeInstallPath(kind, name)
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(runtimePath); statErr == nil {
		return "", nil
	}

	hostKeyPath, err := CGHostKeyPath(kind, name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(hostKeyPath), 0o700); err != nil {
		return "", fmt.Errorf("enroll: create host key directory: %w", err)
	}

	client, err := factory(capabilitygrant.ClientConfig{
		PlatformURL:    gibsonURL,
		BootstrapToken: bootstrapToken,
		HostKeyPath:    hostKeyPath,
		AgentName:      name,
		AgentMode:      "autonomous",
	})
	if err != nil {
		return "", fmt.Errorf("enroll: capabilitygrant.NewClient: %w", err)
	}
	if httpClient != nil {
		if concrete, ok := client.(*capabilitygrant.Client); ok {
			concrete.SetHTTPClient(httpClient)
		}
	}

	if err := client.Discover(ctx); err != nil {
		return "", fmt.Errorf("enroll: discover: %w", err)
	}
	if err := client.Register(ctx); err != nil {
		return "", fmt.Errorf("enroll: register: %w", err)
	}

	rc, err := client.RuntimeCredential()
	if err != nil {
		return "", fmt.Errorf("enroll: extract runtime credential: %w", err)
	}
	if _, err := SaveRuntimeInstall(kind, name, RuntimeInstall{GibsonURL: gibsonURL, Credential: rc}); err != nil {
		return "", fmt.Errorf("enroll: persist runtime credential: %w", err)
	}

	return client.AgentID(), nil
}

// CGClientFactory builds a CGClient. Tests inject a fake to avoid
// dialing a real platform.
type CGClientFactory func(cfg capabilitygrant.ClientConfig) (CGClient, error)

// DefaultCGClientFactory is the production factory.
var DefaultCGClientFactory CGClientFactory = func(cfg capabilitygrant.ClientConfig) (CGClient, error) {
	return capabilitygrant.NewClient(cfg)
}

// ComponentOptions carries everything EnrollComponent needs to run the
// kind-agnostic CG handshake.
type ComponentOptions struct {
	// Name is the component's metadata.name (from component.yaml).
	Name string
	// Kind is agent | tool | plugin.
	Kind string
	// BootstrapToken authenticates first registration (from enrollment).
	BootstrapToken string
	// GibsonURL overrides the GIBSON_URL env var when non-empty.
	GibsonURL string
	// CGFactory / HTTPClient are test injection points; production callers
	// leave them nil.
	CGFactory  CGClientFactory
	HTTPClient *http.Client
}

// EnrollComponent runs the CG handshake for any component kind and persists the
// host key + RuntimeInstall (ADR-0045 lifecycle A). Returns the agent id.
func EnrollComponent(ctx context.Context, opts ComponentOptions) (string, error) {
	return cgEnroll(ctx, opts.Name, opts.Kind, opts.BootstrapToken, opts.GibsonURL, opts.CGFactory, opts.HTTPClient)
}

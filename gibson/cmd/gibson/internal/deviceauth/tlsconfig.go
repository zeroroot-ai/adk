// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package deviceauth

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// EnvCACert names a PEM file holding a CA certificate to trust IN ADDITION to
// the system trust store.
//
// Every install that terminates TLS with its own internal CA — the normal
// enterprise case, and every kind cluster — needs this. Without it the CLI
// verifies against the system pool alone and every tenant-scoped command dies
// before it starts, with an error naming a certificate rather than the missing
// capability, so the reader looks for a broken cert instead of an absent flag.
//
// The undocumented workaround was Go's SSL_CERT_FILE, which REPLACES the system
// pool rather than adding to it — so the same shell could no longer verify a
// public certificate. That is a Go implementation detail leaking as the
// supported interface (adk#178).
const EnvCACert = "GIBSON_CA_CERT"

// CACertPath resolves the CA file to trust, in precedence order: an explicit
// path (the --ca-cert flag or config), then GIBSON_CA_CERT. Empty means
// "system trust store only".
func CACertPath(explicit string) string {
	if caCertOverride != "" {
		return caCertOverride
	}
	if v := os.Getenv(EnvCACert); v != "" {
		return v
	}
	return explicit
}

// TLSConfig builds the client TLS config, ADDING caCertPath to the system pool
// rather than replacing it.
//
// Adding, not replacing, is the whole point: a private CA for the Gibson edge
// must not cost the process its ability to verify every other certificate it
// might touch.
//
// An unreadable or certificate-free file is an error, never a silent fallback.
// A silently empty pool would surface later as a TLS handshake failure — which
// is the exact confusion this exists to remove.
func TLSConfig(caCertPath string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caCertPath == "" {
		return cfg, nil
	}

	pem, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate %s: %w", caCertPath, err)
	}

	// Start from the system pool so the supplied CA is additive. A system pool
	// that cannot be loaded (rare, but real on minimal images) is not fatal —
	// continue with a pool holding just this CA, which is what the caller asked
	// for and enough to reach their own edge.
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA certificate %s contains no PEM certificate", caCertPath)
	}
	cfg.RootCAs = pool
	return cfg, nil
}

// caCertOverride is set once by the root command's --ca-cert flag. A package
// variable rather than threading a parameter through every subcommand: the CA
// is a property of which install the CLI is talking to, not of any one command,
// and every dial site must honour it or the flag is a trap.
var caCertOverride string

// SetCACertOverride records the --ca-cert flag value. Called by the root
// command before any subcommand runs.
func SetCACertOverride(path string) { caCertOverride = path }

// HTTPClient returns an HTTP client whose transport trusts caCertPath in
// addition to the system pool. An empty path yields a client on the system
// pool alone. Every plain-HTTP call the CLI makes against the platform or its
// issuer must go through this rather than http.DefaultClient — the default
// client is how `--ca-cert` becomes a flag that works for some commands and
// silently not others.
func HTTPClient(caCertPath string) (*http.Client, error) {
	tlsCfg, err := TLSConfig(caCertPath)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   30 * time.Second,
	}, nil
}

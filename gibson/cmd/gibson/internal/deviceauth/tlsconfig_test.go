// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package deviceauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTestCA writes a self-signed CA PEM and returns its path.
func writeTestCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write CA: %v", err)
	}
	return path
}

func TestTLSConfig_NoCAUsesTheSystemStore(t *testing.T) {
	cfg, err := TLSConfig("")
	if err != nil {
		t.Fatalf("TLSConfig: %v", err)
	}
	if cfg.RootCAs != nil {
		t.Error("no CA supplied: RootCAs must stay nil so Go uses the system store")
	}
	if cfg.MinVersion != 0x0303 { // tls.VersionTLS12
		t.Errorf("MinVersion = %x, want TLS 1.2", cfg.MinVersion)
	}
}

func TestTLSConfig_AddsToTheSystemStoreRatherThanReplacingIt(t *testing.T) {
	// This is the whole point of the flag. SSL_CERT_FILE — the undocumented
	// workaround — REPLACES the pool, so trusting a private Gibson edge cost the
	// process its ability to verify every other certificate.
	system, err := x509.SystemCertPool()
	if err != nil || system == nil {
		t.Skip("no system cert pool on this machine")
	}
	systemCount := len(system.Subjects()) //nolint:staticcheck // counting is exactly the point here

	cfg, err := TLSConfig(writeTestCA(t))
	if err != nil {
		t.Fatalf("TLSConfig: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs is nil; the supplied CA was not applied")
	}
	if got := len(cfg.RootCAs.Subjects()); got != systemCount+1 { //nolint:staticcheck
		t.Errorf("pool holds %d CAs, want the system pool (%d) plus the supplied one", got, systemCount)
	}
}

func TestTLSConfig_UnreadableFileIsAnErrorNotASilentFallback(t *testing.T) {
	// A silently empty pool surfaces later as a TLS handshake failure — the exact
	// confusion this option exists to remove.
	_, err := TLSConfig(filepath.Join(t.TempDir(), "absent.pem"))
	if err == nil {
		t.Fatal("expected an error for a missing CA file")
	}
	if !strings.Contains(err.Error(), "absent.pem") {
		t.Errorf("error = %v, want it to name the file", err)
	}
}

func TestTLSConfig_FileWithNoCertificateIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notacert.pem")
	if err := os.WriteFile(path, []byte("this is not a certificate\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := TLSConfig(path)
	if err == nil {
		t.Fatal("expected an error for a file holding no PEM certificate")
	}
	if !strings.Contains(err.Error(), "no PEM certificate") {
		t.Errorf("error = %v, want it to say the file holds no certificate", err)
	}
}

func TestCACertPath_Precedence(t *testing.T) {
	t.Setenv(EnvCACert, "/from/env.pem")
	SetCACertOverride("")
	t.Cleanup(func() { SetCACertOverride("") })

	if got := CACertPath("/from/session.pem"); got != "/from/env.pem" {
		t.Errorf("env should beat the stored session value, got %q", got)
	}

	SetCACertOverride("/from/flag.pem")
	if got := CACertPath("/from/session.pem"); got != "/from/flag.pem" {
		t.Errorf("--ca-cert should beat everything, got %q", got)
	}
}

func TestCACertPath_FallsBackToTheStoredSession(t *testing.T) {
	t.Setenv(EnvCACert, "")
	SetCACertOverride("")
	t.Cleanup(func() { SetCACertOverride("") })

	if got := CACertPath("/from/session.pem"); got != "/from/session.pem" {
		t.Errorf("with no flag and no env, the session value should be used, got %q", got)
	}
	if got := CACertPath(""); got != "" {
		t.Errorf("with nothing configured, want empty (system store only), got %q", got)
	}
}

// The login flow's own HTTP calls must trust the supplied CA — --ca-cert
// working for every command except `gibson login` is the regression this
// pins (the flag was wired into the gRPC dial but the login flow used
// http.DefaultClient).
func TestHTTPClient_TrustsSuppliedCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// The test server's self-signed certificate, PEM-encoded, as the CA file.
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: srv.Certificate().Raw,
	})
	if err := os.WriteFile(caFile, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	hc, err := HTTPClient(caFile)
	if err != nil {
		t.Fatalf("HTTPClient: %v", err)
	}
	resp, err := hc.Get(srv.URL)
	if err != nil {
		t.Fatalf("a client built with the server's CA must verify it: %v", err)
	}
	_ = resp.Body.Close()

	// Sanity: without the CA the same fetch fails, so the test cannot pass
	// vacuously against a system-trusted server.
	plain, err := HTTPClient("")
	if err != nil {
		t.Fatalf("HTTPClient(\"\"): %v", err)
	}
	if _, err := plain.Get(srv.URL); err == nil {
		t.Fatal("the system pool must not trust the test server; the assertion above proved nothing")
	}
}

func TestHTTPClient_UnreadableCAFileIsAnError(t *testing.T) {
	if _, err := HTTPClient(filepath.Join(t.TempDir(), "absent.pem")); err == nil {
		t.Fatal("an unreadable CA file must be an error, not a silent system-pool fallback")
	}
}

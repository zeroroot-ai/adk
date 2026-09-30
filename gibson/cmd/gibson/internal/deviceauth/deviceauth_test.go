// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package deviceauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestCredentialsSaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	in := &Credentials{
		Issuer:       "https://auth.example.com",
		ClientID:     "cli-123",
		TokenURL:     "https://auth.example.com/oauth/v2/token",
		Scopes:       []string{"openid", "offline_access"},
		AccessToken:  "at",
		RefreshToken: "rt",
		Expiry:       time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		ActiveTenant: "acme",
		GibsonURL:    "https://api.example.com",
	}
	if err := in.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := CredentialsPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("credentials mode = %v, want 0600", perm)
	}
	if dir := filepath.Dir(path); filepath.Base(dir) != "auth" {
		t.Fatalf("credentials parent = %q, want .../auth", dir)
	}

	out, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if out.ClientID != in.ClientID || out.RefreshToken != in.RefreshToken || out.ActiveTenant != in.ActiveTenant {
		t.Fatalf("round-trip mismatch: got %+v", out)
	}
	if !out.Expiry.Equal(in.Expiry) {
		t.Fatalf("expiry mismatch: got %v want %v", out.Expiry, in.Expiry)
	}
}

func TestLoadCredentialsNotLoggedIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := LoadCredentials(); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("want ErrNotLoggedIn, got %v", err)
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Delete(); err != nil { // missing file is fine
		t.Fatalf("Delete on missing file: %v", err)
	}
	(&Credentials{Issuer: "i", ClientID: "c", AccessToken: "a", GibsonURL: "g"}).Save()
	if err := Delete(); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}
	if _, err := LoadCredentials(); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("want gone after Delete, got %v", err)
	}
}

func TestJoinURL(t *testing.T) {
	cases := []struct {
		base, path, want string
		wantErr          bool
	}{
		{"https://api.example.com", "/.well-known/gibson-login", "https://api.example.com/.well-known/gibson-login", false},
		{"https://api.example.com/", "/.well-known/gibson-login", "https://api.example.com/.well-known/gibson-login", false},
		{"https://api.example.com:30443", "/x", "https://api.example.com:30443/x", false},
		{"not-a-url", "/x", "", true},
		{"", "/x", "", true},
	}
	for _, tc := range cases {
		got, err := joinURL(tc.base, tc.path)
		if tc.wantErr {
			if err == nil {
				t.Errorf("joinURL(%q): want error", tc.base)
			}
			continue
		}
		if err != nil {
			t.Errorf("joinURL(%q): %v", tc.base, err)
		}
		if got != tc.want {
			t.Errorf("joinURL(%q,%q) = %q, want %q", tc.base, tc.path, got, tc.want)
		}
	}
}

func TestFetchBootstrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != BootstrapPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"https://auth.example.com","client_id":"cli-xyz"}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	b, err := c.FetchBootstrap(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchBootstrap: %v", err)
	}
	if b.Issuer != "https://auth.example.com" || b.ClientID != "cli-xyz" {
		t.Fatalf("unexpected bootstrap: %+v", b)
	}
	// scopes default-filled when omitted
	if len(b.Scopes) == 0 {
		t.Fatalf("expected default scopes to be filled")
	}
}

func TestDiscoverRequiresDeviceEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// advertise token but NOT device_authorization_endpoint
		_, _ = w.Write([]byte(`{"issuer":"x","token_endpoint":"https://t/token"}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	if _, err := c.Discover(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error when device_authorization_endpoint is absent")
	}
}

// TestDiscoverPinsPublicClientAuthStyle proves every poll sends client_id
// in the form body. With the oauth2 default, the first poll tries an
// Authorization header, and each pending answer makes the library send
// the request a second time.
func TestDiscoverPinsPublicClientAuthStyle(t *testing.T) {
	var polls, headerAuth int
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"` + srv.URL + `","token_endpoint":"` + srv.URL +
			`/token","device_authorization_endpoint":"` + srv.URL + `/device"}`))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		polls++
		if r.Header.Get("Authorization") != "" {
			headerAuth++
		}
		w.Header().Set("Content-Type", "application/json")
		if polls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer","expires_in":3600}`))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	endpoint, err := c.Discover(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	cfg := Config(&Bootstrap{ClientID: "cli"}, endpoint)
	da := &oauth2.DeviceAuthResponse{DeviceCode: "dc", Interval: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := PollToken(ctx, cfg, da); err != nil {
		t.Fatalf("PollToken: %v", err)
	}
	if headerAuth != 0 {
		t.Fatalf("%d of %d token requests used an Authorization header, want 0", headerAuth, polls)
	}
}

// TestPollTokenWaitsOutRateLimit proves a 429 from the edge does not end
// the login. The server answers 429, then authorization_pending, then a
// token. oauth2's DeviceAccessToken alone returns the 429 as an error.
func TestPollTokenWaitsOutRateLimit(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			w.Header().Set("X-RateLimit-Reset", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limited"))
		case 2:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		default:
			_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer","expires_in":3600}`))
		}
	}))
	defer srv.Close()

	cfg := &oauth2.Config{ClientID: "cli", Endpoint: oauth2.Endpoint{TokenURL: srv.URL, AuthStyle: PublicClientAuthStyle}}
	da := &oauth2.DeviceAuthResponse{DeviceCode: "dc", Interval: 1, Expiry: time.Now().Add(time.Minute)}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tok, err := PollToken(ctx, cfg, da)
	if err != nil {
		t.Fatalf("PollToken: %v", err)
	}
	if tok.AccessToken != "at" {
		t.Fatalf("access token = %q, want at", tok.AccessToken)
	}
	if calls != 3 {
		t.Fatalf("token endpoint calls = %d, want 3", calls)
	}
}

// TestPollTokenReturnsOtherErrors proves PollToken does not retry an
// error that is not a 429.
func TestPollTokenReturnsOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	}))
	defer srv.Close()

	cfg := &oauth2.Config{ClientID: "cli", Endpoint: oauth2.Endpoint{TokenURL: srv.URL, AuthStyle: PublicClientAuthStyle}}
	da := &oauth2.DeviceAuthResponse{DeviceCode: "dc", Interval: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := PollToken(ctx, cfg, da)
	var re *oauth2.RetrieveError
	if !errors.As(err, &re) || re.ErrorCode != "access_denied" {
		t.Fatalf("err = %v, want access_denied", err)
	}
}

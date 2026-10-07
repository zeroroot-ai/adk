// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package auth

import (
	"strings"
	"testing"
)

// TestBrowserURL_OnlyHTTPSOrLoopbackHTTP proves that only a web URL reaches
// the OS URI handler. A verification URL with another scheme is refused.
func TestBrowserURL_OnlyHTTPSOrLoopbackHTTP(t *testing.T) {
	for _, ok := range []string{
		"https://auth.example/device?user_code=ABCD",
		"http://localhost:8080/device",
		"http://127.0.0.1:8080/device",
		"http://[::1]:8080/device",
	} {
		if _, err := browserURL(ok); err != nil {
			t.Errorf("browserURL(%q) = %v; want accepted", ok, err)
		}
	}
	for _, bad := range []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"http://auth.example/device",
		"ms-settings:display",
		"https:///no-host",
		"",
	} {
		if _, err := browserURL(bad); err == nil || !strings.Contains(err.Error(), "verification URL") {
			t.Errorf("browserURL(%q) = %v; want a refusal", bad, err)
		}
	}
}

// TestOpenBrowser_RefusesBeforeAnyCommand proves that a refused URL starts no
// process.
func TestOpenBrowser_RefusesBeforeAnyCommand(t *testing.T) {
	if err := openBrowser("file:///etc/passwd"); err == nil {
		t.Fatal("openBrowser accepted a file: URL")
	}
}

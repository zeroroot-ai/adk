// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package root

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/docs"
)

// forbiddenInDocs mirrors the customer-terminology deny-list enforced on the
// documentation site (docs-site scripts/check-no-internal-tech-in-docs.mjs).
// The CLI reference is auto-generated from this command tree, so any help text
// that names an internal vendor/implementation would fail the docs build. This
// guard moves that failure left, to the repo that owns the help text.
var forbiddenInDocs = regexp.MustCompile(
	`\b(?:Zitadel|OpenFGA|FGA|SPIFFE|SPIRE|Envoy|JWKS|Langfuse|Neo4j|CNPG|CloudNativePG|ArgoCD|cert-manager|ESO|OPA)\b` +
		`|ext[-_]authz|jwt_authn|x-gibson-identity|cgjwt`,
)

// TestCLISpec_NoInternalTechInHelp asserts the whole generated CLI spec — every
// command's Use/Short/Long/Example/flag usage — is free of internal vendor
// terminology forbidden on the customer documentation surface.
func TestCLISpec_NoInternalTechInHelp(t *testing.T) {
	spec := docs.BuildCLISpec(rootCmd)
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if m := forbiddenInDocs.FindAll(raw, -1); m != nil {
		seen := map[string]bool{}
		for _, b := range m {
			seen[string(b)] = true
		}
		terms := make([]string, 0, len(seen))
		for term := range seen {
			terms = append(terms, term)
		}
		t.Fatalf("gibson --help text contains internal terminology the docs site forbids: %v\n"+
			"The CLI reference is generated from this tree; rephrase the help in customer terms.", terms)
	}
}

// TestCLISpec_NonEmpty is a smoke test that the tree is actually wired: an empty
// spec would make the docs generator silently produce an empty reference.
func TestCLISpec_NonEmpty(t *testing.T) {
	spec := docs.BuildCLISpec(rootCmd)
	if spec.Binary == "" || len(spec.Commands) == 0 {
		t.Fatalf("empty CLI spec: %+v", spec)
	}
}

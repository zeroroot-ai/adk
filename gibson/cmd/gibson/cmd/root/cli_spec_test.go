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

// deletedManifests are the component manifest files that ADR-0097 deleted.
// Help text may say that a file is gone. It must not describe the file as part
// of a scaffold or as an input of a command (adk#119): "component directory
// (containing component.yaml)" sent a developer to look for a file that no
// command writes. plugin.yaml joined the list with adk#118.
var deletedManifests = []string{"component.yaml", "plugin.yaml"}

// collectStrings gathers every string value of a decoded JSON document.
func collectStrings(v any, out *[]string) {
	switch t := v.(type) {
	case string:
		*out = append(*out, t)
	case []any:
		for _, e := range t {
			collectStrings(e, out)
		}
	case map[string]any:
		for _, e := range t {
			collectStrings(e, out)
		}
	}
}

// sentenceSplit breaks help text into sentences. A period that is part of the
// file name itself does not end one.
var sentenceSplit = regexp.MustCompile(`[.:;]\s+|\n\s*\n`)

// TestCLISpec_ADeletedManifestIsNeverALiveFile walks every string of the
// generated CLI spec. A sentence that names a deleted manifest must also say
// that the file no longer exists.
func TestCLISpec_ADeletedManifestIsNeverALiveFile(t *testing.T) {
	raw, err := json.Marshal(docs.BuildCLISpec(rootCmd))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var texts []string
	collectStrings(doc, &texts)
	if len(texts) < 50 {
		t.Fatalf("only %d strings in the CLI spec, so this test read almost nothing", len(texts))
	}

	for _, text := range texts {
		for _, sentence := range sentenceSplit.Split(text, -1) {
			flat := regexp.MustCompile(`\s+`).ReplaceAllString(sentence, " ")
			for _, deleted := range deletedManifests {
				if !regexp.MustCompile(regexp.QuoteMeta(deleted)).MatchString(flat) {
					continue
				}
				if !regexp.MustCompile(`no longer exists|was deleted|is gone`).MatchString(flat) {
					t.Errorf("help text describes %s as a live file: %q", deleted, flat)
				}
			}
		}
	}
}

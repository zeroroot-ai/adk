// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package validate is the kind-aware local validator behind
// `gibson component validate`. Each kind has its own checks:
//
//   - plugin: delegates to sdk/plugin/manifest.Validate (the same
//     function the SDK and daemon call).
//   - tool:   structural component.yaml + (if buf is on PATH) `buf lint`
//     over api/proto/gibson/tools/<pkg>/v1/, plus a grep-check that
//     the response message reserves field 100 = DiscoveryResult.
//   - agent:  structural component.yaml + go/parser sanity-check on
//     main.go to catch obvious typos before `go build`.
package validate

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zeroroot-ai/sdk/plugin/manifest"
	"github.com/zeroroot-ai/sdk/taxonomy"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/layout"
)

// Issue is a single validation finding.
type Issue struct {
	Path    string // file path or "component.yaml"
	Line    int    // 0 if N/A
	Message string
}

func (i Issue) String() string {
	if i.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", i.Path, i.Line, i.Message)
	}
	return fmt.Sprintf("%s: %s", i.Path, i.Message)
}

// Report aggregates findings.
type Report struct {
	Errors   []Issue
	Warnings []Issue
}

// HasErrors reports whether the report contains at least one error.
func (r *Report) HasErrors() bool { return len(r.Errors) > 0 }

// addError appends an Issue to Errors.
func (r *Report) addError(path, msg string) {
	r.Errors = append(r.Errors, Issue{Path: path, Message: msg})
}

// addWarning appends an Issue to Warnings.
func (r *Report) addWarning(path, msg string) {
	r.Warnings = append(r.Warnings, Issue{Path: path, Message: msg})
}

// Run validates the component at dir. The kind is auto-detected from
// component.yaml; pass kind="" to use auto-detection or override.
//
// The first return value is always non-nil. The error return is set
// only for I/O issues that prevent validation from running (e.g.
// component.yaml missing). Validation findings live in the report.
// The three component kinds. They lived on the component package, which existed
// to parse component.yaml; with the file gone the vocabulary belongs with the
// checks that branch on it.
const (
	KindAgent  = "agent"
	KindTool   = "tool"
	KindPlugin = "plugin"
)

// Kinds returns the accepted kinds, in help-text order.
func Kinds() []string { return []string{KindAgent, KindTool, KindPlugin} }

// Run executes the kind-aware local checks for the component in dir and returns
// a Report. A non-nil error is a setup failure (no kind, an unusable directory
// name) rather than a finding about the component; findings are in the Report.
func Run(dir, kind string) (*Report, error) {
	r := &Report{}

	// The kind used to come from component.yaml, which is gone (adk#90). It is
	// the one thing a directory cannot imply, so it is a required argument and
	// an empty one is a setup error rather than a finding: there is no component
	// to report findings about yet.
	if kind == "" {
		return r, errors.New("validate: --kind is required (agent | tool | plugin); " +
			"it used to be read from component.yaml, which no longer exists")
	}

	name, err := layout.Name(dir)
	if err != nil {
		return r, fmt.Errorf("validate: %w", err)
	}

	switch kind {
	case KindAgent:
		validateAgent(dir, r)
	case KindTool:
		validateTool(dir, name, r)
	case KindPlugin:
		validatePlugin(dir, r)
	default:
		return r, fmt.Errorf("validate: --kind must be one of agent|tool|plugin, got %q", kind)
	}

	// Validate ontology.yaml if present (all kinds support it).
	validateOntology(dir, r)

	return r, nil
}

// validateAgent runs structural checks for agent kind.
func validateAgent(dir string, r *Report) {
	mainGo := layout.MainGo(dir)
	if _, err := os.Stat(mainGo); err != nil {
		r.addError(mainGo, "main.go not found")
		return
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, mainGo, nil, parser.PackageClauseOnly|parser.ParseComments); err != nil {
		r.addError(mainGo, fmt.Sprintf("main.go parse error: %v", err))
	}
}

// validateTool runs validateAgent's checks plus proto / buf checks.
func validateTool(dir, name string, r *Report) {
	validateAgent(dir, r)

	// Locate the tool's primary proto file. It lives at a path mirroring
	// its proto package (buf STANDARD's PACKAGE_DIRECTORY_MATCH), with a
	// hyphen-free filename derived from the component name.
	pkg := layout.ProtoPkg(name)
	protoPath := filepath.Join(dir, "api", "proto", "gibson", "tools", pkg, "v1", pkg+".proto")
	if _, err := os.Stat(protoPath); err != nil {
		r.addError(protoPath, "tool proto not found at expected path api/proto/gibson/tools/<pkg>/v1/<pkg>.proto")
		return
	}
	checkField100(protoPath, r)

	// `buf lint` if buf is on PATH; otherwise emit a warning.
	if _, err := exec.LookPath("buf"); err != nil {
		r.addWarning("buf", "buf not found on PATH; skipping `buf lint`. Install: https://buf.build/docs/installation")
		return
	}
	cmd := exec.Command("buf", "lint")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// buf lint emits one finding per line: <file>:<line>:<col>:<message>
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			r.addError("buf", line)
		}
		if len(r.Errors) == 0 {
			r.addError("buf", fmt.Sprintf("buf lint failed: %v", err))
		}
	}
}

// validatePlugin delegates to the SDK manifest validator.
func validatePlugin(dir string, r *Report) {
	manifestPath := layout.PluginManifest(dir)
	if _, err := os.Stat(manifestPath); err != nil {
		r.addError(manifestPath, "plugin.yaml not found")
		return
	}
	_, err := manifest.Load(manifestPath)
	if err == nil {
		return
	}
	if manifest.IsValidationError(err) {
		r.addError(manifestPath, err.Error())
		return
	}
	// I/O or parse error.
	r.addError(manifestPath, fmt.Sprintf("manifest load: %v", err))
}

// field100Regex catches both `gibson.graphrag.v1.DiscoveryResult discovery = 100;`
// and the reserved-only form. We accept either as proof of contract.
var field100Regex = regexp.MustCompile(`(?m)\bgibson\.graphrag\.v1\.DiscoveryResult\s+\w+\s*=\s*100\b`)

func checkField100(protoPath string, r *Report) {
	b, err := os.ReadFile(protoPath)
	if err != nil {
		r.addError(protoPath, fmt.Sprintf("read proto: %v", err))
		return
	}
	if !field100Regex.Match(b) {
		r.addError(protoPath,
			"tool response message must declare field 100 as gibson.graphrag.v1.DiscoveryResult; see AGENTS.md")
	}
}

// validateOntology checks ontology.yaml (if present) using taxonomy.Parse
// and Ontology.Validate. Absence of the file is not an error — the file is
// optional for all component kinds.
func validateOntology(dir string, r *Report) {
	ontologyPath := filepath.Join(dir, "ontology.yaml")
	b, err := os.ReadFile(ontologyPath)
	if os.IsNotExist(err) {
		return // optional file
	}
	if err != nil {
		r.addError(ontologyPath, fmt.Sprintf("read ontology.yaml: %v", err))
		return
	}

	ont, err := taxonomy.Parse(b)
	if err != nil {
		r.addError(ontologyPath, fmt.Sprintf("parse ontology.yaml: %v", err))
		return
	}

	if err := ont.Validate(); err != nil {
		r.addError(ontologyPath, fmt.Sprintf("validate ontology.yaml: %v", err))
	}
}

// ErrFailed is returned by command callers when the report contains
// errors, to drive a non-zero exit code distinct from I/O errors.
var ErrFailed = errors.New("validate: report has errors")

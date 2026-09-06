// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package scaffold renders Gibson component skeletons (agent, tool, plugin,
// connector)
// from embedded text/template assets.
//
// The package's only public entry point is [Render], which maps a
// [ScaffoldInput] to a map of relative-path-to-file-bytes. Callers
// (cobra subcommands) are responsible for atomic disk writes; Render
// itself never touches the filesystem.
//
// Templates were migrated from core/sdk/plugin/scaffold/ as of the
// adk-developer-workflow spec; the SDK no longer ships scaffolds. See
// the design document for the per-kind template directory layout.
package scaffold

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"github.com/zeroroot-ai/sdk/taxonomy"
)

//go:embed all:templates
var tmplFS embed.FS

// pluginOutputFilename maps a template basename in templates/plugin/ to its
// output path in the rendered component directory. The plugin scaffold is
// Go-first (ADR-0065 R4): one handler.go declares typed request/response
// structs and the handler; the SDK derives each method's JSON-Schema contract
// from those Go types, so there is no .proto, no buf, and no generated code.
var pluginOutputFilename = map[string]string{
	"component.yaml.tmpl":   "component.yaml",
	"plugin.yaml.tmpl":      "plugin.yaml",
	"go.mod.tmpl":           "go.mod",
	"handler.go.tmpl":       "handler.go",
	"handler_test.go.tmpl":  "handler_test.go",
	"echo.json.tmpl":        "testdata/echo.json",
	"helm-values.yaml.tmpl": "helm/values.yaml",
	"Makefile.tmpl":         "Makefile",
	"Dockerfile.tmpl":       "Dockerfile",
	".gitignore.tmpl":       ".gitignore",
	"README.md.tmpl":        "README.md",
	"AGENTS.md.tmpl":        "AGENTS.md",
}

// connectorOutputFilename maps templates/connector/ basenames to output paths.
// A connector is declarative (ADR-0065 R6): the connector.yaml manifest IS the
// integration. The scaffold ships a hermetic schema-validation smoke test so a
// standalone connector directory is individually testable.
var connectorOutputFilename = map[string]string{
	"connector.yaml.tmpl":    "connector.yaml",
	"connector_test.go.tmpl": "connector_test.go",
	"go.mod.tmpl":            "go.mod",
	"README.md.tmpl":         "README.md",
	"AGENTS.md.tmpl":         "AGENTS.md",
}

// agentOutputFilename maps templates/agent/ basenames to output paths.
var agentOutputFilename = map[string]string{
	"component.yaml.tmpl": "component.yaml",
	"main.go.tmpl":        "main.go",
	"go.mod.tmpl":         "go.mod",
	"Makefile.tmpl":       "Makefile",
	"Dockerfile.tmpl":     "Dockerfile",
	".gitignore.tmpl":     ".gitignore",
	"README.md.tmpl":      "README.md",
	"AGENTS.md.tmpl":      "AGENTS.md",
	"ontology.yaml.tmpl":  "ontology.yaml",
}

// toolOutputFilename maps templates/tool/ top-level basenames to output paths.
// The proto template "tool.proto.tmpl" is dynamic: it renders to
// "api/proto/gibson/tools/<pkg>/v1/<pkg>.proto" (a package-matching path so
// buf STANDARD's PACKAGE_DIRECTORY_MATCH passes). Vendored protos under
// templates/tool/proto/vendor/ are copied verbatim by walkVendoredProtos.
var toolOutputFilename = map[string]string{
	"component.yaml.tmpl": "component.yaml",
	"main.go.tmpl":        "main.go",
	"go.mod.tmpl":         "go.mod",
	"Makefile.tmpl":       "Makefile",
	"Dockerfile.tmpl":     "Dockerfile",
	".gitignore.tmpl":     ".gitignore",
	"README.md.tmpl":      "README.md",
	"AGENTS.md.tmpl":      "AGENTS.md",
	"buf.yaml.tmpl":       "buf.yaml",
	"buf.gen.yaml.tmpl":   "buf.gen.yaml",
	"ontology.yaml.tmpl":  "ontology.yaml",
	"tool.proto.tmpl":     "", // dynamic: api/proto/gibson/tools/<pkg>/v1/<pkg>.proto
}

// Render produces the full directory contents for a single component
// init. Keys are forward-slash-separated relative paths; values are
// file bytes. Render is atomic from the caller's perspective: it
// either returns a complete map or an error, never a partial map.
func Render(input ScaffoldInput) (map[string][]byte, error) {
	if input.Version == "" {
		input.Version = "0.1.0"
	}
	if !input.Kind.Valid() {
		return nil, errors.New("scaffold: Kind must be one of agent|tool|plugin|connector")
	}
	if len(input.Secrets) > 0 && input.Kind != KindPlugin {
		return nil, errors.New("scaffold: --with-secret is plugin-only; agent, tool, and connector kinds do not declare broker secrets")
	}

	var (
		out map[string][]byte
		err error
	)
	switch input.Kind {
	case KindPlugin:
		out, err = renderTopLevel(input, "templates/plugin", pluginOutputFilename, nil)
	case KindAgent:
		out, err = renderTopLevel(input, "templates/agent", agentOutputFilename, nil)
	case KindTool:
		out, err = renderTool(input)
	case KindConnector:
		out, err = renderTopLevel(input, "templates/connector", connectorOutputFilename, nil)
	default:
		return nil, errors.New("scaffold: unreachable")
	}
	if err != nil {
		return nil, err
	}

	// Per-kind prompts/ subdirectory.
	if err := renderPrompts(input, out); err != nil {
		return nil, err
	}

	// Shared CLAUDE.md and .claude/settings.json (identical across kinds).
	if err := renderShared(input, out); err != nil {
		return nil, err
	}

	// For kinds that scaffold an ontology.yaml (agent, tool), pre-generate
	// gen/ontology_extension.go so the project compiles immediately after
	// `gibson component init`. The developer regenerates it via
	// `gibson component generate` (or `gibson component build`) after editing
	// ontology.yaml.
	if err := renderOntologyGen(out); err != nil {
		return nil, err
	}

	return out, nil
}

// renderOntologyGen parses the rendered ontology.yaml (if present) and adds
// gen/ontology_extension.go to the output map. Tools auto-wire this via the
// optional serve.OntologyContributor interface on the {Name}Tool struct;
// agents currently leave the file unused (a future SDK change will surface
// it via sdk.WithOntologyExtension).
func renderOntologyGen(out map[string][]byte) error {
	yamlBytes, ok := out["ontology.yaml"]
	if !ok {
		// Plugin kind has no ontology.yaml; nothing to generate.
		return nil
	}
	ont, err := taxonomy.Parse(yamlBytes)
	if err != nil {
		return fmt.Errorf("scaffold: parse rendered ontology.yaml: %w", err)
	}
	src, err := GenerateBytes(ont)
	if err != nil {
		return fmt.Errorf("scaffold: render ontology_extension.go: %w", err)
	}
	out["gen/ontology_extension.go"] = src
	return nil
}

// renderPrompts walks templates/<kind>/prompts/ and renders each template
// into out at "prompts/<basename-without-tmpl>".
func renderPrompts(input ScaffoldInput, out map[string][]byte) error {
	dir := "templates/" + string(input.Kind) + "/prompts"
	entries, err := tmplFS.ReadDir(dir)
	if err != nil {
		// Missing prompts dir is fine for kinds that don't ship prompts yet.
		return nil //nolint:nilerr // a missing optional prompts dir is not an error
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".tmpl") {
			continue
		}
		raw, err := tmplFS.ReadFile(dir + "/" + name)
		if err != nil {
			return fmt.Errorf("scaffold: read prompt %q: %w", name, err)
		}
		t, err := template.New(name).Parse(string(raw))
		if err != nil {
			return fmt.Errorf("scaffold: parse prompt %q: %w", name, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, input); err != nil {
			return fmt.Errorf("scaffold: execute prompt %q: %w", name, err)
		}
		outName := "prompts/" + strings.TrimSuffix(name, ".tmpl")
		out[outName] = buf.Bytes()
	}
	return nil
}

// renderShared emits the kind-agnostic CLAUDE.md and .claude/settings.json
// from templates/_shared/.
func renderShared(input ScaffoldInput, out map[string][]byte) error {
	for tmplPath, outPath := range map[string]string{
		"templates/_shared/CLAUDE.md.tmpl":            "CLAUDE.md",
		"templates/_shared/claude_settings.json.tmpl": ".claude/settings.json",
	} {
		raw, err := tmplFS.ReadFile(tmplPath)
		if err != nil {
			return fmt.Errorf("scaffold: read shared %q: %w", tmplPath, err)
		}
		t, err := template.New(tmplPath).Parse(string(raw))
		if err != nil {
			return fmt.Errorf("scaffold: parse shared %q: %w", tmplPath, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, input); err != nil {
			return fmt.Errorf("scaffold: execute shared %q: %w", tmplPath, err)
		}
		out[outPath] = buf.Bytes()
	}
	return nil
}

// dynamicNameFunc, if non-nil, computes the output path for templates whose
// outputFilename map entry is empty. Different kinds have different rules
// (plugin: <name>.proto; tool: api/proto/<name>/v1/<name>.proto).
type dynamicNameFunc func(input ScaffoldInput, tmplName string) string

// toolDynamicName implements the tool scaffold's "tool.proto.tmpl" rule. As
// with plugins, the proto sits at a path mirroring its package so buf
// STANDARD passes, with a hyphen-free filename (package gibson.tools.<pkg>.v1
// → api/proto/gibson/tools/<pkg>/v1/<pkg>.proto).
func toolDynamicName(input ScaffoldInput, tmplName string) string {
	if tmplName == "tool.proto.tmpl" {
		pkg := input.ProtoPkg()
		return "api/proto/gibson/tools/" + pkg + "/v1/" + pkg + ".proto"
	}
	return ""
}

// renderTopLevel walks a templates/<kind>/ directory's top-level files (no
// recursion), templates each .tmpl file against input, and assembles an
// output map keyed by the corresponding paths from outFilename.
func renderTopLevel(
	input ScaffoldInput,
	dir string,
	outFilename map[string]string,
	dyn dynamicNameFunc,
) (map[string][]byte, error) {
	entries, err := tmplFS.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scaffold: read embedded %s: %w", dir, err)
	}

	out := make(map[string][]byte, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		tmplName := entry.Name()

		outName, ok := outFilename[tmplName]
		if !ok {
			// Skip unknown template files silently (forward-compatible).
			continue
		}
		if outName == "" {
			if dyn == nil {
				return nil, fmt.Errorf("scaffold: %s: template %q maps to empty path with no dynamicNameFunc set", dir, tmplName)
			}
			outName = dyn(input, tmplName)
			if outName == "" {
				return nil, fmt.Errorf("scaffold: %s: dynamicNameFunc declined template %q", dir, tmplName)
			}
		}

		raw, err := tmplFS.ReadFile(dir + "/" + tmplName)
		if err != nil {
			return nil, fmt.Errorf("scaffold: read template %q: %w", tmplName, err)
		}

		t, err := template.New(tmplName).Parse(string(raw))
		if err != nil {
			return nil, fmt.Errorf("scaffold: parse template %q: %w", tmplName, err)
		}

		var buf bytes.Buffer
		if err := t.Execute(&buf, input); err != nil {
			return nil, fmt.Errorf("scaffold: execute template %q: %w", tmplName, err)
		}

		out[outName] = buf.Bytes()
	}

	return out, nil
}

// renderTool emits the top-level tool templates plus verbatim copies of
// every file under templates/tool/proto/vendor/.
func renderTool(input ScaffoldInput) (map[string][]byte, error) {
	return renderWithVendor(input, "templates/tool", toolOutputFilename, toolDynamicName, "templates/tool/proto/vendor")
}

// renderWithVendor runs renderTopLevel and then walks the vendor
// subtree, copying every file verbatim into the output map under its
// original path (stripped of the kind prefix).
func renderWithVendor(
	input ScaffoldInput,
	dir string,
	outFilename map[string]string,
	dyn dynamicNameFunc,
	vendorRoot string,
) (map[string][]byte, error) {
	out, err := renderTopLevel(input, dir, outFilename, dyn)
	if err != nil {
		return nil, err
	}
	err = fs.WalkDir(tmplFS, vendorRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		raw, readErr := tmplFS.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("scaffold: read vendored proto %q: %w", path, readErr)
		}
		// Output path strips the templates/<kind>/ prefix → "proto/vendor/...".
		rel := strings.TrimPrefix(path, dir+"/")
		out[rel] = raw
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

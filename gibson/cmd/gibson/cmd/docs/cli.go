// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package docs

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FlagSpec is the machine-readable description of a single command flag.
type FlagSpec struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Usage     string `json:"usage"`
	Default   string `json:"default,omitempty"`
	Type      string `json:"type"`
}

// CommandSpec is the machine-readable description of one command in the tree.
// The shape is intentionally flat and stable: the docs-site CLI-reference
// generator renders directly from it, and a byte-for-byte drift gate compares
// the rendered output against the committed page. Adding a field is
// backwards-compatible; renaming or reordering one is a breaking change to the
// docs contract and must be made deliberately.
type CommandSpec struct {
	// Path is the full invocation path, e.g. "gibson mission submit".
	Path string `json:"path"`
	// Use is the cobra Use string, e.g. "submit [flags]".
	Use         string        `json:"use"`
	Short       string        `json:"short"`
	Long        string        `json:"long,omitempty"`
	Example     string        `json:"example,omitempty"`
	Aliases     []string      `json:"aliases,omitempty"`
	Flags       []FlagSpec    `json:"flags,omitempty"`
	Subcommands []CommandSpec `json:"subcommands,omitempty"`
}

// CLISpec is the top-level machine-readable description of the whole CLI.
type CLISpec struct {
	Binary      string        `json:"binary"`
	Short       string        `json:"short"`
	Long        string        `json:"long,omitempty"`
	GlobalFlags []FlagSpec    `json:"globalFlags,omitempty"`
	Commands    []CommandSpec `json:"commands"`
}

// cliCmd emits the whole command tree as a stable JSON document. It walks
// cmd.Root() at runtime rather than importing the root package, which would
// create an import cycle (root imports docs).
func cliCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cli",
		Short: "Emit the command tree as machine-readable JSON",
		Long: `cli walks the whole gibson command tree and writes it to stdout as a
stable JSON document (sorted, deterministic). It is the source of truth
for the auto-generated CLI reference in the documentation site: the
generator renders the reference from this JSON, and a drift gate fails
if the committed page no longer matches.

Regenerate the committed spec after any command, flag, or help-text
change:

  gibson docs cli > cli-spec.json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec := BuildCLISpec(cmd.Root())
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			return enc.Encode(spec)
		},
	}
}

// BuildCLISpec builds the top-level spec from the root command. It is exported
// so tests in the root package (which owns the fully-assembled command tree)
// can assert invariants on the spec — chiefly that it stays free of internal
// vendor terminology the documentation site forbids on its customer surface.
func BuildCLISpec(root *cobra.Command) CLISpec {
	globalNames := map[string]bool{}
	var global []FlagSpec
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		globalNames[f.Name] = true
		global = append(global, flagSpec(f))
	})

	spec := CLISpec{
		Binary:      root.Name(),
		Short:       root.Short,
		Long:        strings.TrimRight(root.Long, "\n"),
		GlobalFlags: global,
	}
	for _, c := range childCommands(root) {
		spec.Commands = append(spec.Commands, buildCommandSpec(c, root.Name(), globalNames))
	}
	return spec
}

// buildCommandSpec builds the spec for one command and recurses into children.
func buildCommandSpec(cmd *cobra.Command, parentPath string, globalNames map[string]bool) CommandSpec {
	path := parentPath + " " + cmd.Name()
	spec := CommandSpec{
		Path:    path,
		Use:     cmd.Use,
		Short:   cmd.Short,
		Long:    strings.TrimRight(cmd.Long, "\n"),
		Example: strings.TrimRight(cmd.Example, "\n"),
		Aliases: cmd.Aliases,
	}

	// Local (non-inherited) flags only; global flags are captured once at the
	// top level. VisitAll iterates in lexical order, so output is stable.
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" || globalNames[f.Name] {
			return
		}
		spec.Flags = append(spec.Flags, flagSpec(f))
	})

	for _, c := range childCommands(cmd) {
		spec.Subcommands = append(spec.Subcommands, buildCommandSpec(c, path, globalNames))
	}
	return spec
}

// childCommands returns the visible, documentable subcommands of cmd, sorted by
// name for deterministic output. The cobra-generated `help` and `completion`
// commands are excluded: they are not part of the product's command surface.
func childCommands(cmd *cobra.Command) []*cobra.Command {
	out := make([]*cobra.Command, 0, len(cmd.Commands()))
	for _, c := range cmd.Commands() {
		if c.Hidden || !c.IsAvailableCommand() {
			continue
		}
		switch c.Name() {
		case "help", "completion":
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// flagSpec converts a pflag.Flag into its machine-readable description. The
// default value is omitted when it is the zero value for the flag's type, to
// keep the reference free of noise like `--force (default false)`.
func flagSpec(f *pflag.Flag) FlagSpec {
	def := f.DefValue
	switch def {
	case "false", "0", "[]", "{}", "0s":
		def = ""
	}
	return FlagSpec{
		Name:      f.Name,
		Shorthand: f.Shorthand,
		Usage:     f.Usage,
		Default:   def,
		Type:      f.Value.Type(),
	}
}

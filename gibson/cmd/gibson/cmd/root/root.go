// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package root defines the gibson root Cobra command and wires all
// subcommands together.
package root

import (
	"github.com/spf13/cobra"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/agent"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/auth"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/component"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/connector"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/docs"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/inspect"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/mission"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/target"
	wscmd "github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/workspace"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
)

// caCertPath is bound to --ca-cert; see init.
var caCertPath string

var rootCmd = &cobra.Command{
	Use:   "gibson",
	Short: "Gibson Agent Development Kit CLI",
	Long: `gibson — tooling for the Gibson agent / tool / plugin
development lifecycle.

Subcommands:
  login      authenticate the CLI against the platform (device flow)
  logout     end the CLI session and remove stored credentials
  init       initialise a Gibson workspace (.gibson/workspace.yaml)
  component  scaffold, validate, register, run components (agent | tool | plugin)
  connector  enable and manage tenant connectors (catalog | enable | list | disable)
  docs       emit machine-readable docs (JSON Schemas, etc.)
  inspect    show what this principal can do (calls WhoAmI)
  mission    author, validate, render, submit gibson missions
  target     create, list, inspect, update, delete assessment targets
  agent      manage agent/tool/plugin machine identities`,
	SilenceUsage: true,
}

func init() {
	// --ca-cert is persistent: which CA to trust is a property of the install
	// the CLI is talking to, not of any one subcommand. Without it, a private-CA
	// install (every self-hosted deployment with an internal CA, and every kind
	// cluster) fails the TLS handshake before any command can run, with an error
	// that names a certificate rather than the missing option (adk#178).
	rootCmd.PersistentFlags().StringVar(&caCertPath, "ca-cert", "",
		"PEM file with a CA certificate to trust IN ADDITION to the system store "+
			"(env: "+deviceauth.EnvCACert+")")
	cobra.OnInitialize(func() { deviceauth.SetCACertOverride(caCertPath) })

	rootCmd.AddCommand(wscmd.Command())
	rootCmd.AddCommand(auth.LoginCommand())
	rootCmd.AddCommand(auth.LogoutCommand())
	rootCmd.AddCommand(component.Command())
	rootCmd.AddCommand(connector.Command())
	rootCmd.AddCommand(docs.Command())
	rootCmd.AddCommand(inspect.Command())
	rootCmd.AddCommand(mission.Command())
	rootCmd.AddCommand(target.Command())
	rootCmd.AddCommand(agent.Command())
}

// Execute runs the root command. Called from main.
func Execute() error {
	return rootCmd.Execute()
}

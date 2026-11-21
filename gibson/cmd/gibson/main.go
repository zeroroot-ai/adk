// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Command gibson is the Gibson Agent Development Kit command-line interface.
//
// It provides tooling for agent / tool / plugin authors and operators
// working with the Gibson platform. `gibson --help` lists each top-level
// command group. The list is in the Long text of the root command
// (cmd/root/root.go).
package main

import (
	"errors"
	"os"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/component"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/root"
)

func main() {
	if err := root.Execute(); err != nil {
		var exitErr component.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(1)
	}
}

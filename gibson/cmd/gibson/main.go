// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Command gibson is the Gibson Agent Development Kit command-line interface.
//
// It provides tooling for agent / tool / plugin authors and operators
// working with the Gibson platform. Top-level verb groups:
//
//	gibson workspace   initialise and manage a Gibson workspace
//	gibson component   scaffold, build, validate, register, run components
//	                   (--kind agent | tool | plugin)
//	gibson mission     author, validate, render, and submit missions
//	gibson inspect     show identity + permissions for the local credential
//	gibson docs        emit machine-readable docs (JSON Schemas, etc.)
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

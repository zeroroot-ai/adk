// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/layout"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/runner"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/validate"
	"github.com/zeroroot-ai/sdk/capabilitygrant"
)

// runCmd returns `gibson component run`.
func runCmd() *cobra.Command {
	var (
		dir          string
		kindFlag     string
		drainTimeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the compiled component binary, supervising signals + exit code 75",
		Long: `run starts the compiled component binary in this directory, forwards
its stdout/stderr to the operator's terminal, hooks SIGINT/SIGTERM, and
waits up to --drain-timeout (default 30s) for graceful shutdown before
escalating to SIGKILL.

run is a thin process supervisor — it does NOT compile the binary
(use ` + "`make build`" + ` first) and it does NOT mock handlers. The
component's existing graceful-drain logic in plugin.Serve / serve.Agent /
serve.Tool is the contract; this verb just supervises it.

--kind is required. It used to be auto-detected from component.yaml, which
no longer exists (ADR-0097). The binary is <dir>/<dir-name>, which is what
the scaffold's Makefile produces.

Pre-flight: refuses to launch if the runtime credential at
~/.gibson/<kind>/<name>.runtime.json is missing, and points at the two
environment variables that enrol a component at boot.

Exit codes are surfaced verbatim from the child. Notably:
  75  the SDK's plugin rotation contract — not a crash; the platform
      should restart the binary. The CLI prints a clear note.

Examples:
  gibson component run --kind tool
  gibson component run --kind plugin --dir ./my-plugin
  gibson component run --kind agent --drain-timeout 60s`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return doRun(dir, kindFlag, drainTimeout)
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "component directory (containing the compiled binary)")
	cmd.Flags().StringVar(&kindFlag, "kind", "", "component kind: "+strings.Join(validate.Kinds(), " | ")+" (required)")
	cmd.Flags().DurationVar(&drainTimeout, "drain-timeout", 30*time.Second, "max wait between SIGTERM and SIGKILL on shutdown")
	return cmd
}

func doRun(dir, kind string, drainTimeout time.Duration) error {
	// The kind came from component.yaml, which is gone (adk#90). Nothing about a
	// directory implies it, so it is required rather than guessed: guessing wrong
	// means looking for the wrong credential and reporting a component
	// unregistered when it is not.
	if kind == "" {
		return fmt.Errorf("component run: --kind is required (%s); "+
			"it used to be read from component.yaml, which no longer exists",
			strings.Join(validate.Kinds(), " | "))
	}
	if !slices.Contains(validate.Kinds(), kind) {
		return fmt.Errorf("component run: --kind must be one of %s, got %q",
			strings.Join(validate.Kinds(), " | "), kind)
	}

	name, err := layout.Name(dir)
	if err != nil {
		return fmt.Errorf("component run: %w", err)
	}

	if err := preflightCredentials(kind, name); err != nil {
		return err
	}

	binPath, err := layout.Binary(dir)
	if err != nil {
		return fmt.Errorf("component run: %w", err)
	}
	if _, statErr := os.Stat(binPath); statErr != nil {
		return fmt.Errorf("component run: binary not found at %s — did you `make build`?: %w", binPath, statErr)
	}

	exitCode, runErr := runner.Run(context.Background(), runner.RunOptions{
		Binary:       binPath,
		DrainTimeout: drainTimeout,
	})
	if runErr != nil {
		return runErr // setup or supervisor error → exit 1
	}

	switch exitCode {
	case 0:
		return nil
	case runner.ExitCodeRotation:
		fmt.Fprintln(os.Stderr, "component run: child requested rotation (exit 75); not restarting (CLI is one-shot)")
	}
	return ExitCodeError{Code: exitCode}
}

// preflightCredentials returns an error if this component has not been
// registered. Kind-uniform (ADR-0045): every kind persists a runtime
// credential at ~/.gibson/<kind>/<name>.runtime.json after the CG handshake.
func preflightCredentials(kind, name string) error {
	// capabilitygrant.RuntimeInstallPath is the one place this path is built; `gibson
	// inspect` reads the same file through the same package. My first pass
	// duplicated the filepath.Join here, which is how two spellings of one path
	// start to drift.
	path, err := capabilitygrant.RuntimeInstallPath(kind, name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("component run: %s %q is not enrolled (no runtime credential at %s). "+
				"A component enrols itself at boot from two environment variables: set GIBSON_URL and "+
				"GIBSON_BOOTSTRAP_TOKEN and start the binary once. `gibson component register` used to "+
				"do this and was deleted with component.yaml (ADR-0097, sdk#128)", kind, name, path)
		}
		return fmt.Errorf("component run: stat %s: %w", path, err)
	}
	return nil
}

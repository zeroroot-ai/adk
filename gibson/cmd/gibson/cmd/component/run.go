// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/component"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/enroll"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/runner"
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

Pre-flight: refuses to launch if the kind-appropriate credential file
is missing (~/.gibson/<kind>/credentials for agent/tool;
~/.gibson/plugin/<name>/host_key for plugin) and points at
` + "`gibson component register`" + `.

Exit codes are surfaced verbatim from the child. Notably:
  75  the SDK's plugin rotation contract — not a crash; the platform
      should restart the binary. The CLI prints a clear note.

Examples:
  gibson component run
  gibson component run --dir ./my-tool
  gibson component run --drain-timeout 60s`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return doRun(dir, kindFlag, drainTimeout)
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "component directory (containing component.yaml + compiled binary)")
	cmd.Flags().StringVar(&kindFlag, "kind", "", "override kind (agent | tool | plugin); auto-detected from component.yaml when unset")
	cmd.Flags().DurationVar(&drainTimeout, "drain-timeout", 30*time.Second, "max wait between SIGTERM and SIGKILL on shutdown")
	return cmd
}

func doRun(dir, kindStr string, drainTimeout time.Duration) error {
	c, _, err := loadComponent(dir)
	if err != nil {
		return err
	}
	kind := c.Kind
	if kindStr != "" {
		k := component.Kind(kindStr)
		if !k.Valid() {
			return fmt.Errorf("component run: --kind must be one of agent|tool|plugin, got %q", kindStr)
		}
		if k != c.Kind {
			return fmt.Errorf("component run: --kind=%s does not match component.yaml kind=%s", k, c.Kind)
		}
		kind = k
	}

	if err := preflightCredentials(kind, c); err != nil {
		return err
	}

	binPath, err := resolveBinaryPath(dir, c)
	if err != nil {
		return err
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
func preflightCredentials(kind component.Kind, c *component.Component) error {
	path, err := enroll.RuntimeInstallPath(string(kind), c.Metadata.Name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("component run: %s not registered (no runtime credential at %s) — run `gibson component register --token <bootstrap-token>` first", kind, path)
		}
		return fmt.Errorf("component run: stat %s: %w", path, err)
	}
	return nil
}

// resolveBinaryPath returns the absolute path to the compiled binary.
// Default: <dir>/<metadata.name> — matches what the scaffold's Makefile
// produces. spec.image is informational only at this layer.
func resolveBinaryPath(dir string, c *component.Component) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("component run: abs %s: %w", dir, err)
	}
	bin := filepath.Join(abs, c.Metadata.Name)
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("component run: binary not found at %s — did you `make build`?: %w", bin, err)
	}
	return bin, nil
}

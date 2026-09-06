// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/component"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/enroll"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/workspace"
)

const envBootstrapToken = "GIBSON_BOOTSTRAP_TOKEN"

// registerCmd returns `gibson component register`. Kind-uniform (ADR-0045):
// agent, tool, and plugin all register via the Capability-Grant handshake using
// the one-time bootstrap token issued at enrollment. The CLI does NOT mint
// identity — run the dashboard's "Register Agent / Tool / Plugin" wizard (or
// `gibson agent enroll`) first; it returns the bootstrap token you paste here.
//
// On success it persists, under ~/.gibson/<kind>/ (relocatable via GIBSON_HOME):
//   - <name>.host_key      — the host key for idempotent re-registration
//   - <name>.runtime.json  — the RuntimeCredential (agent key + agent id +
//     component scope) that `gibson inspect` / the serve loop reuse to sign
//     per-RPC CG-JWTs without re-registering (lifecycle A).
func registerCmd() *cobra.Command {
	var (
		dir       string
		kindFlag  string
		gibsonURL string
		nameFlag  string
		token     string
	)

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register this component via its bootstrap token (Capability-Grant handshake)",
		Long: `register performs first-time registration of this component install.

The CLI does NOT mint identity. Run the dashboard's "Register Agent /
Tool / Plugin" wizard (or ` + "`gibson agent enroll`" + `) first; it
returns a one-time bootstrap token. Paste it here:

  gibson component register --token <bootstrap-token>

This runs the Capability-Grant handshake (Discover → Register) and
persists, under ~/.gibson/<kind>/ (relocatable via GIBSON_HOME):

  <name>.host_key      host key for idempotent re-registration
  <name>.runtime.json  the runtime credential gibson inspect / serve reuse

The kind is auto-detected from component.yaml in --dir.

NEVER paste the token into shell history. Use stdin ("-") or set
GIBSON_BOOTSTRAP_TOKEN in your shell.

Examples:
  gibson component register --token eyJhbGci...
  gibson component register --token -            # read from stdin
  gibson component register --kind plugin --token eyJhbGci...   # override kind`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRegister(cmd.InOrStdin(), registerOptions{
				Dir:       dir,
				KindFlag:  kindFlag,
				GibsonURL: gibsonURL,
				Name:      nameFlag,
				Token:     token,
			})
		},
	}

	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "component directory (containing component.yaml)")
	cmd.Flags().StringVar(&kindFlag, "kind", "", "override kind (agent | tool | plugin); auto-detected from component.yaml when unset")
	cmd.Flags().StringVar(&gibsonURL, "gibson-url", "", "Gibson platform URL; falls back to env / workspace.")
	cmd.Flags().StringVar(&nameFlag, "name", "", "optional install name (allows multiple credentials per kind on one host)")
	cmd.Flags().StringVar(&token, "token", "", `bootstrap token. Use "-" to read from stdin or set GIBSON_BOOTSTRAP_TOKEN.`)
	return cmd
}

type registerOptions struct {
	Dir       string
	KindFlag  string
	GibsonURL string
	Name      string
	Token     string
}

func runRegister(stdin io.Reader, opts registerOptions) error {
	c, _, err := loadComponent(opts.Dir)
	if err != nil {
		return err
	}
	kind := c.Kind
	if opts.KindFlag != "" {
		k := component.Kind(opts.KindFlag)
		if !k.Valid() {
			return fmt.Errorf("component register: --kind must be one of agent|tool|plugin, got %q", opts.KindFlag)
		}
		if k != c.Kind {
			return fmt.Errorf("component register: --kind=%s does not match component.yaml kind=%s", k, c.Kind)
		}
		kind = k
	}

	resolvedURL := opts.GibsonURL
	if resolvedURL == "" {
		if res, rerr := workspace.Resolve(""); rerr == nil {
			resolvedURL = res.GibsonURL
		}
	}

	token, err := resolveToken(stdin, opts.Token)
	if err != nil {
		return err
	}

	agentID, err := enroll.EnrollComponent(context.Background(), enroll.ComponentOptions{
		Name:           c.Metadata.Name,
		Kind:           string(kind),
		BootstrapToken: token,
		GibsonURL:      resolvedURL,
	})
	if err != nil {
		return err
	}

	runtimePath, _ := enroll.RuntimeInstallPath(string(kind), c.Metadata.Name)
	fmt.Printf("component register: registered %s %q (agent_id=%s)\n", kind, c.Metadata.Name, agentID)
	fmt.Printf("  runtime credential: %s\n", runtimePath)

	return appendStateRecord(opts.Dir, stateRecord{
		Kind:        string(kind),
		PrincipalID: agentID,
		GibsonURL:   resolvedURL,
		EnrolledAt:  time.Now().UTC().Format(time.RFC3339),
	})
}

// resolveToken implements the standard precedence:
//  1. flag value, when not "-" and non-empty
//  2. stdin, when flag value is "-"
//  3. GIBSON_BOOTSTRAP_TOKEN env var
func resolveToken(stdin io.Reader, flagValue string) (string, error) {
	switch {
	case flagValue == "-":
		return readSecretFromStdin(stdin)
	case flagValue != "":
		return flagValue, nil
	}
	if v := os.Getenv(envBootstrapToken); v != "" {
		return v, nil
	}
	return "", errors.New(`component register: --token missing (set the flag, pass "-" for stdin, or export GIBSON_BOOTSTRAP_TOKEN)`)
}

func readSecretFromStdin(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return "", errors.New(`component register: stdin was empty (--token -)`)
	}
	v := strings.TrimRight(scanner.Text(), "\r\n")
	if v == "" {
		return "", errors.New(`component register: stdin contained an empty line`)
	}
	return v, nil
}

// loadComponent reads dir/component.yaml.
func loadComponent(dir string) (*component.Component, string, error) {
	path := filepath.Join(dir, "component.yaml")
	c, err := component.Load(path)
	if err != nil {
		return nil, "", fmt.Errorf("component register: %w", err)
	}
	return c, path, nil
}

// stateRecord is one entry appended to ./.gibson/state.json on
// successful registration. Per design.md "Model 3" — developer aid only.
type stateRecord struct {
	Kind        string `json:"kind"`
	PrincipalID string `json:"principal_id,omitempty"`
	GibsonURL   string `json:"gibson_url,omitempty"`
	EnrolledAt  string `json:"enrolled_at"`
}

type stateFile struct {
	Registrations []stateRecord `json:"registrations"`
}

func appendStateRecord(dir string, rec stateRecord) error {
	path := filepath.Join(dir, ".gibson", "state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("component register: create state dir: %w", err)
	}
	var s stateFile
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s) // tolerate corrupt/missing
	}
	s.Registrations = append(s.Registrations, rec)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("component register: marshal state: %w", err)
	}
	return os.WriteFile(path, b, 0o644)
}

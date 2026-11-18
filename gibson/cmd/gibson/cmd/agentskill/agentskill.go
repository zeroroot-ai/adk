// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package agentskill implements the `gibson agent-skill` subcommands: validate
// and publish.
//
// An Agent Skill is a folder with one SKILL.md and optional script, reference
// and asset files. The daemon has no write path to a registry. The publish
// command packs the folder as an OCI image, pushes the image by digest to the
// registry that the user names, and then asks the daemon to pull and store it.
// The command uses the docker credential of the user for the push. The daemon
// uses a tenant secret for the pull. The daemon resolves the tenant from the
// bearer token of the login session (ADR-0093).
package agentskill

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/component"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/agentskill"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	tenantv1 "github.com/zeroroot-ai/adk/gibson/internal/genproto/gibson/tenant/v1"
)

// Command returns the `gibson agent-skill` parent command.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "agent-skill",
		Short: "Validate and publish Agent Skills",
		Long: `gibson agent-skill - check and publish Agent Skills.

An Agent Skill is a folder in the Agent Skills format. The folder has one
SKILL.md file. It can also have the folders scripts, references and assets.
The format is at https://agentskills.io/specification.

Subcommands:
  validate  check a folder against the format. No network call.
  publish   validate a folder, push it to your registry, and store it in your tenant`,
		SilenceUsage: true,
	}
	c.AddCommand(validateCmd())
	c.AddCommand(publishCmd())
	return c
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <folder>",
		Short: "Check an Agent Skill folder against the format",
		Long: `Check an Agent Skill folder against the Agent Skills format and the size
limits of the platform. The command makes no network call.

The name of the folder must equal the name in the frontmatter of SKILL.md.
The command prints each problem that it finds. It prints "ok" when the folder
is valid. If the folder has a problem, the command exits with code 2.

Limits: SKILL.md body 64 KiB, one file 1 MiB, the folder 8 MiB, 200 files.`,
		Example: `  gibson agent-skill validate ./web-recon`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := loadAndPack(cmd, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return nil
		},
	}
}

func publishCmd() *cobra.Command {
	var (
		gibsonURL      string
		timeout        time.Duration
		repository     string
		registrySecret string
	)
	c := &cobra.Command{
		Use:   "publish <folder>",
		Short: "Validate an Agent Skill folder and publish it to your tenant",
		Long: `Publish an Agent Skill folder to your tenant. The command does these steps:

1. Check the folder, as ` + "`gibson agent-skill validate`" + ` does.
2. Pack the folder as one OCI image. The digest depends on the content only.
3. Push the image by digest to the repository in --repository. The command
   uses your local docker credential for the registry.
4. Ask the daemon to pull the image by digest and store it.
5. Print the digest.

The registry is yours. The daemon has no write path to it. If the registry
needs a credential for a pull, store the credential as a tenant secret with
` + "`gibson secret set`" + ` and name the secret in --registry-secret. The secret
holds the bytes of a docker config. If the registry allows an anonymous pull,
leave --registry-secret empty.

A new publish of a name adds a version and makes it the current version.
Publish does not grant the Agent Skill to an agent. Authenticate first with
` + "`gibson login`" + `.`,
		Example: `  gibson agent-skill publish ./web-recon --repository registry.example.com/team/skills
  gibson agent-skill publish ./web-recon --repository registry.example.com/team/skills \
    --registry-secret cred:skills-registry`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublish(cmd, args[0], repository, registrySecret, gibsonURL, timeout)
		},
	}
	c.Flags().StringVar(&repository, "repository", "",
		"Registry repository to push to, as <registry>/<repository>, with no tag (required)")
	c.Flags().StringVar(&registrySecret, "registry-secret", "",
		"Name of the tenant secret that holds the registry credential for the daemon pull")
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "Deadline for the push and the daemon call")
	_ = c.MarkFlagRequired("repository")
	return c
}

// loadAndPack reads the folder, checks it and packs it. It prints each problem
// to stderr and returns an exit code 2 error when the folder is not valid.
func loadAndPack(cmd *cobra.Command, dir string) (*agentskill.Artifact, error) {
	name, files, readProblems, err := agentskill.ReadFolder(dir)
	if err != nil {
		return nil, err
	}
	_, problems := agentskill.Check(name, files)
	problems = append(readProblems, problems...)
	if len(problems) > 0 {
		w := cmd.ErrOrStderr()
		for _, p := range problems {
			_, _ = fmt.Fprintf(w, "error: %s\n", p)
		}
		_, _ = fmt.Fprintf(w, "\nvalidation failed: %d error(s)\n", len(problems))
		return nil, component.ExitCodeError{Code: 2}
	}
	return agentskill.Package(name, files)
}

func runPublish(cmd *cobra.Command, dir, repository, registrySecret, gibsonURL string, timeout time.Duration) error {
	repo, err := agentskill.ParseRepository(repository)
	if err != nil {
		return err
	}
	art, err := loadAndPack(cmd, dir)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	// Dial first. A missing login fails before the push, so the registry gets
	// no image that the daemon cannot take.
	conn, err := deviceauth.Dial(ctx, gibsonURL)
	if err != nil {
		return fmt.Errorf("connect to the daemon: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ref, err := agentskill.Push(ctx, repo, art)
	if err != nil {
		return err
	}
	resp, err := tenantv1.NewAgentSkillServiceClient(conn).PublishAgentSkill(ctx, &tenantv1.PublishAgentSkillRequest{
		Name:           art.Manifest.Name,
		Reference:      ref,
		RegistrySecret: registrySecret,
	})
	if err != nil {
		return fmt.Errorf("PublishAgentSkill: %w", err)
	}
	if got := resp.GetEntry().GetDigest(); got != art.Digest.String() {
		return fmt.Errorf("the daemon stored the digest %q, but the command pushed %q", got, art.Digest.String())
	}
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "name:      %s\n", art.Manifest.Name)
	_, _ = fmt.Fprintf(w, "digest:    %s\n", art.Digest.String())
	_, _ = fmt.Fprintf(w, "reference: %s\n", ref)
	return nil
}

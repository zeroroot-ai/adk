// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package secret groups the `gibson secret` subcommands: set, get, list,
// rotate, delete, count, and backend.
//
// A tenant secret is a name and bytes. Nothing else. There is no credential
// "type" or "shape" field, by decision: formats are not enumerable, a closed
// enum can never drop a value, and a label that can disagree with the bytes
// invites trust rather than earning it. A consumer that needs a kubeconfig
// parses the bytes as a kubeconfig and fails if they are not one, which is
// fail-closed and needs no shared vocabulary (ADR-0096).
//
// A Target does not name a secret either. Which credentials a piece of work may
// read is declared by its job, in JobSpec.credential_names, and the daemon
// resolves the name server-side at dispatch. Nothing here, and nothing a person
// types, puts a credential reference on a Target (ADR-0096).
//
// The commands call gibson.secrets.v1.SecretsService over the authenticated
// login session established by `gibson login` (bearer token only; the daemon
// derives the tenant from the token's Zitadel org, ADR-0093). The service lives
// in the OSS SDK as its own wire package because a developer supplying the
// credential their tool needs is authoring, not administering — the 2026-10-01
// amendment to ADR-0058.
//
// SECURITY: no subcommand ever prints a secret's value, and none accepts one as
// an argument. See the comment on valueFlags.
package secret

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"

	secretsv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/secrets/v1"
)

// Command returns the `gibson secret` parent command.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "secret",
		Short: "Store, list, rotate, and delete tenant secrets",
		Long: `gibson secret — manage the credentials your components use.

A secret is a NAME and BYTES. There is no type or format field: the thing that
reads a credential is the only party that knows what it needs, so it validates
by parsing (ADR-0096).

NAMES ARE EXACT. The name you give is the stored key, and it is also the
additional authenticated data of the secret's own envelope, so a name that
differs by one character is a different secret and a mistyped one is
unrecoverable. Nothing here trims or rewrites what you type.

Every name begins with a namespace:

  cred:<name>                         a credential a component uses
  provider_config:<provider>:<field>  an LLM provider setting

` + "`gibson secret list`" + ` prints exact keys, which are exactly what goes into a
mission's credential_names.

Subcommands:
  set      store a secret, from a file or stdin
  get      show one secret's metadata (never its value)
  list     list secret names and metadata (never values)
  rotate   store a new value for an existing secret
  delete   remove a secret
  count    how many secrets the active backend holds
  backend  show or configure which backend stores the secrets`,
		SilenceUsage: true,
	}
	c.AddCommand(setCmd())
	c.AddCommand(getCmd())
	c.AddCommand(listCmd())
	c.AddCommand(rotateCmd())
	c.AddCommand(deleteCmd())
	c.AddCommand(countCmd())
	c.AddCommand(backendCmd())
	return c
}

// connFlags are the daemon-dial flags shared by every subcommand.
type connFlags struct {
	gibsonURL string
	timeout   time.Duration
}

func (f *connFlags) bind(c *cobra.Command) {
	c.Flags().StringVar(&f.gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().DurationVar(&f.timeout, "timeout", 30*time.Second, "Request deadline")
}

// dial opens a SecretsService client over the authenticated login session.
// The daemon resolves the caller's tenant from the bearer token (ADR-0093).
func (f *connFlags) dial(ctx context.Context) (secretsv1.SecretsServiceClient, func(), context.Context, context.CancelFunc, error) {
	cctx, cancel := context.WithTimeout(ctx, f.timeout)
	conn, err := deviceauth.Dial(cctx, f.gibsonURL)
	if err != nil {
		cancel()
		return nil, nil, nil, nil, err
	}
	cleanup := func() { _ = conn.Close() }
	return secretsv1.NewSecretsServiceClient(conn), cleanup, cctx, cancel, nil
}

// categoryOf derives the SecretCategory from the name's namespace prefix.
//
// It is derived and never taken as a flag, on purpose. SetSecret takes a
// category AND a name, and the server prepends the category's prefix to a name
// that does not already carry it. A category that disagrees with the prefix
// therefore produces a double-prefixed key — storedName(CRED,
// "provider_config:openai:default") yields "cred:provider_config:openai:default"
// — and because the stored name is also the envelope's AAD, that key is not
// merely wrong, it is unfindable. Deriving from the prefix makes the two
// impossible to disagree.
func categoryOf(name string) (secretsv1.SecretCategory, error) {
	switch {
	case strings.HasPrefix(name, "cred:"):
		return secretsv1.SecretCategory_SECRET_CATEGORY_CRED, nil
	case strings.HasPrefix(name, "provider_config:"):
		return secretsv1.SecretCategory_SECRET_CATEGORY_PROVIDER_CONFIG, nil
	default:
		return secretsv1.SecretCategory_SECRET_CATEGORY_UNSPECIFIED,
			fmt.Errorf("name %q has no known namespace: begin it with %q or %q", name, "cred:", "provider_config:")
	}
}

// valueFlags reads a secret's bytes from a file or from stdin.
//
// There is deliberately NO --value flag. A secret passed as an argument is
// written to the shell's history file, is visible in `ps` output to every other
// process on the host for the lifetime of the call, and is captured verbatim by
// any CI system that echoes the command it ran. A file or a pipe has none of
// those properties.
type valueFlags struct {
	fromFile  string
	fromStdin bool
}

func (v *valueFlags) bind(c *cobra.Command) {
	c.Flags().StringVar(&v.fromFile, "from-file", "", "Read the value from this file")
	c.Flags().BoolVar(&v.fromStdin, "stdin", false, "Read the value from stdin")
}

func (v *valueFlags) read(cmd *cobra.Command) ([]byte, error) {
	switch {
	case v.fromFile != "" && v.fromStdin:
		return nil, errors.New("--from-file and --stdin are mutually exclusive")
	case v.fromFile != "":
		b, err := os.ReadFile(v.fromFile)
		if err != nil {
			return nil, fmt.Errorf("reading --from-file: %w", err)
		}
		if len(b) == 0 {
			return nil, fmt.Errorf("%s is empty: a secret with no value is a deletion, use `gibson secret delete`", v.fromFile)
		}
		return b, nil
	case v.fromStdin:
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		if len(b) == 0 {
			return nil, errors.New("stdin was empty: a secret with no value is a deletion, use `gibson secret delete`")
		}
		return b, nil
	default:
		return nil, errors.New("one of --from-file or --stdin is required (a secret is never passed as an argument: it would land in shell history and in ps output)")
	}
}

func setCmd() *cobra.Command {
	var (
		cf connFlags
		vf valueFlags
	)
	c := &cobra.Command{
		Use:   "set <name>",
		Short: "Store a secret, from a file or stdin",
		Long: `Store a secret under an exact name.

The name is the stored key and the envelope's AAD, so it is used verbatim —
nothing is trimmed. The namespace prefix determines the category; there is no
--category flag, because a category that disagreed with the prefix would write
a key nobody can read.

  gibson secret set cred:goat-cluster --from-file ~/.kube/goat.yaml
  kubectl config view --raw | gibson secret set cred:goat-cluster --stdin`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cat, err := categoryOf(name)
			if err != nil {
				return err
			}
			value, err := vf.read(cmd)
			if err != nil {
				return err
			}
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.SetSecret(ctx, &secretsv1.SetSecretRequest{
				Name:     name,
				Category: cat,
				Value:    value,
			})
			if err != nil {
				return fmt.Errorf("SetSecret: %w", err)
			}
			md := resp.GetMetadata()
			fmt.Fprintf(cmd.OutOrStdout(), "%s (version %d)\n", md.GetName(), md.GetVersion())
			return nil
		},
	}
	cf.bind(c)
	vf.bind(c)
	return c
}

func rotateCmd() *cobra.Command {
	var (
		cf connFlags
		vf valueFlags
	)
	c := &cobra.Command{
		Use:   "rotate <name>",
		Short: "Store a new value for an existing secret",
		Long: `Store a new value under an existing name, creating a new version.

Rotation keeps the name, which matters: the name is what a job's
credential_names declares, so rotating a credential needs no change anywhere
else.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := vf.read(cmd)
			if err != nil {
				return err
			}
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.RotateSecret(ctx, &secretsv1.RotateSecretRequest{
				Name:  args[0],
				Value: value,
			})
			if err != nil {
				return fmt.Errorf("RotateSecret: %w", err)
			}
			md := resp.GetMetadata()
			fmt.Fprintf(cmd.OutOrStdout(), "%s (version %d)\n", md.GetName(), md.GetVersion())
			return nil
		},
	}
	cf.bind(c)
	vf.bind(c)
	return c
}

func getCmd() *cobra.Command {
	var cf connFlags
	c := &cobra.Command{
		Use:   "get <name>",
		Short: "Show one secret's metadata (never its value)",
		Long: `Show a secret's metadata.

This never prints the value, and not because the CLI withholds it: the RPC does
not return it. The only RPC in Gibson that returns a plaintext credential is the
harness callback a sandboxed component uses, which is not callable from here.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.GetSecret(ctx, &secretsv1.GetSecretRequest{Name: args[0]})
			if err != nil {
				return fmt.Errorf("GetSecret: %w", err)
			}
			printMetadata(cmd, resp.GetMetadata())
			return nil
		},
	}
	cf.bind(c)
	return c
}

func printMetadata(cmd *cobra.Command, md *secretsv1.SecretMetadata) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\t%s\n", md.GetName())
	fmt.Fprintf(w, "CATEGORY\t%s\n", md.GetCategory())
	fmt.Fprintf(w, "VERSION\t%d\n", md.GetVersion())
	fmt.Fprintf(w, "CREATED\t%s\tby %s\n", unixOrDash(md.GetCreatedAtUnix()), md.GetCreatedBy())
	fmt.Fprintf(w, "UPDATED\t%s\tby %s\n", unixOrDash(md.GetUpdatedAtUnix()), md.GetUpdatedBy())
	fmt.Fprintf(w, "LAST READ\t%s\n", unixOrDash(md.GetLastAccessedAtUnix()))
	if pa := md.GetPluginAssociations(); len(pa) > 0 {
		fmt.Fprintf(w, "PLUGINS\t%s\n", strings.Join(pa, ", "))
	}
	_ = w.Flush()
}

func unixOrDash(sec int64) string {
	if sec == 0 {
		return "-"
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func listCmd() *cobra.Command {
	var (
		cf     connFlags
		prefix string
		limit  int32
		offset int32
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List secret names and metadata (never values)",
		Long: `List the tenant's secrets.

The names printed are exact stored keys, which is what a mission's
credential_names needs, so they can be copied without translation.

  gibson secret list
  gibson secret list --prefix cred:`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.ListSecrets(ctx, &secretsv1.ListSecretsRequest{
				NamePrefix: prefix,
				Limit:      limit,
				Offset:     offset,
			})
			if err != nil {
				return fmt.Errorf("ListSecrets: %w", err)
			}
			secrets := resp.GetSecrets()
			if len(secrets) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no secrets")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tCATEGORY\tVERSION\tUPDATED\tUPDATED BY")
			for _, md := range secrets {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
					md.GetName(), md.GetCategory(), md.GetVersion(),
					unixOrDash(md.GetUpdatedAtUnix()), md.GetUpdatedBy())
			}
			_ = w.Flush()
			fmt.Fprintf(cmd.ErrOrStderr(), "\n%d shown, %d total\n", len(secrets), resp.GetTotal())
			return nil
		},
	}
	cf.bind(c)
	c.Flags().StringVar(&prefix, "prefix", "", "Only names beginning with this prefix (e.g. cred:)")
	c.Flags().Int32Var(&limit, "limit", 0, "Maximum number to return (0 = server default)")
	c.Flags().Int32Var(&offset, "offset", 0, "Number to skip")
	return c
}

func deleteCmd() *cobra.Command {
	var (
		cf  connFlags
		yes bool
	)
	c := &cobra.Command{
		Use:   "delete <name>",
		Short: "Remove a secret",
		Long: `Remove a secret.

A job whose credential_names still declares this name will fail to resolve it
once it is gone, so check with ` + "`gibson secret get`" + ` before deleting a name you did
not create.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("refusing to delete %q without --yes", args[0])
			}
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			if _, err := client.DeleteSecret(ctx, &secretsv1.DeleteSecretRequest{Name: args[0]}); err != nil {
				return fmt.Errorf("DeleteSecret: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return nil
		},
	}
	cf.bind(c)
	c.Flags().BoolVar(&yes, "yes", false, "Confirm the deletion")
	return c
}

func countCmd() *cobra.Command {
	var cf connFlags
	c := &cobra.Command{
		Use:   "count",
		Short: "How many secrets the active backend holds",
		Long: `Count the secrets in the tenant's active backend.

This asks the backend rather than the metadata index, so it is the check for
whether a backend swap moved everything it should have.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.CountSecrets(ctx, &secretsv1.CountSecretsRequest{})
			if err != nil {
				return fmt.Errorf("CountSecrets: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), resp.GetCount())
			return nil
		},
	}
	cf.bind(c)
	return c
}

// backendCmd groups the broker-configuration subcommands.
//
// These are here rather than on a separate admin surface because `gibson secret`
// has to do all of the work: a developer whose tenant has no backend configured
// cannot store a secret at all, so a CRUD-only command group would strand them
// (the 2026-10-01 amendment to ADR-0058).
func backendCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "backend",
		Short: "Show or configure which backend stores the secrets",
		Long: `gibson secret backend — which store the tenant's secrets live in.

Two backends exist. Every other provider was retired and its enum value
reserved, so there is no AWS, GCP or Azure option to choose:

  hosted  the platform-managed OpenBao, isolated per tenant by namespace.
          Zero configuration, and already active.
  byo     your own Vault or OpenBao, isolated by path prefix because OSS
          Vault and OpenBao CE have no namespaces.

Subcommands:
  show   print the active configuration, with sensitive fields redacted
  probe  validate a candidate configuration WITHOUT saving it
  set    probe a candidate configuration and save it on success`,
		SilenceUsage: true,
	}
	c.AddCommand(backendShowCmd())
	c.AddCommand(backendProbeCmd())
	c.AddCommand(backendSetCmd())
	return c
}

func backendShowCmd() *cobra.Command {
	var cf connFlags
	c := &cobra.Command{
		Use:   "show",
		Short: "Print the active backend configuration, redacted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.GetBrokerConfig(ctx, &secretsv1.GetBrokerConfigRequest{})
			if err != nil {
				return fmt.Errorf("GetBrokerConfig: %w", err)
			}
			if !resp.GetConfigured() {
				fmt.Fprintln(cmd.ErrOrStderr(), "no backend configured for this tenant")
				return nil
			}
			cfg := resp.GetConfig()
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "PROVIDER\t%s\n", cfg.GetProvider())
			writeIfSet(w, "ADDRESS", cfg.GetAddress())
			writeIfSet(w, "NAMESPACE/PATH", cfg.GetNamespaceOrPath())
			writeIfSet(w, "MOUNT", cfg.GetMount())
			writeIfSet(w, "AUTH METHOD", cfg.GetAuthMethod())
			// sensitive_fields_set names which secrets the server HOLDS, never
			// their values. It is the only way to tell a missing credential from
			// a wrong one without sending the credential back.
			if s := cfg.GetSensitiveFieldsSet(); len(s) > 0 {
				fmt.Fprintf(w, "SECRETS STORED\t%s\n", strings.Join(s, ", "))
			}
			fmt.Fprintf(w, "UPDATED\t%s\tby %s\n", unixOrDash(cfg.GetUpdatedAtUnix()), cfg.GetUpdatedBy())
			_ = w.Flush()
			return nil
		},
	}
	cf.bind(c)
	return c
}

func writeIfSet(w io.Writer, label, value string) {
	if value != "" {
		fmt.Fprintf(w, "%s\t%s\n", label, value)
	}
}

// candidateFlags are the fields a live broker provider actually needs.
//
// CandidateConfig still carries region, project, client_id, role_arn and the
// AWS/GCP/Azure credential fields, left from the providers ADR retired and whose
// enum values are now reserved. They are not exposed here: a flag for a provider
// that cannot be selected is a flag that can only mislead.
type candidateFlags struct {
	provider     string
	address      string
	pathPrefix   string
	mount        string
	authMethod   string
	tokenFile    string
	roleID       string
	secretIDFile string
}

func (v *candidateFlags) bind(c *cobra.Command) {
	c.Flags().StringVar(&v.provider, "provider", "", "hosted or byo")
	c.Flags().StringVar(&v.address, "address", "", "Vault/OpenBao address (byo)")
	c.Flags().StringVar(&v.pathPrefix, "path-prefix", "", "Path prefix that isolates this tenant (byo)")
	c.Flags().StringVar(&v.mount, "mount", "", "KV mount (byo)")
	c.Flags().StringVar(&v.authMethod, "auth-method", "", "token or approle (byo)")
	c.Flags().StringVar(&v.tokenFile, "token-file", "", "File holding the Vault token (auth-method token)")
	c.Flags().StringVar(&v.roleID, "approle-role-id", "", "AppRole role id (auth-method approle)")
	c.Flags().StringVar(&v.secretIDFile, "approle-secret-id-file", "", "File holding the AppRole secret id (auth-method approle)")
}

// build turns the flags into a CandidateConfig. Credentials are read from files
// for the same reason secret values are: an argument reaches shell history and
// ps output.
func (v *candidateFlags) build() (*secretsv1.CandidateConfig, error) {
	cc := &secretsv1.CandidateConfig{
		Address:         v.address,
		NamespaceOrPath: v.pathPrefix,
		Mount:           v.mount,
		AuthMethod:      v.authMethod,
	}
	switch v.provider {
	case "hosted":
		cc.Provider = secretsv1.BrokerProvider_BROKER_PROVIDER_VAULT_HOSTED
	case "byo":
		cc.Provider = secretsv1.BrokerProvider_BROKER_PROVIDER_VAULT_BYO
	case "":
		return nil, errors.New("--provider is required (hosted or byo)")
	default:
		return nil, fmt.Errorf("unknown --provider %q: hosted or byo", v.provider)
	}
	if v.tokenFile != "" {
		b, err := os.ReadFile(v.tokenFile)
		if err != nil {
			return nil, fmt.Errorf("reading --token-file: %w", err)
		}
		cc.VaultToken = b
	}
	if v.roleID != "" {
		cc.ApproleRoleId = v.roleID
	}
	if v.secretIDFile != "" {
		b, err := os.ReadFile(v.secretIDFile)
		if err != nil {
			return nil, fmt.Errorf("reading --approle-secret-id-file: %w", err)
		}
		cc.ApproleSecretId = b
	}
	return cc, nil
}

func backendProbeCmd() *cobra.Command {
	var (
		cf connFlags
		vf candidateFlags
	)
	c := &cobra.Command{
		Use:   "probe",
		Short: "Validate a candidate configuration without saving it",
		Long: `Check that a candidate backend configuration works, and change nothing.

Run this before ` + "`set`" + `. A backend that cannot be reached and is saved anyway
leaves every secret unreadable until it is fixed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc, err := vf.build()
			if err != nil {
				return err
			}
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.ProbeBrokerConfig(ctx, &secretsv1.ProbeBrokerConfigRequest{Candidate: cc})
			if err != nil {
				return fmt.Errorf("ProbeBrokerConfig: %w", err)
			}
			return reportProbe(cmd, resp.GetResult())
		},
	}
	cf.bind(c)
	vf.bind(c)
	return c
}

func backendSetCmd() *cobra.Command {
	var (
		cf connFlags
		vf candidateFlags
	)
	c := &cobra.Command{
		Use:   "set",
		Short: "Probe a candidate configuration and save it on success",
		Long: `Save a backend configuration, after the server probes it.

The server probes before it persists, so a configuration that cannot be reached
is refused rather than stored.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc, err := vf.build()
			if err != nil {
				return err
			}
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.SetBrokerConfig(ctx, &secretsv1.SetBrokerConfigRequest{Candidate: cc})
			if err != nil {
				return fmt.Errorf("SetBrokerConfig: %w", err)
			}
			if err := reportProbe(cmd, resp.GetProbeResult()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "saved")
			return nil
		},
	}
	cf.bind(c)
	vf.bind(c)
	return c
}

// reportProbe prints a probe result and fails the command when the probe did.
// A probe that reports failure must not exit 0: a script that configures a
// backend has to be able to tell.
func reportProbe(cmd *cobra.Command, r *secretsv1.ProbeResult) error {
	if r == nil {
		return errors.New("server returned no probe result")
	}
	if r.GetOk() {
		fmt.Fprintf(cmd.OutOrStdout(), "probe ok (%dms)\n", r.GetDurationMs())
		return nil
	}
	// error_class is the server's category for the failure and error_message is
	// its detail. Both are printed: the class is what a script can branch on,
	// the message is what a person needs.
	if c := r.GetErrorClass(); c != "" {
		return fmt.Errorf("probe failed [%s]: %s", c, r.GetErrorMessage())
	}
	return fmt.Errorf("probe failed: %s", r.GetErrorMessage())
}

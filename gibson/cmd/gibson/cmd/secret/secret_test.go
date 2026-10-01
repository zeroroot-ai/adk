// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package secret

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	secretsv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/secrets/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeSecretsServer stubs only the RPCs a test needs; the embed keeps it
// forward-compatible when the service gains one.
type fakeSecretsServer struct {
	secretsv1.UnimplementedSecretsServiceServer

	setFn    func(context.Context, *secretsv1.SetSecretRequest) (*secretsv1.SetSecretResponse, error)
	getFn    func(context.Context, *secretsv1.GetSecretRequest) (*secretsv1.GetSecretResponse, error)
	listFn   func(context.Context, *secretsv1.ListSecretsRequest) (*secretsv1.ListSecretsResponse, error)
	rotateFn func(context.Context, *secretsv1.RotateSecretRequest) (*secretsv1.RotateSecretResponse, error)
	deleteFn func(context.Context, *secretsv1.DeleteSecretRequest) (*secretsv1.DeleteSecretResponse, error)
	probeFn  func(context.Context, *secretsv1.ProbeBrokerConfigRequest) (*secretsv1.ProbeBrokerConfigResponse, error)
}

func (s *fakeSecretsServer) SetSecret(ctx context.Context, r *secretsv1.SetSecretRequest) (*secretsv1.SetSecretResponse, error) {
	return s.setFn(ctx, r)
}

func (s *fakeSecretsServer) GetSecret(ctx context.Context, r *secretsv1.GetSecretRequest) (*secretsv1.GetSecretResponse, error) {
	return s.getFn(ctx, r)
}

func (s *fakeSecretsServer) ListSecrets(ctx context.Context, r *secretsv1.ListSecretsRequest) (*secretsv1.ListSecretsResponse, error) {
	return s.listFn(ctx, r)
}

func (s *fakeSecretsServer) RotateSecret(ctx context.Context, r *secretsv1.RotateSecretRequest) (*secretsv1.RotateSecretResponse, error) {
	return s.rotateFn(ctx, r)
}

func (s *fakeSecretsServer) DeleteSecret(ctx context.Context, r *secretsv1.DeleteSecretRequest) (*secretsv1.DeleteSecretResponse, error) {
	return s.deleteFn(ctx, r)
}

func (s *fakeSecretsServer) ProbeBrokerConfig(ctx context.Context, r *secretsv1.ProbeBrokerConfigRequest) (*secretsv1.ProbeBrokerConfigResponse, error) {
	return s.probeFn(ctx, r)
}

func serve(t *testing.T, svc secretsv1.SecretsServiceServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	secretsv1.RegisterSecretsServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

// run executes `secret <args...>` against a fake daemon through the normal
// authenticated session path, the same way the target tests do.
func run(t *testing.T, addr string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	// gosec G101 fires on AccessToken below. It is a fake token for a fake
	// in-process server on a loopback port, which gosec cannot distinguish from
	// a real one. Suppressed at the struct rather than the file, so a genuine
	// credential added elsewhere in this file still trips it.
	creds := &deviceauth.Credentials{ //nolint:gosec // G101: fake token for a test server
		Issuer:       "http://127.0.0.1:1",
		ClientID:     "gibson-cli",
		TokenURL:     "http://127.0.0.1:1/token",
		AccessToken:  "test-access-token",
		Expiry:       time.Now().Add(time.Hour),
		ActiveTenant: "tenant-test",
		GibsonURL:    "http://" + addr,
	}
	require.NoError(t, creds.Save())

	cmd := Command()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestCommand_WiresSubcommands(t *testing.T) {
	want := map[string]bool{
		"set": false, "get": false, "list": false,
		"rotate": false, "delete": false, "count": false, "backend": false,
	}
	for _, sub := range Command().Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("subcommand %q not registered under `gibson secret`", name)
		}
	}
}

// TestNoValueFlagExists is a design guard, not a behaviour test. A secret passed
// as an argument reaches the shell's history file and every process that can
// read ps output. If someone adds --value for convenience, this fails.
func TestNoValueFlagExists(t *testing.T) {
	for _, sub := range Command().Commands() {
		for _, banned := range []string{"value", "secret", "password", "token"} {
			if f := sub.Flags().Lookup(banned); f != nil {
				t.Errorf("`gibson secret %s` has a --%s flag: a secret must never be an argument", sub.Name(), banned)
			}
		}
	}
}

func TestCategoryOf(t *testing.T) {
	cases := []struct {
		name    string
		want    secretsv1.SecretCategory
		wantErr bool
	}{
		{name: "cred:goat-cluster", want: secretsv1.SecretCategory_SECRET_CATEGORY_CRED},
		{name: "provider_config:openai:default", want: secretsv1.SecretCategory_SECRET_CATEGORY_PROVIDER_CONFIG},
		{name: "goat-cluster", wantErr: true},
		{name: "", wantErr: true},
		// A name that merely CONTAINS a namespace is not in it.
		{name: "mine-cred:x", wantErr: true},
	}
	for _, tc := range cases {
		got, err := categoryOf(tc.name)
		if tc.wantErr {
			if err == nil {
				t.Errorf("categoryOf(%q) = %v, want an error", tc.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("categoryOf(%q): %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("categoryOf(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestSet_SendsNameVerbatimAndDerivesCategory is the corruption guard. The
// stored name is the envelope's AAD, so the CLI must send exactly what it was
// given, and the category must match the name's own prefix — a category that
// disagreed would make the server write a double-prefixed key nobody can read.
func TestSet_SendsNameVerbatimAndDerivesCategory(t *testing.T) {
	cases := []struct {
		arg      string
		wantName string
		wantCat  secretsv1.SecretCategory
	}{
		{"cred:goat-cluster", "cred:goat-cluster", secretsv1.SecretCategory_SECRET_CATEGORY_CRED},
		{"provider_config:openai:default", "provider_config:openai:default", secretsv1.SecretCategory_SECRET_CATEGORY_PROVIDER_CONFIG},
		// Trailing dot and mixed case are part of the key, not noise to tidy.
		{"cred:Goat.Cluster", "cred:Goat.Cluster", secretsv1.SecretCategory_SECRET_CATEGORY_CRED},
	}
	for _, tc := range cases {
		var gotName string
		var gotCat secretsv1.SecretCategory
		var gotValue []byte
		addr := serve(t, &fakeSecretsServer{
			setFn: func(_ context.Context, r *secretsv1.SetSecretRequest) (*secretsv1.SetSecretResponse, error) {
				gotName, gotCat, gotValue = r.GetName(), r.GetCategory(), r.GetValue()
				return &secretsv1.SetSecretResponse{
					Metadata: &secretsv1.SecretMetadata{Name: r.GetName(), Version: 1},
				}, nil
			},
		})
		f := filepath.Join(t.TempDir(), "v")
		require.NoError(t, os.WriteFile(f, []byte("kubeconfig-bytes"), 0o600))

		_, err := run(t, addr, "set", tc.arg, "--from-file", f)
		require.NoError(t, err)
		if gotName != tc.wantName {
			t.Errorf("sent name %q, want %q — the name is the AAD and must not be rewritten", gotName, tc.wantName)
		}
		if gotCat != tc.wantCat {
			t.Errorf("sent category %v for %q, want %v", gotCat, tc.arg, tc.wantCat)
		}
		if string(gotValue) != "kubeconfig-bytes" {
			t.Errorf("sent value %q, want the file's bytes", gotValue)
		}
	}
}

func TestSet_RejectsUnprefixedName(t *testing.T) {
	f := filepath.Join(t.TempDir(), "v")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	// No server: the name is rejected before anything dials.
	out, err := run(t, "127.0.0.1:1", "set", "goat-cluster", "--from-file", f)
	require.Error(t, err)
	require.Contains(t, out+err.Error(), "no known namespace")
}

func TestSet_RequiresAValueSource(t *testing.T) {
	out, err := run(t, "127.0.0.1:1", "set", "cred:x")
	require.Error(t, err)
	require.Contains(t, out+err.Error(), "--from-file or --stdin")
}

func TestSet_RejectsBothValueSources(t *testing.T) {
	f := filepath.Join(t.TempDir(), "v")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	out, err := run(t, "127.0.0.1:1", "set", "cred:x", "--from-file", f, "--stdin")
	require.Error(t, err)
	require.Contains(t, out+err.Error(), "mutually exclusive")
}

// TestSet_RejectsEmptyValue matters because an empty write would silently
// replace a working credential with nothing, which reads as "rotated" in the
// audit log.
func TestSet_RejectsEmptyValue(t *testing.T) {
	f := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.WriteFile(f, nil, 0o600))
	out, err := run(t, "127.0.0.1:1", "set", "cred:x", "--from-file", f)
	require.Error(t, err)
	require.Contains(t, out+err.Error(), "empty")
}

// TestGet_PrintsMetadataAndNoValue pins that nothing in the output could be a
// credential. The RPC cannot return one, and this asserts the CLI does not
// invent a field that looks like one.
func TestGet_PrintsMetadataAndNoValue(t *testing.T) {
	addr := serve(t, &fakeSecretsServer{
		getFn: func(_ context.Context, r *secretsv1.GetSecretRequest) (*secretsv1.GetSecretResponse, error) {
			return &secretsv1.GetSecretResponse{Metadata: &secretsv1.SecretMetadata{
				Name:      r.GetName(),
				Category:  secretsv1.SecretCategory_SECRET_CATEGORY_CRED,
				Version:   3,
				CreatedBy: "someone@example.com",
			}}, nil
		},
	})
	out, err := run(t, addr, "get", "cred:goat-cluster")
	require.NoError(t, err)
	require.Contains(t, out, "cred:goat-cluster")
	require.Contains(t, out, "3")
	for _, banned := range []string{"VALUE", "PLAINTEXT", "SECRET\t"} {
		if strings.Contains(out, banned) {
			t.Errorf("get output contains %q; it must never present a value", banned)
		}
	}
}

// TestList_PrintsExactStoredKeys is why list exists in this shape: its output is
// pasted into a mission's credential_names, so a prettified name would be wrong.
func TestList_PrintsExactStoredKeys(t *testing.T) {
	addr := serve(t, &fakeSecretsServer{
		listFn: func(_ context.Context, _ *secretsv1.ListSecretsRequest) (*secretsv1.ListSecretsResponse, error) {
			return &secretsv1.ListSecretsResponse{
				Secrets: []*secretsv1.SecretMetadata{
					{Name: "cred:goat-cluster", Version: 2},
					{Name: "provider_config:openai:default", Version: 1},
				},
				Total: 2,
			}, nil
		},
	})
	out, err := run(t, addr, "list")
	require.NoError(t, err)
	require.Contains(t, out, "cred:goat-cluster")
	require.Contains(t, out, "provider_config:openai:default")
}

func TestList_PassesPrefixThrough(t *testing.T) {
	var gotPrefix string
	addr := serve(t, &fakeSecretsServer{
		listFn: func(_ context.Context, r *secretsv1.ListSecretsRequest) (*secretsv1.ListSecretsResponse, error) {
			gotPrefix = r.GetNamePrefix()
			return &secretsv1.ListSecretsResponse{}, nil
		},
	})
	_, err := run(t, addr, "list", "--prefix", "cred:")
	require.NoError(t, err)
	require.Equal(t, "cred:", gotPrefix)
}

func TestDelete_RefusesWithoutYes(t *testing.T) {
	called := false
	addr := serve(t, &fakeSecretsServer{
		deleteFn: func(_ context.Context, _ *secretsv1.DeleteSecretRequest) (*secretsv1.DeleteSecretResponse, error) {
			called = true
			return &secretsv1.DeleteSecretResponse{}, nil
		},
	})
	_, err := run(t, addr, "delete", "cred:x")
	require.Error(t, err)
	require.False(t, called, "delete reached the server without --yes")
}

func TestDelete_SendsNameVerbatimWithYes(t *testing.T) {
	var got string
	addr := serve(t, &fakeSecretsServer{
		deleteFn: func(_ context.Context, r *secretsv1.DeleteSecretRequest) (*secretsv1.DeleteSecretResponse, error) {
			got = r.GetName()
			return &secretsv1.DeleteSecretResponse{}, nil
		},
	})
	_, err := run(t, addr, "delete", "cred:goat-cluster", "--yes")
	require.NoError(t, err)
	require.Equal(t, "cred:goat-cluster", got)
}

func TestRotate_SendsNameAndValue(t *testing.T) {
	var gotName string
	var gotValue []byte
	addr := serve(t, &fakeSecretsServer{
		rotateFn: func(_ context.Context, r *secretsv1.RotateSecretRequest) (*secretsv1.RotateSecretResponse, error) {
			gotName, gotValue = r.GetName(), r.GetValue()
			return &secretsv1.RotateSecretResponse{
				Metadata: &secretsv1.SecretMetadata{Name: r.GetName(), Version: 4},
			}, nil
		},
	})
	f := filepath.Join(t.TempDir(), "v")
	require.NoError(t, os.WriteFile(f, []byte("new-bytes"), 0o600))
	out, err := run(t, addr, "rotate", "cred:goat-cluster", "--from-file", f)
	require.NoError(t, err)
	require.Equal(t, "cred:goat-cluster", gotName)
	require.Equal(t, "new-bytes", string(gotValue))
	require.Contains(t, out, "version 4")
}

func TestBackend_RejectsUnknownProvider(t *testing.T) {
	out, err := run(t, "127.0.0.1:1", "backend", "probe", "--provider", "awssm")
	require.Error(t, err)
	require.Contains(t, out+err.Error(), "hosted or byo")
}

func TestBackend_RequiresProvider(t *testing.T) {
	out, err := run(t, "127.0.0.1:1", "backend", "probe")
	require.Error(t, err)
	require.Contains(t, out+err.Error(), "--provider is required")
}

// TestBackendProbe_FailedProbeIsNonZero matters for scripting: a probe that
// reports failure while exiting 0 would let a bringup continue onto a backend
// that cannot be reached.
func TestBackendProbe_FailedProbeIsNonZero(t *testing.T) {
	addr := serve(t, &fakeSecretsServer{
		probeFn: func(_ context.Context, _ *secretsv1.ProbeBrokerConfigRequest) (*secretsv1.ProbeBrokerConfigResponse, error) {
			return &secretsv1.ProbeBrokerConfigResponse{
				Result: &secretsv1.ProbeResult{Ok: false, ErrorClass: "unreachable", ErrorMessage: "connection refused"},
			}, nil
		},
	})
	out, err := run(t, addr, "backend", "probe", "--provider", "byo", "--address", "http://127.0.0.1:1")
	require.Error(t, err)
	require.Contains(t, out+err.Error(), "connection refused")
}

func TestBackendProbe_OkIsZero(t *testing.T) {
	addr := serve(t, &fakeSecretsServer{
		probeFn: func(_ context.Context, _ *secretsv1.ProbeBrokerConfigRequest) (*secretsv1.ProbeBrokerConfigResponse, error) {
			return &secretsv1.ProbeBrokerConfigResponse{Result: &secretsv1.ProbeResult{Ok: true}}, nil
		},
	})
	out, err := run(t, addr, "backend", "probe", "--provider", "hosted")
	require.NoError(t, err)
	require.Contains(t, out, "probe ok")
}

// TestSet_SurfacesServerRefusal covers the whitespace case the server owns: the
// CLI does not pre-validate it, so the refusal has to reach the user intact.
func TestSet_SurfacesServerRefusal(t *testing.T) {
	addr := serve(t, &fakeSecretsServer{
		setFn: func(_ context.Context, _ *secretsv1.SetSecretRequest) (*secretsv1.SetSecretResponse, error) {
			return nil, status.Error(codes.InvalidArgument, "secret name has leading or trailing whitespace")
		},
	})
	f := filepath.Join(t.TempDir(), "v")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	out, err := run(t, addr, "set", "cred: spaced ", "--from-file", f)
	require.Error(t, err)
	require.Contains(t, out+err.Error(), "whitespace")
}

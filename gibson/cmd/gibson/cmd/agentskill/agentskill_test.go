// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package agentskill

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/cmd/component"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	tenantv1 "github.com/zeroroot-ai/adk/gibson/internal/genproto/gibson/tenant/v1"
)

// goldenDigest is the digest of writeSkill's folder. The gibson daemon gets the
// same digest for the same content. See the golden test in the internal
// agentskill package.
const goldenDigest = "sha256:d585fe5073f5a1078419e2ab03f1ee00ca8e9256063459a47ee205d054a61e97"

// writeSkill writes the shared fixture folder and returns its path.
func writeSkill(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "web-recon")
	files := map[string]string{
		"SKILL.md": "---\nname: web-recon\ndescription: Map a web target before a scan.\n" +
			"license: Apache-2.0\ncompatibility: Needs httpx.\nmetadata:\n  author: acme\n" +
			"allowed-tools: native:httpx native:nmap\n---\n# Steps\nRun httpx first.\n",
		"scripts/run.sh":      "#!/bin/sh\necho recon\n",
		"references/guide.md": "# Guide\nUse the flags in the order shown.\n",
	}
	for p, c := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
		require.NoError(t, os.WriteFile(full, []byte(c), 0o600))
	}
	return dir
}

type fakeServer struct {
	tenantv1.UnimplementedAgentSkillServiceServer
	publishFn func(context.Context, *tenantv1.PublishAgentSkillRequest) (*tenantv1.PublishAgentSkillResponse, error)
}

func (s *fakeServer) PublishAgentSkill(ctx context.Context, r *tenantv1.PublishAgentSkillRequest) (*tenantv1.PublishAgentSkillResponse, error) {
	return s.publishFn(ctx, r)
}

func serve(t *testing.T, svc tenantv1.AgentSkillServiceServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	tenantv1.RegisterAgentSkillServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

func login(t *testing.T, addr string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
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
}

func execute(args ...string) (stdout, stderr string, err error) {
	cmd := Command()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errBuf.String(), err
}

func TestCommand_WiresSubcommands(t *testing.T) {
	got := map[string]bool{}
	for _, sub := range Command().Commands() {
		got[sub.Name()] = true
	}
	require.True(t, got["validate"])
	require.True(t, got["publish"])
}

func TestValidate_Ok(t *testing.T) {
	out, _, err := execute("validate", writeSkill(t))
	require.NoError(t, err)
	require.Equal(t, "ok\n", out)
}

func TestValidate_ReportsEachProblemAndExitsWithTwo(t *testing.T) {
	dir := writeSkill(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: other\n---\nbody\n"), 0o600))
	out, errText, err := execute("validate", dir)
	var exit component.ExitCodeError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 2, exit.Code)
	require.Empty(t, out)
	require.Contains(t, errText, "description is required")
	require.Contains(t, errText, "does not match the folder name")
	require.Contains(t, errText, "validation failed: 2 error(s)")
}

func TestValidate_NeedsNoLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, _, err := execute("validate", writeSkill(t))
	require.NoError(t, err)
}

func TestValidate_MissingFolder(t *testing.T) {
	_, _, err := execute("validate", filepath.Join(t.TempDir(), "none"))
	require.Error(t, err)
}

func TestPublish_PushesThenCallsTheDaemon(t *testing.T) {
	reg := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(reg.Close)
	host := strings.TrimPrefix(reg.URL, "http://")

	var got *tenantv1.PublishAgentSkillRequest
	addr := serve(t, &fakeServer{publishFn: func(_ context.Context, r *tenantv1.PublishAgentSkillRequest) (*tenantv1.PublishAgentSkillResponse, error) {
		got = r
		return &tenantv1.PublishAgentSkillResponse{Entry: &tenantv1.AgentSkillEntry{Name: r.GetName(), Digest: goldenDigest}}, nil
	}})
	login(t, addr)

	out, _, err := execute("publish", writeSkill(t),
		"--repository", host+"/team/skills", "--registry-secret", "cred:skills-registry")
	require.NoError(t, err)
	require.Contains(t, out, "digest:    "+goldenDigest)
	require.NotNil(t, got)
	require.Equal(t, "web-recon", got.GetName())
	require.Equal(t, host+"/team/skills@"+goldenDigest, got.GetReference())
	require.Equal(t, "cred:skills-registry", got.GetRegistrySecret())
}

func TestPublish_RefusesAnInvalidFolderBeforeAnyCall(t *testing.T) {
	called := false
	addr := serve(t, &fakeServer{publishFn: func(context.Context, *tenantv1.PublishAgentSkillRequest) (*tenantv1.PublishAgentSkillResponse, error) {
		called = true
		return nil, nil
	}})
	login(t, addr)
	dir := writeSkill(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "SKILL.md")))
	_, errText, err := execute("publish", dir, "--repository", "127.0.0.1:1/team/skills")
	var exit component.ExitCodeError
	require.ErrorAs(t, err, &exit)
	require.Contains(t, errText, "no SKILL.md")
	require.False(t, called)
}

func TestPublish_RefusesATagInTheRepository(t *testing.T) {
	_, _, err := execute("publish", writeSkill(t), "--repository", "127.0.0.1:1/team/skills:v1")
	require.ErrorContains(t, err, "must not have a tag")
}

func TestPublish_NeedsTheRepositoryFlag(t *testing.T) {
	_, _, err := execute("publish", writeSkill(t))
	require.ErrorContains(t, err, "repository")
}

func TestPublish_NeedsALogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, _, err := execute("publish", writeSkill(t), "--repository", "127.0.0.1:1/team/skills")
	require.Error(t, err)
}

func TestPublish_ShowsADaemonRefusal(t *testing.T) {
	reg := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(reg.Close)
	addr := serve(t, &fakeServer{publishFn: func(context.Context, *tenantv1.PublishAgentSkillRequest) (*tenantv1.PublishAgentSkillResponse, error) {
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}})
	login(t, addr)
	_, _, err := execute("publish", writeSkill(t), "--repository", strings.TrimPrefix(reg.URL, "http://")+"/team/skills")
	require.ErrorContains(t, err, "PublishAgentSkill")
	require.ErrorContains(t, err, "admin only")
}

func TestPublish_RefusesADigestThatDiffers(t *testing.T) {
	reg := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(reg.Close)
	addr := serve(t, &fakeServer{publishFn: func(context.Context, *tenantv1.PublishAgentSkillRequest) (*tenantv1.PublishAgentSkillResponse, error) {
		return &tenantv1.PublishAgentSkillResponse{Entry: &tenantv1.AgentSkillEntry{Digest: "sha256:" + strings.Repeat("0", 64)}}, nil
	}})
	login(t, addr)
	out, _, err := execute("publish", writeSkill(t), "--repository", strings.TrimPrefix(reg.URL, "http://")+"/team/skills")
	require.ErrorContains(t, err, "the daemon stored the digest")
	require.Empty(t, out)
}

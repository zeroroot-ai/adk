// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package agentskill_test

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/agentskill"
)

// writeGolden writes the shared fixture to <root>/web-recon and returns it.
func writeGolden(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "web-recon")
	for _, f := range goldenFiles() {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, f.Content, 0o644))
	}
	return dir
}

func TestReadFolder_GivesTheGoldenDigest(t *testing.T) {
	name, files, problems, err := agentskill.ReadFolder(writeGolden(t))
	require.NoError(t, err)
	require.Empty(t, problems)
	require.Equal(t, "web-recon", name)
	a, err := agentskill.Package(name, files)
	require.NoError(t, err)
	require.Equal(t, goldenDigest, a.Digest.String())
}

func TestReadFolder_RefusesALink(t *testing.T) {
	dir := writeGolden(t)
	require.NoError(t, os.Symlink("/etc/hostname", filepath.Join(dir, "link")))
	_, _, problems, err := agentskill.ReadFolder(dir)
	require.NoError(t, err)
	require.Contains(t, strings.Join(problems, "\n"), `"link" is not a regular file`)
}

func TestReadFolder_DoesNotReadAFileOverTheLimit(t *testing.T) {
	dir := writeGolden(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.bin"), make([]byte, agentskill.MaxFileBytes+1), 0o644))
	_, files, problems, err := agentskill.ReadFolder(dir)
	require.NoError(t, err)
	require.Contains(t, strings.Join(problems, "\n"), "over a size limit")
	for _, f := range files {
		if f.Path == "big.bin" {
			require.Empty(t, f.Content)
		}
	}
}

func TestReadFolder_MissingFolder(t *testing.T) {
	_, _, _, err := agentskill.ReadFolder(filepath.Join(t.TempDir(), "none"))
	require.Error(t, err)
}

func TestParseRepository(t *testing.T) {
	for _, bad := range []string{"host/repo:tag", "host/repo@sha256:" + strings.Repeat("a", 64), "host:5000", "UPPER/Repo"} {
		_, err := agentskill.ParseRepository(bad)
		require.Error(t, err, bad)
	}
	repo, err := agentskill.ParseRepository("localhost:5000/team/skills")
	require.NoError(t, err)
	require.Equal(t, "localhost:5000/team/skills", repo.Name())
}

func TestPush_WritesTheImageByDigest(t *testing.T) {
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	a, err := agentskill.Package("web-recon", goldenFiles())
	require.NoError(t, err)
	repo, err := agentskill.ParseRepository(host + "/team/skills")
	require.NoError(t, err)

	ref, err := agentskill.Push(context.Background(), repo, a)
	require.NoError(t, err)
	require.Equal(t, host+"/team/skills@"+goldenDigest, ref)

	// Read it back by digest. The bytes are the same as the local image.
	digest, err := name.NewDigest(ref)
	require.NoError(t, err)
	got, err := remote.Image(digest)
	require.NoError(t, err)
	gotDigest, err := got.Digest()
	require.NoError(t, err)
	require.Equal(t, goldenDigest, gotDigest.String())
	mt, err := got.MediaType()
	require.NoError(t, err)
	require.Equal(t, "application/vnd.oci.image.manifest.v1+json", string(mt))
	m, err := got.Manifest()
	require.NoError(t, err)
	require.Equal(t, agentskill.ConfigMediaType, m.Config.MediaType)
	require.Len(t, m.Layers, 1)
	require.Equal(t, agentskill.LayerMediaType, m.Layers[0].MediaType)
}

func TestPush_FailsWhenTheRegistryIsDown(t *testing.T) {
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	host := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()
	a, err := agentskill.Package("web-recon", goldenFiles())
	require.NoError(t, err)
	repo, err := agentskill.ParseRepository(host + "/team/skills")
	require.NoError(t, err)
	_, err = agentskill.Push(context.Background(), repo, a)
	require.Error(t, err)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package scaffold_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/scaffold"
)

// sdkPin returns the sdk version that the adk itself requires, which is the
// version that `gibson component init` pins in a new project.
func sdkPin(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "go.mod"))
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^\s*github\.com/zeroroot-ai/sdk\s+(v\S+)`).FindSubmatch(b)
	require.NotNil(t, m, "go.mod requires the sdk")
	return string(m[1])
}

// TestRenderedAgentBuildsAndItsTestPasses renders the agent scaffold, resolves
// its modules, builds it and runs the test that the scaffold writes. Text
// goldens cannot catch a template that renders code which does not compile, so
// this test runs in the normal lane. It needs the Go module proxy and is
// skipped under -short.
func TestRenderedAgentBuildsAndItsTestPasses(t *testing.T) {
	if testing.Short() {
		t.Skip("needs the Go module proxy; skipped under -short")
	}
	files, err := scaffold.Render(scaffold.ScaffoldInput{
		Name: "demo-agent", Version: "0.1.0", Kind: scaffold.KindAgent, SDKVersion: sdkPin(t),
	})
	require.NoError(t, err)

	dir := t.TempDir()
	for rel, content := range files {
		dst := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
		require.NoError(t, os.WriteFile(dst, content, 0o644))
	}
	for _, args := range [][]string{{"mod", "tidy"}, {"build", "./..."}, {"test", "./..."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		require.NoError(t, cmd.Run(), fmt.Sprintf("go %s in the rendered agent:\n%s", strings.Join(args, " "), out.String()))
	}
}

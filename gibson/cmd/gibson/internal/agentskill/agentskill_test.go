// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package agentskill_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/agentskill"
)

type file = agentskill.File

// goldenSkill, goldenFiles and goldenDigest are the shared fixture. The gibson
// daemon has the same fixture and gets the same digest from its Package
// function. Do not change one without the other.
const goldenSkill = "---\nname: web-recon\ndescription: Map a web target before a scan.\n" +
	"license: Apache-2.0\ncompatibility: Needs httpx.\nmetadata:\n  author: acme\n" +
	"allowed-tools: native:httpx native:nmap\n---\n# Steps\nRun httpx first.\n"

const goldenDigest = "sha256:d585fe5073f5a1078419e2ab03f1ee00ca8e9256063459a47ee205d054a61e97"

func goldenFiles() []file {
	return []file{
		{Path: "scripts/run.sh", Content: []byte("#!/bin/sh\necho recon\n")},
		{Path: "SKILL.md", Content: []byte(goldenSkill)},
		{Path: "references/guide.md", Content: []byte("# Guide\nUse the flags in the order shown.\n")},
	}
}

func TestPackage_GoldenDigest(t *testing.T) {
	a, err := agentskill.Package("web-recon", goldenFiles())
	require.NoError(t, err)
	require.Equal(t, goldenDigest, a.Digest.String())
}

func TestPackage_DigestIgnoresFileOrder(t *testing.T) {
	files := goldenFiles()
	files[0], files[2] = files[2], files[0]
	a, err := agentskill.Package("web-recon", files)
	require.NoError(t, err)
	require.Equal(t, goldenDigest, a.Digest.String())
}

func TestPackage_DigestChangesWithContent(t *testing.T) {
	files := goldenFiles()
	files[0].Content = []byte("#!/bin/sh\necho other\n")
	a, err := agentskill.Package("web-recon", files)
	require.NoError(t, err)
	require.NotEqual(t, goldenDigest, a.Digest.String())
}

func TestPackage_ReadsTheManifest(t *testing.T) {
	a, err := agentskill.Package("web-recon", goldenFiles())
	require.NoError(t, err)
	m := a.Manifest
	require.Equal(t, "web-recon", m.Name)
	require.Equal(t, "Apache-2.0", m.License)
	require.Equal(t, map[string]string{"author": "acme"}, m.Metadata)
	require.Equal(t, []string{"native:httpx", "native:nmap"}, m.AllowedTools)
	require.Equal(t, "# Steps\nRun httpx first.\n", m.Body)
}

func skillWith(front string) []file {
	return []file{{Path: "SKILL.md", Content: []byte("---\n" + front + "---\nbody\n")}}
}

func TestCheck_Refusals(t *testing.T) {
	cases := []struct {
		name   string
		folder string
		files  []file
		want   string
	}{
		{"no SKILL.md", "web-recon", []file{{Path: "a.md", Content: []byte("x")}}, "no SKILL.md"},
		{"lowercase skill.md", "web-recon", []file{{Path: "skill.md", Content: []byte(goldenSkill)}}, "no SKILL.md"},
		{"no frontmatter", "web-recon", []file{{Path: "SKILL.md", Content: []byte("body")}}, "does not start with a frontmatter"},
		{"no closing line", "web-recon", []file{{Path: "SKILL.md", Content: []byte("---\nname: a\n")}}, "no closing line"},
		{"no name", "web-recon", skillWith("description: d\n"), "not a valid Agent Skill name"},
		{"no description", "web-recon", skillWith("name: web-recon\n"), "description is required"},
		{"blank description", "web-recon", skillWith("name: web-recon\ndescription: '  '\n"), "description is required"},
		{"name mismatch", "web-recon", skillWith("name: other\ndescription: d\n"), "does not match the folder name"},
		{"uppercase name", "Web-Recon", skillWith("name: Web-Recon\ndescription: d\n"), "not a valid Agent Skill name"},
		{"double hyphen", "a--b", skillWith("name: a--b\ndescription: d\n"), "not a valid Agent Skill name"},
		{"leading hyphen", "-ab", skillWith("name: -ab\ndescription: d\n"), "not a valid Agent Skill name"},
		{"trailing hyphen", "ab-", skillWith("name: ab-\ndescription: d\n"), "not a valid Agent Skill name"},
		{"name over 64", strings.Repeat("a", 65), skillWith("name: " + strings.Repeat("a", 65) + "\ndescription: d\n"), "not a valid Agent Skill name"},
		{"duplicate field", "web-recon", skillWith("name: web-recon\nname: web-recon\ndescription: d\n"), "not valid"},
		{"description over 1024", "web-recon", skillWith("name: web-recon\ndescription: " + strings.Repeat("d", 1025) + "\n"), "limit is 1024"},
		{
			"compatibility over 500", "web-recon",
			skillWith("name: web-recon\ndescription: d\ncompatibility: " + strings.Repeat("c", 501) + "\n"),
			"limit is 500",
		},
		{"metadata not a map", "web-recon", skillWith("name: web-recon\ndescription: d\nmetadata: x\n"), "not valid"},
		{
			"body over 64 KiB", "web-recon",
			[]file{{Path: "SKILL.md", Content: []byte("---\nname: web-recon\ndescription: d\n---\n" + strings.Repeat("b", 64<<10+1))}},
			"limit is 65536",
		},
		{"invalid UTF-8", "web-recon", []file{{Path: "SKILL.md", Content: []byte("---\nname: web-recon\n\xff---\n")}}, "not valid UTF-8"},
		{"absolute path", "web-recon", append(skillWith("name: web-recon\ndescription: d\n"), file{Path: "/etc/x"}), "is absolute"},
		{"parent path", "web-recon", append(skillWith("name: web-recon\ndescription: d\n"), file{Path: "../x"}), "leaves the folder"},
		{"backslash path", "web-recon", append(skillWith("name: web-recon\ndescription: d\n"), file{Path: `a\b`}), "backslash"},
		{"empty path", "web-recon", append(skillWith("name: web-recon\ndescription: d\n"), file{Path: ""}), "empty path"},
		{
			"duplicate path", "web-recon",
			append(skillWith("name: web-recon\ndescription: d\n"), file{Path: "a"}, file{Path: "a"}),
			"appears twice",
		},
		{
			"file over 1 MiB", "web-recon",
			append(skillWith("name: web-recon\ndescription: d\n"), file{Path: "big", Content: make([]byte, 1<<20+1)}),
			"limit is 1048576",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, problems := agentskill.Check(tc.folder, tc.files)
			require.NotEmpty(t, problems)
			require.Contains(t, strings.Join(problems, "\n"), tc.want)
			_, err := agentskill.Package(tc.folder, tc.files)
			require.ErrorIs(t, err, agentskill.ErrInvalidFolder)
		})
	}
}

func TestCheck_TooManyFiles(t *testing.T) {
	files := skillWith("name: web-recon\ndescription: d\n")
	for i := range agentskill.MaxFiles {
		files = append(files, file{Path: "f/" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + "/" + itoa(i)})
	}
	_, problems := agentskill.Check("web-recon", files)
	require.Contains(t, strings.Join(problems, "\n"), "the limit is 200")
}

func TestCheck_FolderOverLimit(t *testing.T) {
	files := skillWith("name: web-recon\ndescription: d\n")
	for i := range 9 {
		files = append(files, file{Path: "blob" + itoa(i), Content: make([]byte, 1<<20)})
	}
	_, problems := agentskill.Check("web-recon", files)
	require.Contains(t, strings.Join(problems, "\n"), "the folder has")
}

func TestCheck_ReportsEachProblem(t *testing.T) {
	_, problems := agentskill.Check("web-recon", skillWith("name: other\n"))
	joined := strings.Join(problems, "\n")
	require.Contains(t, joined, "description is required")
	require.Contains(t, joined, "does not match the folder name")
}

func TestCheck_AcceptsUnknownFieldsAndCRLF(t *testing.T) {
	skill := "---\r\nname: web-recon\r\ndescription: d\r\nfuture-field: 1\r\n---\r\nbody\r\n"
	_, problems := agentskill.Check("web-recon", []file{{Path: "SKILL.md", Content: []byte(skill)}})
	require.Empty(t, problems)
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{digits[i%10]}, b...)
	}
	return string(b)
}

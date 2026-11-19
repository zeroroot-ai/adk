// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package agentskill checks an Agent Skill folder against the Agent Skills
// format and packs it as an OCI image.
//
// The gibson daemon applies the same rules and builds the same image. The
// daemon code is closed source, so this package holds its own copy of the
// rules. The golden test in this package fixes the digest of one folder. The
// daemon has the same fixture and the same digest. If one side changes a rule,
// the digest of the fixture changes and that test fails.
//
// This package never changes a file and never cuts one. A folder that breaks a
// rule or a limit is refused.
package agentskill

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"gopkg.in/yaml.v3"
)

// SkillFile is the name of the file that each Agent Skill folder holds at its
// root.
const SkillFile = "SKILL.md"

// Media types of the OCI image of an Agent Skill. The daemon accepts only
// these.
const (
	// ConfigMediaType is the media type of the config blob.
	ConfigMediaType types.MediaType = "application/vnd.zeroroot.agent-skill.config.v1+json"
	// LayerMediaType is the media type of the one layer. The layer is a tar
	// archive of the folder.
	LayerMediaType types.MediaType = "application/vnd.zeroroot.agent-skill.layer.v1.tar"
)

// The size limits of the daemon. A folder over a limit is refused.
const (
	// MaxBodyBytes is the largest body of SKILL.md, after the frontmatter.
	MaxBodyBytes = 64 << 10
	// MaxFileBytes is the largest single file in the folder.
	MaxFileBytes = 1 << 20
	// MaxFolderBytes is the largest sum of the sizes of all files.
	MaxFolderBytes = 8 << 20
	// MaxFiles is the largest number of files in the folder.
	MaxFiles = 200
	// MaxNameLen is the longest name, in characters.
	MaxNameLen = 64
	// MaxDescriptionLen is the longest description, in characters.
	MaxDescriptionLen = 1024
	// MaxCompatibilityLen is the longest compatibility text, in characters.
	MaxCompatibilityLen = 500
)

// ErrInvalidFolder marks a folder that breaks a rule or a limit.
var ErrInvalidFolder = errors.New("invalid Agent Skill folder")

// nameRE is the name rule of the format. The name has lowercase letters,
// digits and single hyphens. It does not start or end with a hyphen.
var nameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidName reports whether name follows the name rule of the format.
func ValidName(name string) bool {
	return len(name) <= MaxNameLen && nameRE.MatchString(name)
}

// File is one file of the folder. Path is relative to the root of the folder
// and uses "/" as the separator.
type File struct {
	Path    string
	Content []byte
}

// Manifest is the frontmatter of SKILL.md and its body.
type Manifest struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	Metadata      map[string]string
	AllowedTools  []string
	Body          string
}

// frontmatter is the YAML schema of SKILL.md. The format does not say that a
// reader refuses an unknown field, so the decoder ignores it.
type frontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Compatibility string            `yaml:"compatibility"`
	Metadata      map[string]string `yaml:"metadata"`
	AllowedTools  string            `yaml:"allowed-tools"`
}

// Check applies the format rules and the size limits to the folder. It returns
// each problem that it finds, in a stable order. An empty list means the
// folder is valid. The manifest is nil when SKILL.md cannot be read.
func Check(folderName string, files []File) (manifest *Manifest, problems []string) {
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if !ValidName(folderName) {
		add("the folder name %q is not a valid Agent Skill name", folderName)
	}
	problems = append(problems, checkFiles(files)...)

	var skill []byte
	found := false
	for _, f := range files {
		if f.Path == SkillFile {
			skill, found = f.Content, true
		}
	}
	if !found {
		add("the folder has no %s at its root", SkillFile)
		return nil, problems
	}
	m, fieldProblems := parseSkillFile(skill)
	problems = append(problems, fieldProblems...)
	if m != nil && m.Name != "" && m.Name != folderName {
		add("the name %q in the frontmatter does not match the folder name %q", m.Name, folderName)
	}
	return m, problems
}

// checkFiles applies the path rules and the size limits to the file list.
func checkFiles(files []File) []string {
	var problems []string
	if len(files) > MaxFiles {
		problems = append(problems, fmt.Sprintf("the folder has %d files, the limit is %d", len(files), MaxFiles))
	}
	seen := make(map[string]struct{}, len(files))
	var total int
	for _, f := range files {
		if err := validPath(f.Path); err != "" {
			problems = append(problems, err)
			continue
		}
		if _, dup := seen[f.Path]; dup {
			problems = append(problems, fmt.Sprintf("the path %q appears twice", f.Path))
		}
		seen[f.Path] = struct{}{}
		if len(f.Content) > MaxFileBytes {
			problems = append(problems, fmt.Sprintf("the file %q has %d bytes, the limit is %d",
				f.Path, len(f.Content), MaxFileBytes))
		}
		total += len(f.Content)
	}
	if total > MaxFolderBytes {
		problems = append(problems, fmt.Sprintf("the folder has %d bytes, the limit is %d", total, MaxFolderBytes))
	}
	return problems
}

// validPath returns a problem text for a path that is not a clean relative
// path inside the folder. It returns an empty string for a good path.
func validPath(p string) string {
	switch {
	case p == "":
		return "a file has an empty path"
	case !utf8.ValidString(p):
		return fmt.Sprintf("the path %q is not valid UTF-8", p)
	case strings.ContainsAny(p, "\\\x00"):
		return fmt.Sprintf("the path %q has a backslash or a NUL byte", p)
	case strings.HasPrefix(p, "/"):
		return fmt.Sprintf("the path %q is absolute", p)
	case path.Clean(p) != p:
		return fmt.Sprintf("the path %q is not in clean form", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return fmt.Sprintf("the path %q leaves the folder or has an empty segment", p)
		}
	}
	return ""
}

// parseSkillFile splits SKILL.md into the frontmatter and the body and checks
// each field. It returns a nil manifest when the frontmatter cannot be read.
func parseSkillFile(raw []byte) (manifest *Manifest, problems []string) {
	if !utf8.Valid(raw) {
		return nil, []string{SkillFile + " is not valid UTF-8"}
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return nil, []string{SkillFile + " does not start with a frontmatter"}
	}
	var head, body string
	switch {
	case strings.HasPrefix(rest, "---\n"):
		body = strings.TrimPrefix(rest, "---\n")
	case rest == "---":
	default:
		end := strings.Index(rest, "\n---\n")
		switch {
		case end >= 0:
			head, body = rest[:end], rest[end+len("\n---\n"):]
		case strings.HasSuffix(rest, "\n---"):
			head = strings.TrimSuffix(rest, "\n---")
		default:
			return nil, []string{"the frontmatter of " + SkillFile + " has no closing line"}
		}
	}
	var fm frontmatter
	dec := yaml.NewDecoder(strings.NewReader(head))
	if err := dec.Decode(&fm); err != nil && !errors.Is(err, io.EOF) {
		return nil, []string{fmt.Sprintf("the frontmatter of %s is not valid: %v", SkillFile, err)}
	}

	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if len(body) > MaxBodyBytes {
		add("the body of %s has %d bytes, the limit is %d", SkillFile, len(body), MaxBodyBytes)
	}
	if !ValidName(fm.Name) {
		add("the name %q in the frontmatter is not a valid Agent Skill name", fm.Name)
	}
	if strings.TrimSpace(fm.Description) == "" {
		add("the description is required")
	}
	if n := utf8.RuneCountInString(fm.Description); n > MaxDescriptionLen {
		add("the description has %d characters, the limit is %d", n, MaxDescriptionLen)
	}
	if n := utf8.RuneCountInString(fm.Compatibility); n > MaxCompatibilityLen {
		add("compatibility has %d characters, the limit is %d", n, MaxCompatibilityLen)
	}
	return &Manifest{
		Name:          fm.Name,
		Description:   fm.Description,
		License:       fm.License,
		Compatibility: fm.Compatibility,
		Metadata:      fm.Metadata,
		AllowedTools:  strings.Fields(fm.AllowedTools),
		Body:          body,
	}, problems
}

// Artifact is a validated Agent Skill as an OCI image.
type Artifact struct {
	Manifest *Manifest
	Image    v1.Image
	// Digest is the digest of the image manifest.
	Digest v1.Hash
}

// Package checks the folder and writes it as an OCI image with one layer. The
// same content always gives the same digest, in any file order. A folder with
// a problem gives an error that wraps ErrInvalidFolder and names each problem.
func Package(folderName string, files []File) (*Artifact, error) {
	m, problems := Check(folderName, files)
	if len(problems) > 0 {
		return nil, &ProblemsError{Problems: problems}
	}
	layer, err := tarLayer(files)
	if err != nil {
		return nil, err
	}
	img, err := mutate.Append(empty.Image, mutate.Addendum{Layer: layer})
	if err != nil {
		return nil, fmt.Errorf("agentskill: append the layer: %w", err)
	}
	img = mutate.MediaType(img, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, ConfigMediaType)
	digest, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("agentskill: digest of the image: %w", err)
	}
	return &Artifact{Manifest: m, Image: img, Digest: digest}, nil
}

// ProblemsError holds each problem that Check found.
type ProblemsError struct {
	Problems []string
}

// Error joins the problems.
func (e *ProblemsError) Error() string {
	return fmt.Sprintf("%v: %s", ErrInvalidFolder, strings.Join(e.Problems, "; "))
}

// Unwrap lets errors.Is match ErrInvalidFolder.
func (e *ProblemsError) Unwrap() error { return ErrInvalidFolder }

// tarLayer writes the files, sorted by path, into one tar archive with no time
// and no owner, so the bytes depend on the content only.
func tarLayer(files []File) (v1.Layer, error) {
	sorted := make([]File, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, f := range sorted {
		hdr := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     f.Path,
			Mode:     0o644,
			Size:     int64(len(f.Content)),
			Format:   tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("agentskill: tar header for %q: %w", f.Path, err)
		}
		if _, err := tw.Write(f.Content); err != nil {
			return nil, fmt.Errorf("agentskill: tar content of %q: %w", f.Path, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("agentskill: close the tar archive: %w", err)
	}
	return static.NewLayer(buf.Bytes(), LayerMediaType), nil
}

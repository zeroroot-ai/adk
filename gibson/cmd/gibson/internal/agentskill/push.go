// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package agentskill

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ParseRepository checks the repository that the user named. It must have no
// tag and no digest. It returns the full name, with the registry host.
func ParseRepository(s string) (name.Repository, error) {
	if strings.ContainsAny(s, "@") || strings.Contains(s[strings.LastIndex(s, "/")+1:], ":") {
		return name.Repository{}, fmt.Errorf("repository %q must not have a tag or a digest", s)
	}
	repo, err := name.NewRepository(s, name.StrictValidation)
	if err != nil {
		return name.Repository{}, fmt.Errorf("repository %q: %w", s, err)
	}
	return repo, nil
}

// Push writes the image to the repository by its digest. It uses the docker
// credential of the current user for the registry. It does not set a tag.
// remoteOpts can add options, for example a transport for a test. It returns
// the reference with the digest, in the form <registry>/<repository>@sha256:<hex>.
func Push(ctx context.Context, repo name.Repository, a *Artifact, remoteOpts ...remote.Option) (string, error) {
	ref := repo.Digest(a.Digest.String())
	opts := append([]remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	}, remoteOpts...)
	if err := remote.Write(ref, a.Image, opts...); err != nil {
		return "", fmt.Errorf("push to %s: %w", repo.Name(), err)
	}
	return ref.String(), nil
}

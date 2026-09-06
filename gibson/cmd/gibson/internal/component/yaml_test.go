// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/component"
)

func TestComponent_ValidateRejects(t *testing.T) {
	cases := map[string]component.Component{
		"empty kind": {
			APIVersion: component.APIVersionV1,
			Metadata:   component.ComponentMetadata{Name: "x", Version: "1"},
		},
		"bad kind": {
			APIVersion: component.APIVersionV1,
			Kind:       component.Kind("nonsense"),
			Metadata:   component.ComponentMetadata{Name: "x", Version: "1"},
		},
		"bad apiVersion": {
			APIVersion: "wrong",
			Kind:       component.KindAgent,
			Metadata:   component.ComponentMetadata{Name: "x", Version: "1"},
		},
		"name fails regex": {
			APIVersion: component.APIVersionV1,
			Kind:       component.KindAgent,
			Metadata:   component.ComponentMetadata{Name: "BadName", Version: "1"},
		},
		"missing version": {
			APIVersion: component.APIVersionV1,
			Kind:       component.KindAgent,
			Metadata:   component.ComponentMetadata{Name: "x"},
		},
		"runtime on agent": {
			APIVersion: component.APIVersionV1,
			Kind:       component.KindAgent,
			Metadata:   component.ComponentMetadata{Name: "x", Version: "1"},
			Spec:       component.ComponentSpec{Runtime: "process"},
		},
		"manifest_path on tool": {
			APIVersion: component.APIVersionV1,
			Kind:       component.KindTool,
			Metadata:   component.ComponentMetadata{Name: "x", Version: "1"},
			Spec:       component.ComponentSpec{ManifestPath: "./plugin.yaml"},
		},
		"bad runtime on plugin": {
			APIVersion: component.APIVersionV1,
			Kind:       component.KindPlugin,
			Metadata:   component.ComponentMetadata{Name: "x", Version: "1"},
			Spec:       component.ComponentSpec{Runtime: "lambda"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			c := c
			require.Error(t, c.Validate())
		})
	}
}

func TestComponent_EffectiveDefaults(t *testing.T) {
	plugin := &component.Component{
		APIVersion: component.APIVersionV1,
		Kind:       component.KindPlugin,
		Metadata:   component.ComponentMetadata{Name: "p", Version: "0.1.0"},
	}
	assert.Equal(t, "./", plugin.EffectiveMainPath())
	assert.Equal(t, "./plugin.yaml", plugin.EffectiveManifestPath())
	assert.Equal(t, "process", plugin.EffectiveRuntime())

	agent := &component.Component{
		APIVersion: component.APIVersionV1,
		Kind:       component.KindAgent,
		Metadata:   component.ComponentMetadata{Name: "a", Version: "0.1.0"},
	}
	assert.Empty(t, agent.EffectiveRuntime())
}

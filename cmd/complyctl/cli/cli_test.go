// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/gemaraproj/go-gemara"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/complytime/complyctl/internal/complytime"
	"github.com/complytime/complyctl/internal/policy"
)

func chdirTemp(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func writeWorkspaceConfig(t *testing.T, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile("complytime.yaml", []byte(content), 0600))
}

const minimalConfig = `policies:
  - url: registry.example.com/policies/test-policy@v1.0
    id: test-policy
targets:
  - id: local
    policies:
      - test-policy
    variables:
      profile: test
`

// --- scanOptions tests ---

func TestScanOptions_Run_NoWorkspace(t *testing.T) {
	chdirTemp(t)
	o := &scanOptions{
		Common:  &Common{},
		timeout: 5 * time.Second,
	}
	err := o.run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load workspace config")
}

func TestScanOptions_Run_NoTargets(t *testing.T) {
	chdirTemp(t)
	writeWorkspaceConfig(t, `policies:
  - url: registry.example.com/policies/test@v1.0
targets: []
`)
	o := &scanOptions{
		Common:  &Common{},
		timeout: 5 * time.Second,
	}
	err := o.run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no targets")
}

func TestScanOptions_Run_PolicyNotFound(t *testing.T) {
	chdirTemp(t)
	writeWorkspaceConfig(t, minimalConfig)
	o := &scanOptions{
		Common:   &Common{},
		policyID: "nonexistent-policy",
		timeout:  5 * time.Second,
		cacheDir: t.TempDir(),
	}
	err := o.run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in config")
}

func TestScanOptions_Validate_InvalidFormat(t *testing.T) {
	o := &scanOptions{format: "xml"}
	err := o.validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

func TestScanOptions_Validate_ValidFormats(t *testing.T) {
	for _, f := range []string{"", "oscal", "pretty", "sarif"} {
		o := &scanOptions{format: f}
		assert.NoError(t, o.validate(), "format %q should be valid", f)
	}
}

// --- generateOptions tests ---

func TestGenerateOptions_Run_NoWorkspace(t *testing.T) {
	chdirTemp(t)
	o := &generateOptions{
		Common:  &Common{},
		timeout: 5 * time.Second,
	}
	err := o.run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load workspace config")
}

func TestGenerateOptions_Run_PolicyNotFound(t *testing.T) {
	chdirTemp(t)
	writeWorkspaceConfig(t, minimalConfig)
	o := &generateOptions{
		Common:   &Common{},
		policyID: "nonexistent",
		timeout:  5 * time.Second,
		cacheDir: t.TempDir(),
	}
	err := o.run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in config")
}

// --- getOptions tests ---

func TestGetOptions_Run_NoWorkspace(t *testing.T) {
	chdirTemp(t)
	o := &getOptions{
		Common:   &Common{},
		timeout:  5 * time.Second,
		cacheDir: t.TempDir(),
	}
	err := o.run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load workspace config")
}

// --- listOptions tests ---

func TestListOptions_Run_NoWorkspace(t *testing.T) {
	chdirTemp(t)
	o := &listOptions{
		Common:   &Common{Output: Output{Out: &bytes.Buffer{}}},
		cacheDir: t.TempDir(),
	}
	err := o.run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load workspace config")
}

func TestListOptions_Run_ValidWorkspace(t *testing.T) {
	chdirTemp(t)
	writeWorkspaceConfig(t, minimalConfig)
	var buf bytes.Buffer
	o := &listOptions{
		Common:   &Common{Output: Output{Out: &buf}},
		cacheDir: t.TempDir(),
	}
	err := o.run(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "POLICY ID")
}

// --- initOptions tests ---

func TestInitOptions_Run_AlreadyExists(t *testing.T) {
	chdirTemp(t)
	writeWorkspaceConfig(t, minimalConfig)
	o := &initOptions{Common: &Common{}}
	err := o.run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// --- providersOptions tests ---

func TestProvidersOptions_Run_EmptyPluginDir(t *testing.T) {
	o := &providersOptions{
		Common:    &Common{},
		pluginDir: t.TempDir(),
	}
	err := o.run(context.Background())
	require.NoError(t, err)
}

// --- helper tests ---

func TestFilterTargetsForPolicy(t *testing.T) {
	targets := []complytime.TargetConfig{
		{ID: "t1", Policies: []string{"p1", "p2"}},
		{ID: "t2", Policies: []string{"p2"}},
		{ID: "t3", Policies: []string{"p3"}},
	}
	result := filterTargetsForPolicy(targets, "p2")
	require.Len(t, result, 2)
	assert.Equal(t, "t1", result[0].ID)
	assert.Equal(t, "t2", result[1].ID)
}

func TestExtractReqToControlMap_NilGraph(t *testing.T) {
	m := extractReqToControlMap(nil)
	assert.Empty(t, m)
}

func TestExtractReqToControlMap_WithControls(t *testing.T) {
	graph := &policy.DependencyGraph{
		Controls: []policy.Control{
			{
				Parsed: &gemara.ControlCatalog{
					Controls: []gemara.Control{
						{
							Id: "ctrl-1",
							AssessmentRequirements: []gemara.AssessmentRequirement{
								{Id: "req-1"},
								{Id: "req-2"},
							},
						},
					},
				},
			},
		},
	}
	m := extractReqToControlMap(graph)
	assert.Equal(t, "ctrl-1", m["req-1"])
	assert.Equal(t, "ctrl-1", m["req-2"])
}

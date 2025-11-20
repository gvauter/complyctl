// SPDX-License-Identifier: Apache-2.0

package complytime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	oscalTypes "github.com/defenseunicorns/go-oscal/src/types/oscal-1-1-3"
	"github.com/oscal-compass/oscal-sdk-go/validation"
	"github.com/stretchr/testify/require"
)

func TestApplicationDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	appDir, err := newApplicationDirectory(tmpDir, false)
	require.NoError(t, err)

	expectedAppDir := filepath.Join(tmpDir, "complytime")
	expectedPluginDir := filepath.Join(tmpDir, "complytime", "plugins")
	expectedPluginManifestDir := filepath.Join(tmpDir, "complytime", "plugins")
	expectedBundleDir := filepath.Join(tmpDir, "complytime", "bundles")
	expectedControlDir := filepath.Join(tmpDir, "complytime", "controls")

	require.Equal(t, expectedAppDir, appDir.AppDir())
	require.Equal(t, expectedPluginDir, appDir.PluginDir())
	require.Equal(t, expectedPluginManifestDir, appDir.PluginManifestDir())
	require.Equal(t, expectedBundleDir, appDir.BundleDir())
	require.Equal(t, expectedControlDir, appDir.ControlDir())
	require.Equal(t, []string{expectedAppDir, expectedPluginDir, expectedPluginManifestDir, expectedBundleDir, expectedControlDir}, appDir.Dirs())

	appDir, err = newApplicationDirectory(tmpDir, true)
	require.NoError(t, err)
	_, err = os.Stat(appDir.AppDir())
	require.NoError(t, err)
	_, err = os.Stat(appDir.PluginDir())
	require.NoError(t, err)
	_, err = os.Stat(appDir.BundleDir())
	require.NoError(t, err)
	_, err = os.Stat(appDir.ControlDir())
	require.NoError(t, err)
}

func TestFindComponentDefinitions(t *testing.T) {
	compDefs, err := FindComponentDefinitions("testdata/complytime/bundles", validation.NoopValidator{})
	require.NoError(t, err)
	require.Len(t, compDefs, 1)

	_, err = FindComponentDefinitions("testdata/", validation.NoopValidator{})
	require.ErrorIs(t, err, ErrNoComponentDefinitionsFound)

}

func TestEnsureUserWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	testPlanPath := filepath.Join(tmpDir, "test_workspace")

	// Ensure user workspace directory is properly created
	err := EnsureUserWorkspace(testPlanPath)
	require.NoError(t, err)

	info, err := os.Stat(testPlanPath)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.True(t, info.IsDir(), "expected a directory, got something else")

	// Now lets remove the created directory and set the parent dir as read-only
	err = os.RemoveAll(testPlanPath)
	require.NoError(t, err, "failed to remove test_workspace directory")

	err = os.Chmod(tmpDir, 0500) // read + execute only, no write
	require.NoError(t, err, "failed to chmod parent dir")

	// Try to create the user workspace again but now it should fail
	err = EnsureUserWorkspace(testPlanPath)
	require.Error(t, err, "expected error when trying to create dir in read-only parent")
}

func TestLoadLayer2Catalogs(t *testing.T) {
	catalogs, ok, err := loadLayer2Catalogs("testdata/complytime/governance", hclog.Default())
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, catalogs, 1)
}

func TestLoadLayer1Guidance_Valid(t *testing.T) {
	logger := hclog.NewNullLogger()
	docs, err := loadLayer1Guidance("testdata/complytime/governance", logger)
	require.NoError(t, err)
	require.NotEmpty(t, docs)
	require.NotEmpty(t, docs[0].Metadata.Id)
}

func TestLoadLayer1Guidance_NoGuidanceDir(t *testing.T) {
	tmpDir := t.TempDir()
	logger := hclog.NewNullLogger()
	docs, err := loadLayer1Guidance(tmpDir, logger)
	require.NoError(t, err)
	require.Empty(t, docs)
}

func TestLoadLayer1Guidance_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	logger := hclog.NewNullLogger()
	guidanceDir := filepath.Join(tmpDir, "guidance")
	require.NoError(t, os.MkdirAll(guidanceDir, 0o700))
	// Create an invalid file
	require.NoError(t, os.WriteFile(filepath.Join(guidanceDir, "bad.txt"), []byte("not guidance"), 0o600))
	_, err := loadLayer1Guidance(tmpDir, logger)
	require.Error(t, err)
}

func TestGuidanceToProfile_AndWriteProfile(t *testing.T) {
	// Load L1 guidance from testdata
	logger := hclog.NewNullLogger()
	gdocs, err := loadLayer1Guidance("testdata/complytime/governance", logger)
	require.NoError(t, err)
	require.NotEmpty(t, gdocs)

	gd := gdocs[0]
	profile, err := guidanceToProfile(gd, gd.Metadata.Id)
	require.NoError(t, err)

	// Write profile to a temp appDir controls/
	tmpRoot := t.TempDir()
	appDir, err := newApplicationDirectory(tmpRoot, true)
	require.NoError(t, err)

	href, err := writeProfile(profile, gd.Metadata.Id, appDir, logger)
	require.NoError(t, err)
	require.Contains(t, href, "file://controls/")

	// Check file exists
	filename := filepath.Base(href[len("file://"):])
	dstPath := filepath.Join(appDir.ControlDir(), filename)
	_, statErr := os.Stat(dstPath)
	require.NoError(t, statErr)
}

func TestWriteProfile_WritesToControls(t *testing.T) {
	tmpRoot := t.TempDir()
	logger := hclog.NewNullLogger()
	appDir, err := newApplicationDirectory(tmpRoot, true)
	require.NoError(t, err)
	// Minimal empty profile is acceptable for writeProfile
	var profile oscalTypes.Profile
	href, err := writeProfile(profile, "test-guidance", appDir, logger)
	require.NoError(t, err)
	require.Equal(t, "file://controls/test-guidance.profile.json", href)
	// Ensure file exists
	dstPath := filepath.Join(appDir.ControlDir(), "test-guidance.profile.json")
	_, statErr := os.Stat(dstPath)
	require.NoError(t, statErr)
}

func TestPrepareGuidanceProfiles_NoGuidanceDir(t *testing.T) {
	tmpRoot := t.TempDir()
	logger := hclog.NewNullLogger()
	appDir, err := newApplicationDirectory(tmpRoot, true)
	require.NoError(t, err)
	m, err := prepareGuidanceProfiles(tmpRoot, appDir, logger)
	require.NoError(t, err)
	require.Empty(t, m)
}
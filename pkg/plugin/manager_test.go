// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/complytime/complyctl/pkg/plugin"
)

func TestManager_GetPlugin(t *testing.T) {
	mgr, err := plugin.NewManager("/nonexistent", nil)
	require.NoError(t, err)

	mgr.RegisterPluginForTest("mock", &plugin.LoadedPlugin{
		Info: plugin.PluginInfo{
			PluginID:       "mock",
			EvaluatorID:    "mock",
			ExecutablePath: "(test)",
		},
	})

	p, err := mgr.GetPlugin("mock")
	require.NoError(t, err)
	assert.Equal(t, "mock", p.Info.PluginID)
}

func TestManager_GetPlugin_UnknownID(t *testing.T) {
	mgr, err := plugin.NewManager("/nonexistent", nil)
	require.NoError(t, err)

	mgr.RegisterPluginForTest("mock", &plugin.LoadedPlugin{
		Info: plugin.PluginInfo{
			PluginID:       "mock",
			EvaluatorID:    "mock",
			ExecutablePath: "(test)",
		},
	})

	_, err = mgr.GetPlugin("unknown-evaluator")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin not found for evaluator ID")
}

func TestManager_ListPlugins(t *testing.T) {
	mgr, err := plugin.NewManager("/nonexistent", nil)
	require.NoError(t, err)

	mgr.RegisterPluginForTest("mock", &plugin.LoadedPlugin{
		Info: plugin.PluginInfo{
			PluginID:       "mock",
			EvaluatorID:    "mock",
			ExecutablePath: "(test)",
		},
	})

	plugins := mgr.ListPlugins()
	assert.Len(t, plugins, 1)
	assert.Equal(t, "mock", plugins[0].Info.PluginID)
}

func TestManager_EmptyPluginDir(t *testing.T) {
	tmpDir := t.TempDir()

	mgr, err := plugin.NewManager(tmpDir, nil)
	require.NoError(t, err)
	require.NoError(t, mgr.LoadPlugins())

	plugins := mgr.ListPlugins()
	assert.Empty(t, plugins)
}

type failingMockClient struct {
	mockClient
	failGenerate bool
	failScan     bool
}

func (f *failingMockClient) Generate(_ context.Context, _ *plugin.GenerateRequest) (*plugin.GenerateResponse, error) {
	if f.failGenerate {
		return nil, fmt.Errorf("generate RPC failed")
	}
	return &plugin.GenerateResponse{Success: true}, nil
}

func (f *failingMockClient) Scan(_ context.Context, _ *plugin.ScanRequest) (*plugin.ScanResponse, error) {
	if f.failScan {
		return nil, fmt.Errorf("scan RPC failed")
	}
	return &plugin.ScanResponse{Assessments: []plugin.AssessmentLog{{RequirementID: "test-req"}}}, nil
}

func TestManager_RouteGenerate_SpecificPlugin(t *testing.T) {
	mgr, err := plugin.NewManager(t.TempDir(), nil)
	require.NoError(t, err)

	mock := newMockClient()
	lp := plugin.NewMockLoadedPlugin("test-plugin", "test-eval", mock)
	mgr.RegisterPluginForTest("test-eval", lp)

	err = mgr.RouteGenerate(context.Background(), "test-eval", nil, nil, []plugin.AssessmentConfiguration{
		{RequirementID: "req-1"},
	})
	require.NoError(t, err)
}

func TestManager_RouteGenerate_UnknownEvaluator(t *testing.T) {
	mgr, err := plugin.NewManager(t.TempDir(), nil)
	require.NoError(t, err)

	err = mgr.RouteGenerate(context.Background(), "nonexistent", nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no plugin registered")
}

func TestManager_RouteGenerate_Broadcast(t *testing.T) {
	mgr, err := plugin.NewManager(t.TempDir(), nil)
	require.NoError(t, err)

	mock := newMockClient()
	lp := plugin.NewMockLoadedPlugin("test-plugin", "test-eval", mock)
	mgr.RegisterPluginForTest("test-eval", lp)

	err = mgr.RouteGenerate(context.Background(), "", nil, nil, nil)
	require.NoError(t, err)
}

func TestManager_RouteGenerate_PluginReturnsFailure(t *testing.T) {
	mgr, err := plugin.NewManager(t.TempDir(), nil)
	require.NoError(t, err)

	fm := &failingMockClient{failGenerate: true}
	lp := plugin.NewMockLoadedPlugin("fail-plugin", "fail-eval", fm)
	mgr.RegisterPluginForTest("fail-eval", lp)

	err = mgr.RouteGenerate(context.Background(), "fail-eval", nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generate RPC failed")
}

func TestManager_RouteScan_SpecificPlugin(t *testing.T) {
	mgr, err := plugin.NewManager(t.TempDir(), nil)
	require.NoError(t, err)

	mock := &failingMockClient{}
	lp := plugin.NewMockLoadedPlugin("test-plugin", "test-eval", mock)
	mgr.RegisterPluginForTest("test-eval", lp)

	results, err := mgr.RouteScan(context.Background(), "test-eval", []plugin.Target{{TargetID: "t1"}})
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

func TestManager_RouteScan_UnknownEvaluator(t *testing.T) {
	mgr, err := plugin.NewManager(t.TempDir(), nil)
	require.NoError(t, err)

	_, err = mgr.RouteScan(context.Background(), "nonexistent", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no plugin registered")
}

func TestManager_RouteScan_Broadcast(t *testing.T) {
	mgr, err := plugin.NewManager(t.TempDir(), nil)
	require.NoError(t, err)

	mock := &failingMockClient{}
	lp := plugin.NewMockLoadedPlugin("test-plugin", "test-eval", mock)
	mgr.RegisterPluginForTest("test-eval", lp)

	results, err := mgr.RouteScan(context.Background(), "", []plugin.Target{{TargetID: "t1"}})
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

func TestManager_RouteScan_PluginError_ReturnsErrorAssessment(t *testing.T) {
	mgr, err := plugin.NewManager(t.TempDir(), nil)
	require.NoError(t, err)

	fm := &failingMockClient{failScan: true}
	lp := plugin.NewMockLoadedPlugin("fail-plugin", "fail-eval", fm)
	mgr.RegisterPluginForTest("fail-eval", lp)

	results, err := mgr.RouteScan(context.Background(), "fail-eval", nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Message, "scan RPC failed")
}

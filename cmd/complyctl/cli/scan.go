// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/gemaraproj/go-gemara"
	"github.com/spf13/cobra"

	"github.com/complytime/complyctl/internal/cache"
	"github.com/complytime/complyctl/internal/complytime"
	"github.com/complytime/complyctl/internal/output"
	"github.com/complytime/complyctl/internal/policy"
	"github.com/complytime/complyctl/internal/terminal"
	"github.com/complytime/complyctl/pkg/plugin"
)

type scanOptions struct {
	*Common
	policyID  string
	format    string
	timeout   time.Duration
	cacheDir  string
	pluginDir string
}

func scanCmd(common *Common) *cobra.Command {
	o := &scanOptions{
		Common: common,
	}
	cmd := &cobra.Command{
		Use:   "scan [flags]",
		Short: "Scan targets and produce compliance reports",
		Example: `complyctl scan --policy-id nist-800-53-r5
  complyctl scan --policy-id nist-800-53-r5 --format pretty
  complyctl scan --policy-id nist-800-53-r5 --format oscal
  complyctl scan --policy-id nist-800-53-r5 --format sarif`,
		SilenceUsage:      true,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := o.validate(); err != nil {
				return err
			}
			if err := o.complete(); err != nil {
				return err
			}
			return o.run(cmd.Context())
		},
	}
	cmd.Flags().StringVarP(&o.policyID, "policy-id", "p", "", "Policy ID to scan (see complyctl list)")
	cmd.Flags().StringVarP(&o.format, "format", "f", "", "Output format: oscal, pretty, sarif")
	cmd.Flags().DurationVarP(&o.timeout, "timeout", "t", complytime.DefaultCommandTimeout, "Maximum time for the scan operation (e.g. 5m, 10m, 1h)")
	if err := cmd.MarkFlagRequired("policy-id"); err != nil {
		logger.Error("Failed to mark policy-id as required", "error", err)
	}
	if err := cmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{complytime.OutputFormatOSCAL, complytime.OutputFormatPretty, complytime.OutputFormatSARIF}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		logger.Error("Failed to register format completion", "error", err)
	}
	if err := cmd.RegisterFlagCompletionFunc("policy-id", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		logger.Error("Failed to register policy-id completion", "error", err)
	}
	return cmd
}

func (o *scanOptions) validate() error {
	if o.format != "" {
		switch o.format {
		case complytime.OutputFormatOSCAL, complytime.OutputFormatPretty, complytime.OutputFormatSARIF:
		default:
			return fmt.Errorf("invalid format %q: must be one of %s, %s, %s",
				o.format, complytime.OutputFormatOSCAL, complytime.OutputFormatPretty, complytime.OutputFormatSARIF)
		}
	}
	return nil
}

func (o *scanOptions) complete() error {
	var err error
	o.cacheDir, err = complytime.ResolveCacheDir()
	if err != nil {
		return fmt.Errorf("failed to resolve cache directory: %w", err)
	}
	o.pluginDir, err = complytime.ResolvePluginDir()
	if err != nil {
		return fmt.Errorf("failed to resolve plugin directory: %w", err)
	}
	return nil
}

func (o *scanOptions) run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	cfg, err := loadWorkspaceConfig()
	if err != nil {
		return err
	}
	if len(cfg.Targets) == 0 {
		return fmt.Errorf("no targets in complytime.yaml (add targets with policies)")
	}

	entry, found := complytime.FindPolicy(cfg.Policies, o.policyID)
	if !found {
		return fmt.Errorf("policy %q not found in config — run complyctl list to see available policy IDs", o.policyID)
	}

	return o.scanPolicy(ctx, cfg, *entry)
}

func loadWorkspaceConfig() (*complytime.WorkspaceConfig, error) {
	ws := complytime.NewWorkspace()
	if err := ws.LoadAndValidate(); err != nil {
		return nil, fmt.Errorf("failed to load workspace config: %w", err)
	}
	return ws.Config(), nil
}

func (o *scanOptions) scanPolicy(ctx context.Context, cfg *complytime.WorkspaceConfig, entry complytime.PolicyEntry) error {
	ref := complytime.ParsePolicyRef(entry.URL)
	eid := entry.EffectiveID()

	version, graph, err := resolveVersionAndGraph(o.cacheDir, ref)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Resolved %s version: %s\n", ref.Repository, version)

	assessmentConfigs := policy.ExtractAssessmentConfigs(ref.Repository, graph)
	groups := policy.GroupByEvaluator(assessmentConfigs, graph)

	mgr, err := loadPlugins(o.pluginDir)
	if err != nil {
		return err
	}
	defer mgr.Cleanup()

	policyTargets := filterTargetsForPolicy(cfg.Targets, eid)
	evaluatorIDs := evaluatorIDList(groups)

	if err := ensureGenerated(ctx, o.cacheDir, mgr, groups, policyTargets, cfg.Variables, ref.Repository, eid, evaluatorIDs); err != nil {
		return err
	}

	targetIDs := targetIDList(policyTargets)
	fmt.Println(output.FormatPreScanSummary(len(assessmentConfigs), evaluatorIDs, targetIDs))

	return runScanAndReport(ctx, o.format, mgr, groups, policyTargets, ref.Repository, eid, graph, targetIDs)
}

func resolveVersionAndGraph(cacheDir string, ref complytime.PolicyRef) (string, *policy.DependencyGraph, error) {
	cacheMgr := cache.NewCache(cacheDir)
	loader := policy.NewLoader(cacheMgr)
	resolver := policy.NewResolver(loader)

	version, err := loader.ResolveVersion(ref.Repository, ref.Version)
	if err != nil {
		return "", nil, err
	}

	graph, err := resolver.ResolvePolicyGraph(ref.Repository, version)
	if err != nil {
		return "", nil, fmt.Errorf("failed to resolve policy graph: %w", err)
	}
	return version, graph, nil
}

func loadPlugins(pluginDir string) (*plugin.Manager, error) {
	mgr, err := plugin.NewManager(pluginDir, logger)
	if err != nil {
		return nil, fmt.Errorf("plugin manager init failed: %w", err)
	}
	if err := mgr.LoadPlugins(); err != nil {
		mgr.Cleanup()
		return nil, fmt.Errorf("plugin discovery failed: %w", err)
	}
	if len(mgr.ListPlugins()) == 0 {
		mgr.Cleanup()
		return nil, fmt.Errorf("no plugins found in %s (Describe may have failed)", pluginDir)
	}
	return mgr, nil
}

func evaluatorIDList(groups map[string]policy.EvaluatorGroup) []string {
	ids := make([]string, 0, len(groups))
	for evalID := range groups {
		ids = append(ids, evalID)
	}
	return ids
}

func targetIDList(targets []complytime.TargetConfig) []string {
	ids := make([]string, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, t.ID)
	}
	return ids
}

func ensureGenerated(ctx context.Context, cacheDir string, mgr *plugin.Manager, groups map[string]policy.EvaluatorGroup, policyTargets []complytime.TargetConfig, globalVars map[string]string, repository, eid string, evaluatorIDs []string) error {
	needsGenerate, policyDigest, err := checkGenerationFreshness(cacheDir, repository, eid)
	if err != nil {
		return err
	}
	if !needsGenerate {
		return nil
	}
	return runGeneration(ctx, mgr, groups, policyTargets, globalVars, repository, policyDigest, evaluatorIDs)
}

func runScanAndReport(ctx context.Context, format string, mgr *plugin.Manager, groups map[string]policy.EvaluatorGroup, policyTargets []complytime.TargetConfig, repository, eid string, graph *policy.DependencyGraph, targetIDs []string) error {
	reqToControl := extractReqToControlMap(graph)
	allAssessments, assessmentTargets, err := executeScan(ctx, mgr, groups, policyTargets)
	if err != nil {
		return err
	}

	eval := buildEvaluator(repository, reqToControl, policyTargets, allAssessments, assessmentTargets)

	outDir := filepath.Join(".", complytime.WorkspaceDir, complytime.ScanOutputDir)
	return writeScanReports(format, eval, outDir, ".", repository, allAssessments, assessmentTargets, reqToControl, eid, targetIDs)
}

func buildEvaluator(repository string, reqToControl map[string]string, policyTargets []complytime.TargetConfig, allAssessments []plugin.AssessmentLog, assessmentTargets []string) *output.Evaluator {
	eval := output.NewEvaluator(repository, reqToControl)
	for _, target := range policyTargets {
		var targetAssessments []plugin.AssessmentLog
		for j, a := range allAssessments {
			if assessmentTargets[j] == target.ID {
				targetAssessments = append(targetAssessments, a)
			}
		}
		eval.AddTarget(targetAssessments)
	}
	return eval
}

func filterTargetsForPolicy(targets []complytime.TargetConfig, policyID string) []complytime.TargetConfig {
	var result []complytime.TargetConfig
	for _, t := range targets {
		if slices.Contains(t.Policies, policyID) {
			result = append(result, t)
		}
	}
	return result
}

func checkGenerationFreshness(cacheDir, repository, eid string) (needsGenerate bool, digest string, err error) {
	cacheState, err := cache.LoadState(cacheDir)
	if err != nil {
		return false, "", fmt.Errorf("failed to load cache state: %w", err)
	}
	policyState, _ := cacheState.GetPolicyState(repository)

	genState, err := policy.LoadGenerationState(".", repository)
	if err != nil {
		return false, "", fmt.Errorf("failed to load generation state: %w", err)
	}

	return needsRegeneration(genState, policyState.Digest, eid), policyState.Digest, nil
}

func needsRegeneration(genState *policy.GenerationState, digest, eid string) bool {
	if genState == nil {
		fmt.Fprintf(os.Stderr, "No prior generation found — generating artifacts for %s\n", eid)
		return true
	}
	if !genState.IsFresh(digest) {
		fmt.Fprintf(os.Stderr, "Policy %s updated since last generate — regenerating\n", eid)
		return true
	}
	fmt.Fprintf(os.Stderr, "Reusing generated artifacts for %s (policy unchanged)\n", eid)
	return false
}

func runGeneration(ctx context.Context, mgr *plugin.Manager, groups map[string]policy.EvaluatorGroup, policyTargets []complytime.TargetConfig, globalVars map[string]string, repository, policyDigest string, evaluatorIDs []string) error {
	genSpin := terminal.NewSpinner("Generating policy artifacts...")
	genSpin.Start()
	defer genSpin.Stop()

	if err := generateForAllTargets(ctx, mgr, groups, policyTargets, globalVars); err != nil {
		return err
	}

	newGenState := policy.NewGenerationState(repository, policyDigest, evaluatorIDs)
	if err := policy.SaveGenerationState(".", repository, newGenState); err != nil {
		return fmt.Errorf("failed to save generation state: %w", err)
	}
	return nil
}

func generateForAllTargets(ctx context.Context, mgr *plugin.Manager, groups map[string]policy.EvaluatorGroup, policyTargets []complytime.TargetConfig, globalVars map[string]string) error {
	for evalID, group := range groups {
		for _, target := range policyTargets {
			if err := mgr.RouteGenerate(ctx, evalID, globalVars, target.Variables, group.Configs); err != nil {
				return err
			}
		}
	}
	return nil
}

func executeScan(ctx context.Context, mgr *plugin.Manager, groups map[string]policy.EvaluatorGroup, policyTargets []complytime.TargetConfig) ([]plugin.AssessmentLog, []string, error) {
	scanSpin := terminal.NewSpinner("Scanning targets...")
	scanSpin.Start()
	defer scanSpin.Stop()

	return scanAllTargets(ctx, mgr, groups, policyTargets)
}

func scanAllTargets(ctx context.Context, mgr *plugin.Manager, groups map[string]policy.EvaluatorGroup, policyTargets []complytime.TargetConfig) ([]plugin.AssessmentLog, []string, error) {
	var allAssessments []plugin.AssessmentLog
	var assessmentTargets []string

	for _, target := range policyTargets {
		results, err := scanSingleTarget(ctx, mgr, groups, target)
		if err != nil {
			return nil, nil, err
		}
		allAssessments = append(allAssessments, results...)
		for range results {
			assessmentTargets = append(assessmentTargets, target.ID)
		}
	}

	return allAssessments, assessmentTargets, nil
}

func scanSingleTarget(ctx context.Context, mgr *plugin.Manager, groups map[string]policy.EvaluatorGroup, target complytime.TargetConfig) ([]plugin.AssessmentLog, error) {
	pluginTargets := []plugin.Target{{
		TargetID:  target.ID,
		Variables: target.Variables,
	}}

	var results []plugin.AssessmentLog
	for evalID := range groups {
		evalResults, routeErr := mgr.RouteScan(ctx, evalID, pluginTargets)
		if routeErr != nil {
			return nil, routeErr
		}
		results = append(results, evalResults...)
	}
	return results, nil
}

func writeScanReports(format string, eval *output.Evaluator, outDir, reportDir, repository string, allAssessments []plugin.AssessmentLog, assessmentTargets []string, reqToControl map[string]string, eid string, targetIDs []string) error {
	logPath, err := eval.Write(outDir)
	if err != nil {
		return fmt.Errorf("failed to write evaluation log: %w", err)
	}
	fmt.Printf("Evaluation log written: %s\n", logPath)

	if err := writeFormatReport(format, eval, logPath, reportDir, repository); err != nil {
		return err
	}

	fmt.Println(output.FormatScanSummary(allAssessments, assessmentTargets, reqToControl, eid, targetIDs))
	return nil
}

func writeFormatReport(format string, eval *output.Evaluator, logPath, reportDir, repository string) error {
	gemaraLog := eval.GemaraLog()
	switch format {
	case complytime.OutputFormatPretty:
		return writePrettyReport(gemaraLog, logPath, reportDir, repository)
	case complytime.OutputFormatSARIF:
		return writeSARIFReport(gemaraLog, reportDir)
	case complytime.OutputFormatOSCAL:
		return writeOSCALReport(gemaraLog, reportDir)
	}
	return nil
}

func writePrettyReport(gemaraLog *gemara.EvaluationLog, logPath, reportDir, repository string) error {
	md := output.NewMarkdown(repository, gemaraLog)
	md.SetEmbedEvaluationLog(logPath)
	mdPath, err := md.Write(reportDir)
	if err != nil {
		return fmt.Errorf("failed to write markdown report: %w", err)
	}
	fmt.Printf("Markdown report written: %s\n\n", mdPath)
	return nil
}

func writeSARIFReport(gemaraLog *gemara.EvaluationLog, reportDir string) error {
	sarifPath, err := output.ToSARIF(gemaraLog, "file:///scan", reportDir)
	if err != nil {
		return fmt.Errorf("failed to export SARIF: %w", err)
	}
	fmt.Printf("SARIF report written: %s\n\n", sarifPath)
	return nil
}

func writeOSCALReport(gemaraLog *gemara.EvaluationLog, reportDir string) error {
	oscalPath, err := output.ToOSCAL(gemaraLog, reportDir)
	if err != nil {
		return fmt.Errorf("failed to export OSCAL: %w", err)
	}
	fmt.Printf("OSCAL report written: %s\n\n", oscalPath)
	return nil
}

// extractReqToControlMap builds a requirement-ID → control-ID mapping
// from the parsed control catalogs in the dependency graph.
func extractReqToControlMap(graph *policy.DependencyGraph) map[string]string {
	m := make(map[string]string)
	if graph == nil {
		return m
	}
	for _, ctrl := range graph.Controls {
		if ctrl.Parsed == nil {
			continue
		}
		for _, c := range ctrl.Parsed.Controls {
			for _, ar := range c.AssessmentRequirements {
				m[ar.Id] = c.Id
			}
		}
	}
	return m
}

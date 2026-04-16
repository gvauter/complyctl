// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/hashicorp/go-hclog"

	"github.com/complytime/complyctl/cmd/openscap-plugin/config"
	"github.com/complytime/complyctl/cmd/openscap-plugin/oscap"
	"github.com/complytime/complyctl/cmd/openscap-plugin/scan"
	"github.com/complytime/complyctl/cmd/openscap-plugin/xccdf"
	"github.com/complytime/complyctl/internal/complytime"
	"github.com/complytime/complyctl/pkg/plugin"
)

var (
	_         plugin.Plugin = (*PluginServer)(nil)
	ovalRegex               = regexp.MustCompile(`^[^:]*?:[^-]*?-(.*?):.*?$`)
)

const ovalCheckType = "http://oval.mitre.org/XMLSchema/oval-definitions-5"

type PluginServer struct{}

func New() *PluginServer {
	return &PluginServer{}
}

func (s *PluginServer) Describe(_ context.Context, _ *plugin.DescribeRequest) (*plugin.DescribeResponse, error) {
	return &plugin.DescribeResponse{
		Healthy:                 true,
		Version:                 "0.1.0",
		RequiredTargetVariables: []string{"profile"},
	}, nil
}

func (s *PluginServer) Generate(ctx context.Context, req *plugin.GenerateRequest) (*plugin.GenerateResponse, error) {
	if err := generateArtifacts(ctx, req); err != nil {
		return &plugin.GenerateResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	return &plugin.GenerateResponse{Success: true}, nil
}

func generateArtifacts(ctx context.Context, req *plugin.GenerateRequest) error {
	if len(req.Configuration) == 0 {
		return fmt.Errorf("no assessment configurations provided")
	}

	profile, datastream, err := resolveProfileAndDatastream(req)
	if err != nil {
		return err
	}

	return executeGeneration(ctx, req.Configuration, datastream, profile)
}

func resolveProfileAndDatastream(req *plugin.GenerateRequest) (string, string, error) {
	vars := mergeVariables(req.GlobalVariables, req.TargetVariables)

	profile, err := config.SanitizeInput(vars["profile"])
	if err != nil {
		return "", "", fmt.Errorf("invalid profile: %w", err)
	}

	datastream, err := config.ResolveDatastream(vars["datastream"])
	if err != nil {
		return "", "", fmt.Errorf("datastream error: %w", err)
	}
	return profile, datastream, nil
}

func executeGeneration(ctx context.Context, configurations []plugin.AssessmentConfiguration, datastream, profile string) error {
	if err := config.EnsureDirectories(); err != nil {
		return fmt.Errorf("directory setup failed: %w", err)
	}

	if err := writeTailoringFile(configurations, datastream, profile); err != nil {
		return err
	}

	hclog.Default().Info("Generating remediation files")
	pluginDir := filepath.Join(complytime.WorkspaceDir, config.PluginDir)
	if err := oscap.OscapGenerateFix(ctx, pluginDir, profile, config.PolicyPath, datastream); err != nil {
		return fmt.Errorf("remediation generation failed: %w", err)
	}
	return nil
}

func writeTailoringFile(configurations []plugin.AssessmentConfiguration, datastream, profile string) error {
	hclog.Default().Info("Generating a tailoring file")
	tailoringXML, err := xccdf.PolicyToXML(configurations, datastream, profile)
	if err != nil {
		return fmt.Errorf("tailoring generation failed: %w", err)
	}

	dst, err := os.Create(config.PolicyPath)
	if err != nil {
		return fmt.Errorf("failed to create policy file: %w", err)
	}
	defer dst.Close()
	if _, err := dst.WriteString(tailoringXML); err != nil {
		return fmt.Errorf("failed to write policy file: %w", err)
	}
	return nil
}

func (s *PluginServer) Scan(ctx context.Context, req *plugin.ScanRequest) (*plugin.ScanResponse, error) {
	if len(req.Targets) == 0 {
		return nil, fmt.Errorf("no targets provided")
	}

	xmlnode, err := runScanAndParseARF(ctx, req.Targets[0].Variables)
	if err != nil {
		return nil, err
	}

	assessments, err := buildAssessmentsFromARF(xmlnode)
	if err != nil {
		return nil, err
	}
	return &plugin.ScanResponse{Assessments: assessments}, nil
}

func runScanAndParseARF(ctx context.Context, vars map[string]string) (*xmlquery.Node, error) {
	profile, err := config.SanitizeInput(vars["profile"])
	if err != nil {
		return nil, fmt.Errorf("invalid profile: %w", err)
	}

	datastream, err := config.ResolveDatastream(vars["datastream"])
	if err != nil {
		return nil, fmt.Errorf("datastream error: %w", err)
	}

	hclog.Default().Info("Running scan", "profile", profile, "datastream", datastream)
	_, err = scan.ScanSystem(ctx, datastream, profile)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	return parseARFFile(config.ARFPath)
}

func parseARFFile(arfPath string) (*xmlquery.Node, error) {
	file, err := os.Open(filepath.Clean(arfPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open ARF: %w", err)
	}
	defer file.Close()

	xmlnode, err := xmlquery.Parse(bufio.NewReader(file))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ARF: %w", err)
	}
	return xmlnode, nil
}

func buildAssessmentsFromARF(xmlnode *xmlquery.Node) ([]plugin.AssessmentLog, error) {
	targetEl := xmlnode.SelectElement("//target")
	if targetEl == nil {
		return nil, errors.New("result has no 'target' attribute")
	}
	target := targetEl.InnerText()

	ruleTable := xccdf.NewRuleHashTable(xmlnode)
	results := xmlnode.SelectElements("//rule-result")

	var assessments []plugin.AssessmentLog
	for i := range results {
		assessment, skip, err := assessmentFromRuleResult(results[i], ruleTable, target)
		if err != nil {
			return nil, err
		}
		if !skip {
			assessments = append(assessments, assessment)
		}
	}
	return assessments, nil
}

func assessmentFromRuleResult(result *xmlquery.Node, ruleTable map[string]*xmlquery.Node, target string) (plugin.AssessmentLog, bool, error) {
	ruleIDRef, rule, resultText, skip := resolveRuleResult(result, ruleTable)
	if skip {
		return plugin.AssessmentLog{}, true, nil
	}

	return buildAssessmentLog(rule, result, ruleIDRef, resultText, target)
}

func resolveRuleResult(result *xmlquery.Node, ruleTable map[string]*xmlquery.Node) (string, *xmlquery.Node, string, bool) {
	resultEl := result.SelectElement("result")
	if resultEl == nil {
		return "", nil, "", true
	}
	resultText := resultEl.InnerText()
	if isSkippableResult(resultText) {
		return "", nil, "", true
	}

	ruleIDRef := result.SelectAttr("idref")
	rule, ok := ruleTable[ruleIDRef]
	if !ok {
		return "", nil, "", true
	}
	return ruleIDRef, rule, resultText, false
}

func isSkippableResult(resultText string) bool {
	return resultText == "notselected" || resultText == "notapplicable"
}

func buildAssessmentLog(rule, result *xmlquery.Node, ruleIDRef, resultText, target string) (plugin.AssessmentLog, bool, error) {
	ovalRefEl := findOVALCheckContentRef(rule)
	if ovalRefEl == nil {
		return plugin.AssessmentLog{}, true, nil
	}

	requirementID, err := parseCheck(ovalRefEl)
	if err != nil {
		return plugin.AssessmentLog{}, false, err
	}

	mappedResult, err := mapResultStatus(resultText)
	if err != nil {
		return plugin.AssessmentLog{}, false, err
	}

	return plugin.AssessmentLog{
		RequirementID: requirementID,
		Steps: []plugin.Step{
			{
				Name:    ruleIDRef,
				Result:  mappedResult,
				Message: ruleResultMessage(rule, result, resultText),
			},
		},
		Message:    fmt.Sprintf("Host %s evaluated", target),
		Confidence: plugin.ConfidenceLevelHigh,
	}, false, nil
}

func findOVALCheckContentRef(rule *xmlquery.Node) *xmlquery.Node {
	for _, check := range rule.SelectElements("//xccdf-1.2:check") {
		if check.SelectAttr("system") == ovalCheckType {
			return check.SelectElement("xccdf-1.2:check-content-ref")
		}
	}
	return nil
}

// mergeVariables combines global and target variable maps into a single
// config map. Target variables override global ones for the same key.
func mergeVariables(global, target map[string]string) map[string]string {
	merged := make(map[string]string, len(global)+len(target))
	for k, v := range global {
		merged[k] = v
	}
	for k, v := range target {
		merged[k] = v
	}
	return merged
}

func parseCheck(check *xmlquery.Node) (string, error) {
	ovalCheckName := strings.TrimSpace(check.SelectAttr("name"))
	if ovalCheckName == "" {
		return "", errors.New("check-content-ref node has no 'name' attribute")
	}
	matches := ovalRegex.FindStringSubmatch(ovalCheckName)

	minimumPart, shortNameLoc := 2, 1
	if len(matches) < minimumPart {
		return "", fmt.Errorf("check id %q is in unexpected format", ovalCheckName)
	}
	return matches[shortNameLoc], nil
}

// ruleResultMessage builds a human-readable step message from the XCCDF
// Rule definition and rule-result node. Prefers the rule title over the
// raw ID, and appends any diagnostic messages OpenSCAP emitted.
func ruleResultMessage(rule *xmlquery.Node, result *xmlquery.Node, resultText string) string {
	title := ""
	if el := rule.SelectElement("xccdf-1.2:title"); el != nil {
		title = strings.TrimSpace(el.InnerText())
	}

	var parts []string
	for _, msg := range result.SelectElements("message") {
		if t := strings.TrimSpace(msg.InnerText()); t != "" {
			parts = append(parts, t)
		}
	}
	diagnostic := strings.Join(parts, "; ")

	if title != "" && diagnostic != "" {
		return fmt.Sprintf("%s — %s (%s)", title, diagnostic, resultText)
	}
	if title != "" {
		return fmt.Sprintf("%s (%s)", title, resultText)
	}
	if diagnostic != "" {
		return fmt.Sprintf("%s (%s)", diagnostic, resultText)
	}
	return fmt.Sprintf("openscap rule-result is %s", resultText)
}

func mapResultStatus(resultText string) (plugin.Result, error) {
	switch resultText {
	case "pass", "fixed":
		return plugin.ResultPassed, nil
	case "fail":
		return plugin.ResultFailed, nil
	case "error", "unknown":
		return plugin.ResultError, nil
	}
	return plugin.ResultError, fmt.Errorf("couldn't match %s", resultText)
}

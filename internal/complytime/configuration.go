// SPDX-License-Identifier: Apache-2.0

package complytime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"encoding/json"

	"github.com/adrg/xdg"
	"github.com/hashicorp/go-hclog"
	"github.com/complytime/gemara2oscal/controls"
	"github.com/complytime/gemara2oscal/component"
	oscalTypes "github.com/defenseunicorns/go-oscal/src/types/oscal-1-1-3"
	"github.com/ossf/gemara/layer1"
	"github.com/ossf/gemara/layer2"
	"github.com/oscal-compass/compliance-to-policy-go/v2/framework"
	"github.com/oscal-compass/compliance-to-policy-go/v2/framework/actions"
	"github.com/oscal-compass/oscal-sdk-go/models"
	"github.com/oscal-compass/oscal-sdk-go/models/components"
	"github.com/oscal-compass/oscal-sdk-go/settings"
	"github.com/oscal-compass/oscal-sdk-go/validation"
)

const (
	compDefSuffix          = "component-definition.json"
	ApplicationDir         = "complytime"
	PluginDir              = "plugins"
	BundlesDir             = "bundles"
	GovernanceDir          = "governance"
	ControlsDir            = "controls"
	DataRootDir            = "/usr/share"
	PluginBinaryRootDir    = "/usr/libexec/"
	DefaultPluginConfigDir = "/etc/complytime/config.d/"
	Placeholder            = "REPLACE_ME"
)

// ErrNoComponentDefinitionsFound returns an error indicated the supplied directory
// does not contain component definitions that are detectable by complytime.
var ErrNoComponentDefinitionsFound = errors.New("no component definitions found")

// ApplicationDirectory represents the directories that make up
// the complytime application directory.
type ApplicationDirectory struct {
	// appDir is the top-level directory
	appDir string
	// pluginDir contains all complytime binary plugins.
	pluginDir string
	// pluginManifestDir contains all complytime plugin manifests.
	pluginManifestDir string
	// bundleDir contains all the detectable component definitions
	bundleDir string
	// governanceDir contains Gemara governance content
	governanceDir string
	// controlDir contains all OSCAL control layer models.
	controlDir string
}

// NewApplicationDirectory returns a new ApplicationDirectory.
//
// Creation of the directories is optional using the `create` input.
// If the application directories exist, this will not overwrite what is
// existing.
func NewApplicationDirectory(create bool, logger hclog.Logger) (ApplicationDirectory, error) {
	// When running local built complytime for development
	if os.Getenv("COMPLYTIME_DEV_MODE") == "1" {
		applicationDirectory := filepath.Join(xdg.DataHome, ApplicationDir)
		if _, err := os.Stat(applicationDirectory); err != nil {
			if os.IsNotExist(err) {
				logger.Info(fmt.Sprintf("Application directory not found, creating directory: %s", applicationDirectory))
			}
		}
		return newApplicationDirectory(xdg.DataHome, create)
	} else {
		return newApplicationDirectory(DataRootDir, false)
	}
}

// newApplicationDirectory returns a new ApplicationDirectory with the
// given root directory. Creation of the directories is optional using the
// `create` input. If the application directories exist, this will not overwrite what is
// existing.
func newApplicationDirectory(rootDir string, create bool) (ApplicationDirectory, error) {
	applicationDir := ApplicationDirectory{
		appDir: filepath.Join(rootDir, ApplicationDir),
	}
	// Drop-in configuration to be supported in CPLYTM-716
	applicationDir.pluginManifestDir = filepath.Join(applicationDir.appDir, PluginDir)
	if rootDir == DataRootDir {
		applicationDir.pluginDir = filepath.Join(PluginBinaryRootDir, ApplicationDir, PluginDir)
	} else {
		applicationDir.pluginDir = applicationDir.pluginManifestDir
	}
	applicationDir.bundleDir = filepath.Join(applicationDir.appDir, BundlesDir)
	applicationDir.governanceDir = filepath.Join(applicationDir.appDir, GovernanceDir)
	applicationDir.controlDir = filepath.Join(applicationDir.appDir, ControlsDir)
	if create {
		return applicationDir, applicationDir.create()
	}
	return applicationDir, nil
}

// create creates the application directories if they do not exist.
func (a ApplicationDirectory) create() error {
	for _, dir := range a.Dirs() {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("unable to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// AppDir returns the top-level application directory.
func (a ApplicationDirectory) AppDir() string {
	return a.appDir
}

// PluginDir returns the plugin directory below the AppDir.
func (a ApplicationDirectory) PluginDir() string {
	return a.pluginDir
}

// BundleDir returns the bundle directory containing the component
// definition.
func (a ApplicationDirectory) BundleDir() string {
	return a.bundleDir
}

// GovernanceDir returns the governance directory containing Gemara content.
func (a ApplicationDirectory) GovernanceDir() string {
	return a.governanceDir
}

// ControlDir returns the directory containing control layer OSCAL artifacts.
func (a ApplicationDirectory) ControlDir() string { return a.controlDir }

// PluginManifestDir returns the directory containing plugin manifests.
// definition.
func (a ApplicationDirectory) PluginManifestDir() string {
	return a.pluginManifestDir
}

// Dirs returns all directories in the ApplicationDirectory.
func (a ApplicationDirectory) Dirs() []string {
	return []string{
		a.appDir,
		a.pluginDir,
		a.pluginManifestDir,
		a.bundleDir,
		a.governanceDir,
		a.controlDir,
	}
}

// EnsureUserWorkspace creates the user workspace directory if it doesn't exist.
// This function should be called early in command execution to ensure the workspace
// is available before any operations that depend on it.
func EnsureUserWorkspace(userWorkspace string) error {
	if err := os.MkdirAll(userWorkspace, 0700); err != nil {
		return fmt.Errorf("failed to create user workspace directory %s: %w", userWorkspace, err)
	}
	return nil
}

// FindComponentDefinitions locates all the OSCAL Component Definitions in the
// given `bundles` directory that meet the defined naming scheme.
//
// The defined scheme is $COMPONENT-NAME-component-definition.json.
func FindComponentDefinitions(bundleDir string, validator validation.Validator) ([]oscalTypes.ComponentDefinition, error) {
	items, err := os.ReadDir(bundleDir)
	if err != nil {
		return nil, fmt.Errorf("unable to read bundle directory %s: %w", bundleDir, err)
	}

	var compDefBundles []oscalTypes.ComponentDefinition
	for _, item := range items {
		if !strings.HasSuffix(item.Name(), compDefSuffix) {
			continue
		}
		compDefPath := filepath.Join(bundleDir, item.Name())
		compDefPath = filepath.Clean(compDefPath)
		file, err := os.Open(compDefPath)
		if err != nil {
			return nil, err
		}
		definition, err := models.NewComponentDefinition(file, validator)
		if err != nil {
			return nil, err
		}
		if definition == nil {
			return nil, fmt.Errorf("could not load component definition from %s", compDefPath)
		}
		compDefBundles = append(compDefBundles, *definition)
	}
	if len(compDefBundles) == 0 {
		return nil, fmt.Errorf("directory %s: %w", bundleDir, ErrNoComponentDefinitionsFound)
	}
	return compDefBundles, nil
}

// FindAllComponentDefinitions locates all OSCAL component definitions from the bundle directory
// and all Gemara content in the governance directory.  Gemara content is converted
// into OSCAL component definitions.
func FindAllComponentDefinitions(appDir ApplicationDirectory, validator validation.Validator, logger hclog.Logger) ([]oscalTypes.ComponentDefinition, error) {
	logger.Debug("Scanning bundles directory", "dir", appDir.BundleDir())
	defs, err := FindComponentDefinitions(appDir.BundleDir(), validator)
	if err != nil {
		logger.Error("Failed to load OSCAL component definitions from bundles", "dir", appDir.BundleDir(), "err", err)
		return nil, err
	}

	logger.Debug("Scanning governance directory", "dir", appDir.GovernanceDir())
	if cats, ok, cerr := loadLayer2Catalogs(appDir.GovernanceDir(), logger); cerr != nil {
		logger.Error("Failed loading Gemara Layer 2 catalogs", "dir", filepath.Join(appDir.GovernanceDir(), "catalogs"), "err", cerr)
		return nil, cerr
	} else if ok {
		logger.Debug("Detected Gemara Layer 2 catalogs", "dir", filepath.Join(appDir.GovernanceDir(), "catalogs"), "count", len(cats))
		// Load parameters from governance directory if present
		params, _ := loadGemaraParameters(appDir.GovernanceDir(), logger)
		for _, cat := range cats {
			def := catalogToCompDef(cat, params)
			defs = append(defs, def)
		}
	}

	profilesByGuidance, gerr := prepareGuidanceProfiles(appDir.GovernanceDir(), appDir, logger)
	if gerr != nil {
		return nil, gerr
	}
	if len(profilesByGuidance) > 0 {
		logger.Info("Attempting to repoint control implementation sources using Layer1 profiles", "profiles", len(profilesByGuidance))
		for di := range defs {
			def := &defs[di]
			if def.Components == nil {
				continue
			}
			for ci := range *def.Components {
				comp := &(*def.Components)[ci]
				if comp.ControlImplementations == nil {
					continue
				}
				for ki := range *comp.ControlImplementations {
					impl := &(*comp.ControlImplementations)[ki]
					frameworkID, ok := settings.GetFrameworkShortName(*impl)
					if !ok {
						continue
					}
					if href, found := profilesByGuidance[strings.ToLower(frameworkID)]; found {
						logger.Info("Repointing control implementation source to local profile", "frameworkId", frameworkID, "oldSource", impl.Source, "newSource", href, "component", comp.Title)
						impl.Source = href
					}
				}
			}
		}
	}
	return defs, nil
}

// loadLayer2Catalogs loads Gemara Layer 2 catalogs from a given directory.
// Returns catalogs, whether any were found, and an error for failures.
func loadLayer2Catalogs(dir string, logger hclog.Logger) ([]layer2.Catalog, bool, error) {
	var catalogs []layer2.Catalog
	searchRoot := filepath.Join(dir, "catalogs")
	if _, err := os.Stat(searchRoot); err != nil {
		logger.Debug("Catalogs directory not found", "path", searchRoot)
		return nil, false, nil
	}
	entries, err := os.ReadDir(searchRoot)
	if err != nil {
		logger.Debug("Catalogs directory unreadable", "path", searchRoot, "err", err)
		return nil, false, fmt.Errorf("failed to read catalogs directory %s: %w", searchRoot, err)
	}

	// Load each file individually as a catalog
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		cleanedPath := filepath.Clean(filepath.Join(searchRoot, e.Name()))
		var cat layer2.Catalog
		if err := cat.LoadFile(fmt.Sprintf("file://%s", cleanedPath)); err != nil {
			return nil, false, fmt.Errorf("failed to load catalog file %s: %w", cleanedPath, err)
		}
		// Minimal validation
		if cat.Metadata.Id != "" && len(cat.ControlFamilies) > 0 {
			catalogs = append(catalogs, cat)
		}
	}

	return catalogs, len(catalogs) > 0, nil
}

// loadGemaraParameters attempts to load Gemara parameters from common locations within governance.
// It returns loaded parameters and a boolean indicating whether any file was found/loaded.
func loadGemaraParameters(governanceDir string, logger hclog.Logger) (component.Parameters, bool) {
	var params component.Parameters
	candidates := []string{
		filepath.Join(governanceDir, "parameters.yaml"),
		filepath.Join(governanceDir, "parameters.yml"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			if loadErr := params.Load(p); loadErr == nil {
				logger.Debug("Loaded Gemara parameters", "file", p)
				return params, true
			}
		}
	}
	return component.Parameters{}, false
}

// catalogToCompDef converts a single Gemara Layer 2 catalog into an OSCAL ComponentDefinition.
// The component and metadata title are derived from the catalog metadata (preferred).
func catalogToCompDef(cat layer2.Catalog, params component.Parameters) oscalTypes.ComponentDefinition {
	componentTitle := cat.Metadata.Title
	if componentTitle == "" {
		componentTitle = cat.Metadata.Id
	}
	version := "v0"
	builder := component.NewDefinitionBuilder(componentTitle, version)
	builder.AddTargetComponent(componentTitle, "system", cat, params)
	return builder.Build()
}

// loadLayer1Guidance loads Gemara Layer 1 guidance documents from governance/guidance
// and returns the successfully parsed documents.
func loadLayer1Guidance(governanceDir string, logger hclog.Logger) ([]layer1.GuidanceDocument, error) {
	var docs []layer1.GuidanceDocument
	guidanceRoot := filepath.Join(governanceDir, "guidance")
	if _, err := os.Stat(guidanceRoot); err != nil {
		logger.Debug("Guidance directory not found", "path", guidanceRoot)
		return docs, nil
	}
	entries, err := os.ReadDir(guidanceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read guidance directory %s: %w", guidanceRoot, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		cleanedPath := filepath.Clean(filepath.Join(guidanceRoot, e.Name()))
		var gd layer1.GuidanceDocument
		if err := gd.LoadFile(fmt.Sprintf("file://%s", cleanedPath)); err != nil {
			return nil, fmt.Errorf("failed to load guidance file %s: %w", cleanedPath, err)
		}
		docs = append(docs, gd)
	}
	return docs, nil
}

// writeProfile writes an OSCAL Profile under controls/ and returns the local file href.
func writeProfile(profile oscalTypes.Profile, guidanceID string, appDir ApplicationDirectory, logger hclog.Logger) (string, error) {
	filename := fmt.Sprintf("%s.profile.json", guidanceID)
	dstPath := filepath.Join(appDir.ControlDir(), filename)
	payload := oscalTypes.OscalModels{
		Profile: &profile,
	}
	data, err := json.MarshalIndent(payload, "", " ")
	if err != nil {
		return "", fmt.Errorf("marshal profile %s: %w", guidanceID, err)
	}
	if err := os.WriteFile(dstPath, data, 0600); err != nil {
		return "", fmt.Errorf("write profile %s: %w", dstPath, err)
	}
	href := fmt.Sprintf("file://%s", filepath.Join(ControlsDir, filename))
	logger.Debug("Wrote local OSCAL profile from guidance", "guidance", guidanceID, "href", href)
	return href, nil
}

// writeCatalog writes an OSCAL Catalog under controls/ and returns the local file href.
func writeCatalog(catalog oscalTypes.Catalog, guidanceID string, appDir ApplicationDirectory, logger hclog.Logger) (string, error) {
	filename := fmt.Sprintf("%s.catalog.json", guidanceID)
	dstPath := filepath.Join(appDir.ControlDir(), filename)
	payload := oscalTypes.OscalModels{
		Catalog: &catalog,
	}
	data, err := json.MarshalIndent(payload, "", " ")
	if err != nil {
		return "", fmt.Errorf("marshal catalog %s: %w", guidanceID, err)
	}
	if err := os.WriteFile(dstPath, data, 0600); err != nil {
		return "", fmt.Errorf("write catalog %s: %w", dstPath, err)
	}
	href := fmt.Sprintf("file://%s", filepath.Join(ControlsDir, filename))
	logger.Debug("Wrote local OSCAL catalog from guidance", "guidance", guidanceID, "href", href)
	return href, nil
}

// prepareGuidanceProfiles orchestrates loading L1 guidance, converting to OSCAL Profiles,
// writing to controls/ (along with catalogs), and returns a map of guidanceId -> profile href.
func prepareGuidanceProfiles(governanceDir string, appDir ApplicationDirectory, logger hclog.Logger) (map[string]string, error) {
	result := make(map[string]string)
	docs, err := loadLayer1Guidance(governanceDir, logger)
	if err != nil {
		return nil, err
	}
	logger.Debug("Layer1 guidance documents discovered", "count", len(docs), "governanceDir", governanceDir)
	for _, gd := range docs {
		gid := gd.Metadata.Id
		if gid == "" {
			// Derive id from a stable surrogate if missing
			gid = strings.ToLower(strings.ReplaceAll(gd.Metadata.Title, " ", "-"))
			if gid == "" {
				logger.Info("Skipping guidance with no identifiable ID or title")
				continue
			}
		}
		logger.Info("Processing guidance", "guidanceId", gid, "title", gd.Metadata.Title)
		// First create a catalog from guidance and write it
		catalog, err := controls.ToOSCALCatalog(gd)
		if err != nil {
			logger.Info("Catalog conversion failed", "guidance", gid, "err", err)
			continue
		}
		catalogHref, err := writeCatalog(catalog, gid, appDir, logger)
		if err != nil {
			logger.Info("Catalog write failed", "guidance", gid, "err", err)
			continue
		}
		// Build profile and make it import the generated catalog for title resolution
		profile, err := controls.ToOSCALProfile(gd, gid)
		if err != nil {
			logger.Info("Profile conversion failed", "guidance", gid, "err", err)
			continue
		}
		profile.Imports = []oscalTypes.Import{
			{
				Href:       catalogHref,
				IncludeAll: &oscalTypes.IncludeAll{},
			},
		}
		profileHref, err := writeProfile(profile, gid, appDir, logger)
		if err != nil {
			logger.Info("Profile write failed", "guidance", gid, "err", err)
			continue
		}
		logger.Info("Prepared local profile for guidance", "guidanceId", gid, "profileHref", profileHref, "catalogHref", catalogHref)
		// Use lower-cased key to allow case-insensitive matching later
		result[strings.ToLower(gid)] = profileHref
	}
	logger.Info("Layer1 guidance profiles prepared", "count", len(result))
	return result, nil
}
// Config creates a new C2P config for the ComplyTime CLI to use to configure
// the plugin manager.
func Config(a ApplicationDirectory) (*framework.C2PConfig, error) {
	cfg := framework.DefaultConfig()
	cfg.PluginDir = a.PluginDir()
	cfg.PluginManifestDir = a.PluginManifestDir()
	return cfg, nil
}

// ActionsContextFromPlan returns a new actions.InputContext from a given OSCAL AssessmentPlan.
func ActionsContextFromPlan(assessmentPlan *oscalTypes.AssessmentPlan) (*actions.InputContext, error) {
	if assessmentPlan.AssessmentAssets.Components == nil {
		return nil, errors.New("assessment plan has no assessment components")
	}
	var allComponents []components.Component
	for _, component := range *assessmentPlan.AssessmentAssets.Components {
		compAdapter := components.NewSystemComponentAdapter(component)
		allComponents = append(allComponents, compAdapter)
	}
	inputContext, err := actions.NewContextFromComponents(allComponents)
	if err != nil {
		return nil, fmt.Errorf("error generating context from plan %s: %w", assessmentPlan.Metadata.Title, err)
	}
	apSettings, err := Settings(assessmentPlan)
	if err != nil {
		return nil, fmt.Errorf("cannot extract settings from plan %s: %w", assessmentPlan.Metadata.Title, err)
	}
	inputContext.Settings = apSettings
	return inputContext, nil
}

// Config default value if it is a placeholder
func replaceString(current_value string, default_value string) string {
	if current_value == Placeholder {
		return default_value
	}
	return current_value
}

// Replace the placeholders for assessment plan
func replacePlaceholdersInPlan(plan *oscalTypes.AssessmentPlan, frameworkId string) {
	if plan == nil {
		return
	}

	// 1. Handle assessment-plan.metadata.title assessment-plan.assessment-assets.assessment-platforms.title
	plan.Metadata.Title = replaceString(
		plan.Metadata.Title,
		fmt.Sprintf("Assessment plan for '%s'", frameworkId),
	)
	// 2. Handle assessment-plan.assessment-assets.import-ssp.href
	plan.ImportSsp.Href = replaceString(
		plan.ImportSsp.Href,
		"ImportSsp Href has not been set.",
	)

	// 3. Handle assessment-plan.assessment-assets.assessment-platforms.title
	if plan.AssessmentAssets != nil && plan.AssessmentAssets.AssessmentPlatforms != nil {
		for i := range plan.AssessmentAssets.AssessmentPlatforms {
			platforms := plan.AssessmentAssets.AssessmentPlatforms
			platforms[i].Title = replaceString(
				platforms[i].Title,
				"The AssessmentPlatforms title has not been set.",
			)
		}
	}
	// 4. Handle assessment-plan.assessment-assets.back-matter.resources.description
	if plan.BackMatter != nil && plan.BackMatter.Resources != nil {
		resources := *plan.BackMatter.Resources
		for i := range resources {
			resources[i].Description = replaceString(
				resources[i].Description,
				"The description of BackMatter Resource has not been set.",
			)
		}
	}
}


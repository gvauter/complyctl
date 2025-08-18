// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/complytime/complyctl/cmd/complyctl/option"
	"github.com/complytime/complyctl/internal/complytime"

	g2o_component "github.com/jpower432/gemara2oscal/component"
	g2o_controls "github.com/jpower432/gemara2oscal/controls"
	oscalTypes "github.com/defenseunicorns/go-oscal/src/types/oscal-1-1-3"
	"github.com/goccy/go-yaml"
	"github.com/ossf/gemara/layer1"
	"github.com/ossf/gemara/layer2"
	"github.com/ossf/gemara/layer3"
	"github.com/ossf/gemara/layer4"
)

func transformCmd(common *option.Common) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "transform",
		Short:        "Transform Gemara artifacts into OSCAL",
		SilenceUsage: true,
	}
	cmd.AddCommand(transformComponentCmd(common), transformCatalogCmd(common))
	return cmd
}


type gemaraOptions struct {
	*option.Common
	complyTimeOpts   *option.ComplyTime
	layer2Path       string
	layer4Path       string
	layer3Path       string
	referenceID      string
	title            string
	componentType    string
	validationSource string
}

func transformComponentCmd(common *option.Common) *cobra.Command {
	opts := &gemaraOptions{
		Common:         common,
		complyTimeOpts: &option.ComplyTime{},
		componentType:  "software",
	}
	cmd := &cobra.Command{
		Use:          "component [flags]",
		Short:        "Convert Gemara to an OSCAL Component Definition",
		Example:      "complyctl transform component --layer2 ./l2.yaml --layer4 ./l4.yaml --title 'My App'",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTransformComponent(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.layer2Path, "layer2", "", "Path to Gemara Layer 2 catalog")
	cmd.Flags().StringVar(&opts.layer4Path, "layer4", "", "Path to Gemara Layer 4 evaluations")
	cmd.Flags().StringVar(&opts.layer3Path, "layer3", "", "Path to optional Gemara Layer 3 parameter modifiers")
	cmd.Flags().StringVar(&opts.referenceID, "reference-id", "", "Layer 2 mapping reference id for parameter modifiers (required if --layer3 is set)")
	cmd.Flags().StringVarP(&opts.title, "title", "t", "", "Component definition title")
	cmd.Flags().StringVar(&opts.componentType, "component-type", opts.componentType, "OSCAL component type for target component (default: software)")
	cmd.Flags().StringVar(&opts.validationSource, "validation-source", "", "Validation component source label")
	opts.complyTimeOpts.BindFlags(cmd.Flags())
	return cmd
}

func runTransformComponent(_ *cobra.Command, opts *gemaraOptions) error {
	
	if strings.TrimSpace(opts.layer2Path) == "" || strings.TrimSpace(opts.layer4Path) == "" {
		return errors.New("layer2 and layer4 are required")
	}

	// Load Gemara Layer 2 catalog
	var l2Catalog layer2.Catalog
	if err := readYAML(opts.layer2Path, &l2Catalog); err != nil {
		return fmt.Errorf("failed to load catalog: %w", err)
	}

	// Load Gemara Layer 4 evaluations
	var l4Evaluations []layer4.ControlEvaluation
	if err := readYAML(opts.layer4Path, &l4Evaluations); err != nil {
		return fmt.Errorf("failed to load layer4: %w", err)
	}

	// Optional Layer 3 modifiers
	var l3Modifiers []layer3.ParameterModifier
	if strings.TrimSpace(opts.layer3Path) != "" {
		if strings.TrimSpace(opts.referenceID) == "" {
			return errors.New("reference-id is required when --layer3 is provided")
		}
		if err := readYAML(opts.layer3Path, &l3Modifiers); err != nil {
			return fmt.Errorf("failed to load layer3: %w", err)
		}
	}

	// Determine title and validation source defaults
	title := opts.title
	if title == "" {
		title = l2Catalog.Metadata.Title
		if title == "" {
			title = "Component"
		}
	}
	validationSource := opts.validationSource
	if validationSource == "" {
		validationSource = filepath.Base(opts.layer4Path)
	}

	// Build Component Definition via gemara2oscal (hard-coded version for parity)
	builder := g2o_component.NewDefinitionBuilder(title, "0.1.0")
	builder.AddTargetComponent(title, opts.componentType, l2Catalog)
	if len(l3Modifiers) > 0 {
		builder.AddParameterModifiers(opts.referenceID, l3Modifiers)
	}
	builder.AddValidationComponent(validationSource, l4Evaluations)
	compDef := builder.Build()

	// Determine output path under application bundle directory
	appDir, err := complytime.NewApplicationDirectory(true)
	if err != nil {
		return fmt.Errorf("failed to initialize application directory: %w", err)
	}
	outName := fmt.Sprintf("%s-component-definition.json", toKebab(title))
	outPath := filepath.Join(appDir.BundleDir(), outName)

	data, err := json.MarshalIndent(compDef, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal component definition: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write component definition: %w", err)
	}
	logger.Info(fmt.Sprintf("Component definition written: %s", outPath))
	return nil
}

func transformCatalogCmd(common *option.Common) *cobra.Command {
	var guidancePath string
	cmd := &cobra.Command{
		Use:          "catalog [flags]",
		Short:        "Convert Gemara Layer 1 guidance to an OSCAL Catalog",
		Example:      "complyctl transform catalog --guidance-path ./guidance.yml",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(guidancePath) == "" {
				return errors.New("--guidance-path is required")
			}

			var l1 layer1.GuidanceDocument
			if err := readYAML(guidancePath, &l1); err != nil {
				return fmt.Errorf("failed to load guidance: %w", err)
			}
			catalog, err := g2o_controls.ToCatalog(l1)
			if err != nil {
				return fmt.Errorf("failed to build catalog: %w", err)
			}
			models := oscalTypes.OscalModels{Catalog: &catalog}
			out, err := json.MarshalIndent(models, "", "  ")
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(os.Stdout, string(out))
			return nil
		},
	}
	cmd.Flags().StringVarP(&guidancePath, "guidance-path", "g", "", "Path to Layer 1 guidance document")
	return cmd
}

func toKebab(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "component"
	}
	return s
}

func readYAML(path string, out interface{}) error {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, out)
} 
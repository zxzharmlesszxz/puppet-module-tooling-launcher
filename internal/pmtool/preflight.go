package pmtool

import (
	"fmt"
	"strings"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/fixturekit"
	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/hostflow"
	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/templatekit"
)

func missingInputs(command string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	return fmt.Errorf("%s: missing required inputs: %s", command, strings.Join(names, ", "))
}
func (a *app) preflightWorkflow(args []string, dry bool) error {
	o := a.parseOperation(args)
	if dry {
		return nil
	}
	var missing []string
	if launcherManifest().Commands[o.Command].TemplateToken && a.templateOperation == nil {
		settings, err := hostflow.ReadSettings(o.Module, hostflow.Settings{Template: o.Settings.Template, CI: o.Settings.CI, Provider: o.Settings.Provider})
		if err != nil {
			return err
		}
		if settings.Template == "" {
			missing = append(missing, "template.use in .sync.yml or --template")
		} else {
			reference, err := templatekit.ParseReference(settings.Template)
			if err != nil {
				return err
			}
			if a.getenv("TOOLING_TEMPLATE_TOKEN") != "" && a.getenv("TOOLING_TEMPLATE_TOKEN_HOST") == "" {
				missing = append(missing, "TOOLING_TEMPLATE_TOKEN_HOST")
			} else if _, err := templatekit.NewCredentials(reference.Repository, a.getenv); err != nil {
				return err
			}
		}
	}
	if o.Command == "check" || o.Command == "voxbox" && o.Stage != "validate" {
		if err := fixturekit.Validate(o.Module, a.getenv); err != nil {
			return err
		}
	}
	return missingInputs(o.Command, missing)
}
func (a *app) preflightForge(dry bool) error {
	var missing []string
	if a.getenv("FORGE_URL") == "" {
		missing = append(missing, "FORGE_URL")
	}
	if !dry && a.getenv("FORGE_PUBLISH_TOKEN") == "" {
		missing = append(missing, "FORGE_PUBLISH_TOKEN")
	}
	return missingInputs("forge-publish", missing)
}

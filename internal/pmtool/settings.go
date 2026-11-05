package pmtool

import (
	"strings"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/internal/hostflow"
)

type workflowSettings struct {
	CI       string `json:"ci"`
	Provider string `json:"provider"`
	Template string `json:"template"`
}

func (a *app) configuredOrigin(module string) (string, error) {
	return hostflow.ConfiguredOrigin(func(args ...string) (string, error) {
		output, err := a.driver.capture("git", append([]string{"-C", module}, args...))
		return string(output), err
	})
}

func (a *app) resolveWorkflowSettings(module string, cli workflowSettings) workflowSettings {
	selected := must(hostflow.ReadSettings(module, hostflow.Settings{CI: cli.CI, Provider: cli.Provider, Template: cli.Template}))
	if selected.Provider == "" {
		origin, err := a.configuredOrigin(module)
		if err != nil {
			fail("cannot read origin from module: %v", err)
		}
		_, provider, err := hostflow.OriginContext(origin, "")
		check(err)
		selected.Provider = provider
	}
	if strings.ContainsAny(selected.CI, "\r\n") {
		fail("CI reference must be a single line")
	}
	if selected.Provider != "gitlab" && selected.Provider != "github" {
		fail("provider must be gitlab or github")
	}
	return workflowSettings{selected.CI, selected.Provider, selected.Template}
}

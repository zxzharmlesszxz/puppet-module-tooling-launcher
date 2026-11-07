package pmtool

import (
	"context"
	"fmt"
	"slices"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/fixturekit"
)

func (a *app) executeToolStep(cfg config, o operationOptions, step workflowStep, native bool) (code int) {
	p := a.toolPlan(cfg, step, native)
	p.Directory = o.Module
	if step.Tool != "voxbox" || !slices.Contains(toolArguments(step.Command), "spec") {
		return a.driver.execute(p)
	}
	prepared, err := fixturekit.Prepare(context.Background(), o.Module, fixturekit.Options{Native: native, Getenv: a.getenv})
	if err != nil {
		a.notice("FAIL", fmt.Sprintf("test fixtures: %v", err), "1;31")
		return 1
	}
	if prepared != nil {
		defer func() {
			if err := prepared.Close(); err != nil {
				a.notice("FAIL", "cannot remove temporary test fixtures", "1;31")
				if code == 0 {
					code = 1
				}
			}
		}()
		a.notice("OK", "Verified host-side test fixtures; no Forge token forwarded to SDK", "32")
		p = withFixtureManifest(p, prepared.Manifest, native)
	}
	return a.driver.execute(p)
}

func withFixtureManifest(p plan, manifest string, native bool) plan {
	if native {
		if p.Environment == nil {
			p.Environment = map[string]string{}
		}
		p.Environment["FIXTURES_YML"] = manifest
	} else {
		p.Args = append([]string{p.Args[0], "--env", "FIXTURES_YML=" + manifest}, p.Args[1:]...)
	}
	return p
}

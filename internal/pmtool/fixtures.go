package pmtool

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/fixturekit"
)

func (a *app) executeToolStep(cfg config, o operationOptions, step workflowStep, native bool) (code int) {
	p := a.toolPlan(cfg, step, native)
	p.Directory = o.Module
	if step.Tool != "voxbox" || !slices.Contains(toolArguments(step.Command), "spec") {
		return a.driver.execute(p)
	}
	prepared, err := fixturekit.Prepare(o.Module, fixturekit.Options{Native: native, Getenv: a.getenv})
	if err != nil {
		a.notice("FAIL", fmt.Sprintf("Forge fixtures: %v", err), "1;31")
		return 1
	}
	if prepared == nil {
		return a.driver.execute(p)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			a.notice("FAIL", "cannot remove temporary Forge installer inputs", "1;31")
			if code == 0 {
				code = 1
			}
		}
	}()
	a.notice("STEP", "Prepare Forge fixtures", "36")
	if code = a.driver.execute(a.forgeFixturePlan(cfg, o, step, prepared, native, true)); code != 0 {
		return code
	}
	if err := prepared.RemoveCredentials(); err != nil {
		a.notice("FAIL", "cannot remove Forge credentials before specs", "1;31")
		return 1
	}
	a.notice("OK", "Forge fixtures prepared; credentials removed before specs", "32")
	return a.driver.execute(a.forgeFixturePlan(cfg, o, step, prepared, native, false))
}

func (a *app) forgeFixturePlan(cfg config, o operationOptions, step workflowStep, prepared *fixturekit.Prepared, native, install bool) plan {
	tasks := []string{"spec:standalone", "fixtures:clean"}
	if install {
		tasks = []string{"fixtures:prep"}
	}
	index := len(step.Command) - len(toolArguments(step.Command)) - 1
	selected := step
	selected.Command = append(slices.Clone(step.Command[:index+1]), tasks...)
	p := a.toolPlan(cfg, selected, native)
	p.Directory = o.Module
	worker := append([]string{"exec", "rake", "-f", "/opt/voxbox/Rakefile"}, tasks...)
	if install && prepared.Installer != "" {
		config := filepath.Join(filepath.Dir(prepared.Manifest), "puppet.conf")
		worker = []string{"exec", "ruby", prepared.Installer, config}
	}
	if native {
		p.Args = worker
		p.Environment["FIXTURES_YML"] = ""
		if install {
			p.Environment["FIXTURES_YML"] = prepared.Manifest
		}
		return p
	}
	var args []string
	for i := 1; i < index; i++ {
		if step.Command[i] == "--tmpfs" && i+1 < index && step.Command[i+1] == "/repo/spec/fixtures/modules" {
			i++
			continue
		}
		args = append(args, step.Command[i])
	}
	args = append(args, containerIdentity(profile{Module: true})...)
	args = append(args, "--mount", mount(prepared.Modules, "/repo/spec/fixtures/modules", false), "--env", "HOME=/tmp",
		"--env", "BUNDLE_IGNORE_CONFIG=1", "--env", "BUNDLE_GEMFILE=/opt/voxbox/Gemfile", "--env", "BUNDLE_PATH=/opt/voxbox/vendor/bundle")
	if install {
		args = append(args, "--mount", mount(prepared.Auth, "/pmtool-forge", true), "--env", "FIXTURES_YML="+prepared.Manifest)
	}
	args = append(args, "--entrypoint", "bundle", step.Command[index])
	p.Args = append(args, worker...)
	p.Environment = privateValidationEnvironment()
	return p
}

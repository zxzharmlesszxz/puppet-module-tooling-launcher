package pmtool

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/internal/hostflow"
	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/internal/templatekit"
)

func (a *app) executeTemplate(cfg config, image string, o operationOptions, kind string, native bool) error {
	if a.templateOperation != nil {
		return a.templateOperation(o, kind, native)
	}
	if kind == "migrate" {
		return templatekit.Migrate(o.Module)
	}
	selected := a.resolveWorkflowSettings(o.Module, o.Settings)
	if selected.Template == "" {
		return fmt.Errorf("set template.use in .sync.yml or --template repository@version")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	origin, err := a.driver.capture("git", []string{"-C", o.Module, "remote", "get-url", "origin"})
	if err != nil {
		return err
	}
	host, _, err := hostflow.OriginContext(strings.TrimSpace(string(origin)), selected.Provider)
	if err != nil {
		return err
	}
	return templatekit.Checkout(ctx, selected.Template, a.getenv, nil, func(t *templatekit.Template) error {
		git := &hostflow.Git{Root: o.Module, Context: ctx}
		e := &templatekit.Engine{Root: o.Module, Provider: selected.Provider, OriginHost: host, CI: selected.CI, LegacyCI: o.CIProject, Files: o.Files, Template: t, Git: git.Run, Out: a.stdout, Context: ctx, Color: a.colorEnabled()}
		e.Render = func(ctx context.Context, docs []templatekit.Document) ([][]byte, error) {
			return a.renderTemplate(cfg, image, native, docs)
		}
		e.Check = func(ctx context.Context, root, path, kind string) error {
			p := a.checkPlan(cfg, image, native, root, path, kind)
			if code := a.driver.execute(p); code != 0 {
				return commandFailure{"audit check", code}
			}
			return nil
		}
		return e.Run(kind != "audit", kind == "sync-noop")
	})
}
func (a *app) renderTemplate(cfg config, image string, native bool, documents []templatekit.Document) ([][]byte, error) {
	p := plan{Docker: "ruby", Args: []string{"-e", templatekit.RendererScript}, Input: must(templatekit.RenderRequest(documents)), Environment: privateValidationEnvironment()}
	if !native {
		p.Docker = cfg.Docker
		p.Args = []string{"run", "--rm", "--read-only", "--tmpfs", "/tmp:exec", "--network", "none", "--env", "HOME=/tmp"}
		p.Args = append(p.Args, containerIdentity(profile{Module: true})...)
		p.Args = append(p.Args, "--interactive", "--entrypoint", "ruby", image, "-e", templatekit.RendererScript)
	}
	data, err := a.driver.capturePlan(p)
	if err != nil {
		return nil, err
	}
	return templatekit.RenderResponse(data)
}
func privateValidationEnvironment() map[string]string {
	r := map[string]string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") || strings.HasPrefix(key, "TOOLING_TEMPLATE_") || strings.HasPrefix(key, "PMTOOL_TEMPLATE_") || strings.HasPrefix(key, "BUNDLE_") {
			r[key] = ""
		}
	}
	for _, key := range []string{"RUBYOPT", "RUBYLIB"} {
		r[key] = ""
	}
	for _, key := range []string{"GITLAB_TOKEN", "GITHUB_TOKEN", "CI_JOB_TOKEN", "TOOLING_TEMPLATE_TOKEN", "TOOLING_TEMPLATE_TOKEN_HOST", "TOOLING_TEMPLATE_USERNAME", "TOOLING_TEMPLATE_REPOSITORY", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		r[key] = ""
	}
	return r
}
func (a *app) checkPlan(cfg config, image string, native bool, root, path, kind string) plan {
	script := filepath.Join(root, path)
	args := []string{script, root}
	p := plan{Docker: "ruby", Args: args, Directory: root, Environment: privateValidationEnvironment()}
	if kind == "voxbox" {
		p.Docker = "bundle"
		p.Args = append([]string{"exec", "ruby"}, args...)
		p.Environment["BUNDLE_IGNORE_CONFIG"] = "1"
		p.Environment["BUNDLE_GEMFILE"] = "/opt/voxbox/Gemfile"
		p.Environment["BUNDLE_PATH"] = "/opt/voxbox/vendor/bundle"
	}
	if native {
		return p
	}
	p.Docker = cfg.Docker
	p.Args = []string{"run", "--rm", "--read-only", "--tmpfs", "/tmp:exec", "--network", "none", "--env", "HOME=/tmp", "--mount", mount(root, root, true), "--workdir", root}
	p.Args = append(p.Args, containerIdentity(profile{Module: true})...)
	if kind == "voxbox" {
		p.Args = append(p.Args, "--env", "BUNDLE_IGNORE_CONFIG=1", "--env", "BUNDLE_GEMFILE=/opt/voxbox/Gemfile", "--env", "BUNDLE_PATH=/opt/voxbox/vendor/bundle", "--entrypoint", "bundle", image, "exec", "ruby")
	} else {
		p.Args = append(p.Args, "--entrypoint", "ruby", image)
	}
	p.Args = append(p.Args, args...)
	return p
}

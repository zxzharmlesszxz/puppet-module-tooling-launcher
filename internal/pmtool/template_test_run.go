package pmtool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/hostflow"
	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/templatekit"
)

func (a *app) runTemplateTest(cfg config, image string, args []string, native, dry bool) int {
	if len(args) != 0 && (len(args) != 2 || args[0] != "--module") {
		fail("use template-test [--module PATH]")
	}
	root := moduleArgument(a.cwd, args)
	if dry {
		must(fmt.Fprintln(a.stdout, `{"operation":"template-test","executor":"go"}`))
		return 0
	}
	a.notice("RUN", "template-test", "1;36")
	if err := a.templateSuite(cfg, image, root, native); err != nil {
		a.notice("FAIL", err.Error(), "1;31")
		return operationExit(err)
	}
	a.notice("OK", "template-test completed", "1;32")
	return 0
}
func (a *app) templateSuite(cfg config, image, root string, native bool) (caught error) {
	t, err := templatekit.Load(root, "git.example.invalid/fixture/template@v0.0.0", "")
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "pmtool-template-test-*")
	if err != nil {
		return err
	}
	defer func() {
		err := os.RemoveAll(tmp)
		if caught == nil {
			caught = err
		}
	}()
	fixture := filepath.Join(tmp, "module")
	if err := os.Mkdir(fixture, 0700); err != nil {
		return err
	}
	metadata := map[string]any{"name": "fixture-sample", "version": "0.0.0"}
	body, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(fixture, "metadata.json"), body, 0600); err != nil {
		return err
	}
	rule, _ := t.Audit["metadata"].(map[string]any)
	if change, ok := rule["changelog"].(map[string]any); ok {
		path, err := templatekit.SafePath(fixture, stringValue(change["file"]))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		text := "## " + strings.ReplaceAll(stringValue(change["heading"]), "%{version}", "0.0.0") + "\n"
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			return err
		}
	}
	ctx := context.Background()
	git := &hostflow.Git{Root: fixture, Context: ctx}
	if _, err := git.Run("init", "--quiet"); err != nil {
		return err
	}
	provider := "gitlab"
	if _, ok := t.CI[provider]; !ok {
		provider = "github"
	}
	explicit := ""
	if len(t.CI) == 0 {
		provider = "gitlab"
		explicit = "git.example.invalid/fixture/template//ci/module.yml@v1"
	}
	e := &templatekit.Engine{Root: fixture, Provider: provider, OriginHost: "git.example.invalid", CI: explicit, Template: t, Context: ctx, Out: a.stdout, Git: git.Run, Render: func(_ context.Context, docs []templatekit.Document) ([][]byte, error) {
		return a.renderTemplate(cfg, image, native, docs)
	}}
	if err := e.Run(true, false); err != nil {
		return err
	}
	result, err := e.Plan(false)
	if err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("template baseline self-check: %s", strings.Join(result.Errors, "; "))
	}
	data, err := templatekit.Fixture(t)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	file := filepath.Join(tmp, "fixture.json")
	if err := os.WriteFile(file, encoded, 0600); err != nil {
		return err
	}
	run := "test/run.rb"
	if _, err := templatekit.SafePath(root, run); err != nil {
		return err
	}
	if info, err := os.Stat(filepath.Join(root, run)); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("template must provide test/run.rb")
	}
	p := a.checkPlan(cfg, image, native, root, run, "ruby")
	p.Environment["PMTOOL_TEMPLATE_FIXTURE"] = file
	if !native {
		position := len(p.Args) - 3
		p.Args = append(p.Args[:position], append([]string{"--mount", mount(tmp, tmp, true), "--env", "PMTOOL_TEMPLATE_FIXTURE=" + file}, p.Args[position:]...)...)
	}
	if code := a.driver.execute(p); code != 0 {
		return commandFailure{"template tests", code}
	}
	return nil
}
func stringValue(v any) string { s, _ := v.(string); return s }

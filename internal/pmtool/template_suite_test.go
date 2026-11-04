package pmtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateSuiteSelfChecksInGoAndPassesFixtureWithoutToolkitSource(t *testing.T) {
	for _, native := range []bool{false, true} {
		a, d, _ := testApp(t)
		for name, body := range map[string]string{
			"template.yml": "schema: 1\nfiles_root: files\ndefaults: defaults.yml\n",
			"defaults.yml": "{}\n",
			"test/run.rb":  "puts 'template tests'\n",
		} {
			path := filepath.Join(a.cwd, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
		}
		d.rendered = []byte(`["rendered CI\n"]`)
		if err := a.templateSuite(config{Docker: "docker"}, imageID, a.cwd, native); err != nil {
			t.Fatal(err)
		}
		if len(d.plans) != 1 || len(d.queries) != 2 {
			t.Fatal("template test skipped Go baseline self-check", d.plans, len(d.queries))
		}
		p := d.plans[0]
		path := p.Environment["PMTOOL_TEMPLATE_FIXTURE"]
		if path == "" || strings.Contains(strings.Join(p.Args, " "), "TOOLKIT_ROOT") {
			t.Fatal("template test requires launcher source")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("temporary fixture not cleaned", err)
		}
		if _, err := os.Stat(filepath.Join(a.cwd, ".sync.lock.json")); !os.IsNotExist(err) {
			t.Fatal("template self-check modified its source checkout")
		}
		if !native && !strings.Contains(strings.Join(p.Args, " "), "PMTOOL_TEMPLATE_FIXTURE=") {
			t.Fatal("container did not receive fixture path")
		}
	}
}
func TestTemplateSuiteFailureAndDryRunDoNotModifySource(t *testing.T) {
	a, d, out := testApp(t)
	if code := a.run([]string{"--dry-run", "template-test"}); code != 0 || len(d.calls) > 0 || len(d.plans) > 0 || !strings.Contains(out.String(), `"executor":"go"`) {
		t.Fatal("template preview executed helpers")
	}
	expectFailure(t, "use template-test", func() { a.runTemplateTest(config{}, "", []string{"--unknown", "x"}, true, false) })
	if err := a.templateSuite(config{}, "", a.cwd, true); err == nil {
		t.Fatal("missing manifest accepted")
	}
}
func TestNativeWorkerActuallyUnsetsCredentialsAndBundlerInjection(t *testing.T) {
	for _, key := range []string{"GITLAB_TOKEN", "GITHUB_TOKEN", "TOOLING_TEMPLATE_TOKEN", "RUBYOPT", "BUNDLE_GEMFILE"} {
		t.Setenv(key, "MUST_NOT_REACH_RUBY")
	}
	p := plan{Docker: "ruby", Environment: privateValidationEnvironment()}
	for _, entry := range commandForPlan(p).Env {
		if strings.Contains(entry, "MUST_NOT_REACH_RUBY") {
			t.Fatal("worker inherited secret or runtime injection")
		}
	}
}

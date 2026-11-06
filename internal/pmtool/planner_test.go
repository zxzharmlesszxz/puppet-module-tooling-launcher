package pmtool

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/templatekit"
)

func TestGoValidationPlanOrderStagesImagesAndAliases(t *testing.T) {
	a, _, _ := testApp(t)
	for command, count := range map[string]int{"check": 4, "module-build": 1, "voxbox": 2, "audit": 1, "sync": 1, "sync-noop": 1, "migrate": 1, "release-checks": 1} {
		o := a.parseOperation([]string{command})
		steps := a.sequence(o).Steps
		if len(steps) != count {
			t.Fatal("sequence changed", command, len(steps))
		}
		for _, step := range steps {
			if step.Permission.Socket || step.Permission.Token || step.Permission.Author {
				t.Fatal("validation gained privileges", step)
			}
			if strings.Contains(strings.Join(step.Command, " "), "docker.sock") {
				t.Fatal("nested socket")
			}
		}
	}
	steps := a.sequence(a.parseOperation([]string{"check"})).Steps
	var labels []string
	for _, step := range steps {
		labels = append(labels, step.Label)
	}
	if !reflect.DeepEqual(labels, []string{"Module audit", "VoxBox validation", "VoxBox specs", "Release/changelog checks"}) || steps[0].Kind != "audit" {
		t.Fatal(labels)
	}
	if !strings.Contains(strings.Join(steps[1].Command, " "), voxboxImage) || !strings.Contains(strings.Join(steps[2].Command, " "), voxboxImage) {
		t.Fatal("default tool image changed")
	}
	if !reflect.DeepEqual(toolArguments(steps[1].Command), []string{"validate", "lint", "rubocop"}) {
		t.Fatal("validation lost syntax, Puppet lint or Ruby lint")
	}
	for _, tool := range []string{"voxbox"} {
		for _, stage := range []string{"validate", "unit"} {
			o := a.parseOperation([]string{tool, "--stage", stage})
			selected := a.sequence(o).Steps
			if len(selected) != 1 || selected[0].Tool != tool {
				t.Fatal("single stage reruns other phase")
			}
		}
	}
	for alias, target := range map[string]string{"bootstrap-local": "sync", "bootstrap-local-noop": "sync-noop", "audit-local": "audit"} {
		if o := a.parseOperation([]string{alias}); o.Command != target {
			t.Fatal(alias, o)
		}
	}
}
func TestRuntimeOptionsAndGuardsAreNative(t *testing.T) {
	a, _, _ := testApp(t)
	for _, row := range []struct {
		args     []string
		fragment string
	}{
		{[]string{"audit", "--host-module", "/wrong"}, "managed by the launcher"},
		{[]string{"audit", "--module", "/"}, "other than /"},
		{[]string{"audit", "--unknown", "x"}, "unsupported runtime option"},
		{[]string{"audit", "--module"}, "missing value"},
		{[]string{"check", "--stage", "validate"}, "supported only"},
		{[]string{"voxbox", "--stage", "invalid"}, "validate|unit"},
		{[]string{"audit", "--template="}, "must not be empty"},
	} {
		expectFailure(t, row.fragment, func() { a.parseOperation(row.args) })
	}
	expectFailure(t, "unsupported validation", func() { a.sequence(a.parseOperation([]string{"unknown"})) })
	file := filepath.Join(a.cwd, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	expectFailure(t, "directory", func() { a.parseOperation([]string{"audit", "--module", file}) })
}
func TestPhysicalModuleAndCSVWorkerMount(t *testing.T) {
	a, _, _ := testApp(t)
	module := filepath.Join(a.cwd, "clone, with space")
	if err := os.Mkdir(module, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(a.cwd, "link")
	if err := os.Symlink(module, link); err != nil {
		t.Fatal(err)
	}
	o := a.parseOperation([]string{"audit", "--module", link})
	if o.Module != module {
		t.Fatal("module path not physical")
	}
	p := a.checkPlan(config{Docker: "docker"}, imageID, false, module, "spec/support/check.rb", "ruby")
	for i, arg := range p.Args {
		if arg != "--mount" {
			continue
		}
		fields, err := csv.NewReader(strings.NewReader(p.Args[i+1])).Read()
		if err != nil || !reflect.DeepEqual(fields, []string{"type=bind", "src=" + module, "dst=" + module, "readonly"}) {
			t.Fatal("mount not CSV encoded", fields, err)
		}
		return
	}
	t.Fatal("worker mount missing")
}
func TestERBWorkerUsesOnlyJSONNoSourceBundleOrCredentials(t *testing.T) {
	a, d, _ := testApp(t)
	d.rendered = []byte(`["result"]`)
	docs := []templatekit.Document{{Source: "<%= 1 %>", Config: templatekit.Mapping{}, Metadata: templatekit.Mapping{"name": "team-sample"}}}
	for _, native := range []bool{false, true} {
		got, err := a.renderTemplate(config{Docker: "docker"}, imageID, native, docs)
		if err != nil || len(got) != 1 || string(got[0]) != "result" {
			t.Fatal(got, err)
		}
		p := d.queries[len(d.queries)-1]
		argv := strings.Join(p.Args, " ")
		if strings.Contains(argv, "docker.sock") || strings.Contains(argv, "--mount") || !strings.Contains(string(p.Input), `"documents"`) {
			t.Fatal("renderer gained host source access")
		}
		for k, v := range p.Environment {
			if v != "" {
				t.Fatal("worker inherited sensitive environment", k)
			}
		}
		if native {
			if p.Docker != "ruby" {
				t.Fatal("native ERB requires a container")
			}
		} else if !strings.Contains(argv, "--network none") || !strings.Contains(argv, imageID) || !strings.Contains(argv, "--read-only") {
			t.Fatal("SDK worker is not isolated/pinned")
		}
	}
}
func TestTemplateAuditCheckPlansSelectBundleAndClearSecrets(t *testing.T) {
	a, _, _ := testApp(t)
	for _, native := range []bool{false, true} {
		for _, runtime := range []string{"ruby", "voxbox"} {
			p := a.checkPlan(config{Docker: "docker"}, imageID, native, a.cwd, "spec/support/check.rb", runtime)
			argv := strings.Join(p.Args, " ")
			if strings.Contains(argv, "docker.sock") || strings.Contains(argv, "GITLAB_TOKEN") || strings.Contains(argv, "TOOLING_TEMPLATE_TOKEN") {
				t.Fatal("audit check received credentials/socket")
			}
			for _, key := range []string{"GITLAB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN", "TOOLING_TEMPLATE_TOKEN", "FORGE_PUBLISH_TOKEN"} {
				if value, ok := p.Environment[key]; !ok || value != "" {
					t.Fatal("native worker failed to clear credentials", key)
				}
			}
			if !native && (!strings.Contains(argv, "--network none") || !strings.Contains(argv, "readonly") || !strings.Contains(argv, imageID)) {
				t.Fatal("audit worker isolation changed")
			}
			if runtime == "voxbox" && native && (p.Docker != "bundle" || p.Environment["BUNDLE_GEMFILE"] != "/opt/voxbox/Gemfile") {
				t.Fatal("audit check lost selected bundle")
			}
		}
	}
}
func TestDryRunPlanningNeedsNoImageTemplateOrCredentials(t *testing.T) {
	a, d, out := testApp(t)
	if code := a.run([]string{"--dry-run", "check"}); code != 0 || len(d.calls) > 0 || len(d.plans) > 0 || len(d.queries) > 0 {
		t.Fatal("dry plan performed I/O")
	}
	if !strings.Contains(out.String(), `"kind": "audit"`) {
		t.Fatal(out.String())
	}
}
func TestOperationExitPreservesWrappedStatus(t *testing.T) {
	if operationExit(fmt.Errorf("wrapped: %w", testExit{23})) != 23 || operationExit(nil) != 0 {
		t.Fatal("wrapped status lost")
	}
}

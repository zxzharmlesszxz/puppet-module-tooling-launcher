package templatekit

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/internal/scaffold"
	"go.yaml.in/yaml/v3"
)

func TestGeneratedGitHubWorkflowAcceptsEngineCallerSecrets(t *testing.T) {
	template := generatedTemplate(t, "github")
	workflow, err := readFile(template.Root, GitHubCI)
	if err != nil {
		t.Fatal(err)
	}
	var callee, caller Mapping
	if err := yaml.Unmarshal(workflow, &callee); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(githubCISource), &caller); err != nil {
		t.Fatal(err)
	}
	declared := mapping(mapping(mapping(callee["on"])["workflow_call"])["secrets"])
	forwarded := mapping(mapping(mapping(caller["jobs"])["module"])["secrets"])
	if len(forwarded) == 0 {
		t.Fatal("caller fixture has no secrets")
	}
	for name := range forwarded {
		declaration, ok := declared[name]
		if !ok || mapping(declaration)["required"] != false {
			t.Fatalf("starter rejects optional caller secret %s", name)
		}
	}
	jobs := mapping(callee["jobs"])
	for _, raw := range jobs {
		steps := mapping(raw)["steps"].([]any)
		for _, step := range steps {
			environment := mapping(mapping(step)["env"])
			if _, ok := environment["FORGE_PUBLISH_TOKEN"]; ok {
				t.Fatal("validation receives a Forge credential")
			}
		}
	}
}

func generatedTemplate(t *testing.T, provider string) *Template {
	t.Helper()
	files, err := scaffold.Files(scaffold.Options{Name: "team", Provider: provider, Version: "v1.1.0", PDK: "pdk:fixture", VoxBox: "voxbox:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for name, body := range files {
		put(t, root, name, string(body))
	}
	host := "git.example.test"
	if provider == "github" {
		host = "github.com"
	}
	template, err := Load(root, host+"/team/template@v1.0.0", fixtureRevision)
	if err != nil {
		t.Fatal(err)
	}
	return template
}

func TestGeneratedTemplateLoadsBothCIProviders(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		template := generatedTemplate(t, provider)
		host := "git.example.test"
		if provider == "github" {
			host = "github.com"
		}
		target, config, err := template.CIConfig(provider, host, "", "")
		if err != nil || target == "" || config["ref"] != fixtureRevision || len(template.Defaults) != 1 {
			t.Fatalf("generated template invalid: %s %+v %v", target, config, err)
		}
	}
}

func TestRealGeneratedStarterSyncAuditOverridesAndOwnSuite(t *testing.T) {
	render := rubyRenderer(t)
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			template := generatedTemplate(t, provider)
			before := snapshot(t, template.Root)
			root := t.TempDir()
			put(t, root, "metadata.json", `{"name":"team-sample","version":"1.0.0"}`)
			put(t, root, ".sync.yml", ".editorconfig:\n  indent_size: 4\n")
			host := "git.example.test"
			if provider == "github" {
				host = "github.com"
			}
			e := &Engine{Root: root, Template: template, Provider: provider, OriginHost: host, Context: ctx, Out: &bytes.Buffer{}, Render: render, Git: func(...string) (string, error) { return "", nil }}
			syncFixture(t, e)
			if !strings.Contains(read(t, root, ".editorconfig"), "indent_size = 4") {
				t.Fatal("ERB defaults/overrides failed")
			}
			plan, err := e.Plan(false)
			if err != nil || len(plan.Errors) != 0 {
				t.Fatalf("generated baseline audit: %+v %v", plan, err)
			}
			put(t, root, ".editorconfig", "drift")
			plan, err = e.Plan(false)
			if err != nil || len(plan.Errors) == 0 {
				t.Fatal("generated baseline accepted drift", err)
			}
			data, err := Fixture(template)
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "fixture.json")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, "ruby", filepath.Join(template.Root, "test/run.rb"))
			cmd.Env = append(cleanEnvironment(), "PMTOOL_TEMPLATE_FIXTURE="+path)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("starter's own tests: %s %v", out, err)
			}
			if !reflect.DeepEqual(before, snapshot(t, template.Root)) {
				t.Fatal("template test changed its package")
			}
		})
	}
}

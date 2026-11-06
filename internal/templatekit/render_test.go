package templatekit

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func rubyRenderer(t *testing.T) Render {
	t.Helper()
	if os.Getenv("PMTOOL_RUBY_TESTS") != "1" {
		t.Skip("real ERB checks run in the workflow-check SDK target")
	}
	return func(ctx context.Context, documents []Document) ([][]byte, error) {
		data, err := RenderRequest(documents)
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, "ruby", "-e", RendererScript)
		cmd.Stdin = strings.NewReader(string(data))
		cmd.Env = cleanEnvironment()
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		return RenderResponse(out)
	}
}
func TestRendererProtocolAndModuleSideHelperAssets(t *testing.T) {
	data, err := RenderRequest([]Document{{Source: "literal", Config: Mapping{}, Metadata: Mapping{"name": "team-sample"}, GroupOrder: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	var request Mapping
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	if len(request["documents"].([]any)) != 1 {
		t.Fatal("renderer protocol changed")
	}
	values, err := RenderResponse([]byte(`["literal","Українська\n"]`))
	if err != nil || len(values) != 2 || string(values[1]) != "Українська\n" {
		t.Fatal(values, err)
	}
	if _, err := RenderResponse([]byte(`{"invalid":true}`)); err == nil {
		t.Fatal("invalid renderer response accepted")
	}
	source, err := profileSource()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(source, "require_relative 'tooling_group_selection'") || strings.Contains(source, "ModuleBootstrap") || !strings.Contains(source, "ToolingContractProfile") {
		t.Fatal("module-side compatibility helper has workflow dependencies")
	}
	if strings.Contains(source, "\n\n\n") || !strings.HasSuffix(source, "\n") {
		t.Fatal("assembled helper has invalid blank-line boundaries")
	}
	if strings.Count(source, "# frozen_string_literal: true") != 1 {
		t.Fatal("assembled helper must have exactly one leading magic comment")
	}
	if strings.Contains(source, "%w[ci template]") || strings.Contains(source, "match?(/") {
		t.Fatal("helper must not force a module's word-array or regexp literal style")
	}
	if strings.Count(source, "\nclass ") != 1 || !strings.Contains(source, "\n  class ToolingGroupSelection\n") {
		t.Fatal("assembled profile must contain one top-level class with a nested selection helper")
	}
	entries, err := Assets.ReadDir("assets")
	if err != nil || len(entries) != 2 {
		t.Fatal("unexpected Ruby engine embedded", err)
	}
}
func TestRealERBMetadataGroupOrderAndProfileCompatibility(t *testing.T) {
	render := rubyRenderer(t)
	template, _ := fixture(t)
	source := []byte("<%= @metadata.fetch(:module_name) %>|<%= @configs.fetch('namespace') %>|<%= @configs.fetch('contract_groups').keys.join(',') %>|<%= @configs.fetch('contract_groups')['repository'].keys.join(',') %>\n")
	doc, err := template.document(source, Mapping{"namespace": "sample"}, Mapping{"name": "team-sample"})
	if err != nil {
		t.Fatal(err)
	}
	values, err := render(context.Background(), []Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	if string(values[0]) != "sample|sample|repository,package|targets,types,legacy_optional_targets\n" {
		t.Fatal(string(values[0]))
	}
	profile, err := template.document([]byte("<%= @configs.fetch('profile_source') %>\nToolingContractProfile.configure(<%= @configs.fetch('contract_groups').inspect %>, <%= @configs.fetch('contracts').inspect %>, <%= @configs.fetch('allow_contract_overrides').inspect %>)\n"), Mapping{}, Mapping{"name": "team-sample"})
	if err != nil {
		t.Fatal(err)
	}
	values, err = render(context.Background(), []Document{profile})
	if err != nil {
		t.Fatal(err)
	}
	command := string(values[0]) + "\nenabled, overrides = ToolingContractProfile.resolve({'repository' => false}, 'sample')\nabort 'profile drift' unless enabled == ['package'] && overrides.fetch('lib/puppet/functions/sample/key.rb').fetch('unmanaged')\n"
	cmd := exec.Command("ruby", "-e", command)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
}
func TestRealERBCICallersAndSync(t *testing.T) {
	render := rubyRenderer(t)
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			template, e := fixture(t)
			e.Render = render
			if provider == "github" {
				e.Provider = "github"
				e.OriginHost = "github.com"
				template.Reference = "github.com/team/template@v1"
				template.CI = Mapping{"github": ".github/workflows/module.yml"}
			}
			syncFixture(t, e)
			before := snapshot(t, e.Root)
			if err := e.Run(false, false); err != nil {
				t.Fatal(err)
			}
			syncFixture(t, e)
			if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
				t.Fatal("real ERB sync not idempotent")
			}
			if provider == "github" {
				body := read(t, e.Root, GitHubCI)
				if !strings.Contains(body, "uses: team/template/.github/workflows/module.yml@"+fixtureRevision) || !strings.Contains(body, "actions: read") || strings.Contains(body, "pull_request:") {
					t.Fatal(body)
				}
			} else {
				body := read(t, e.Root, ".gitlab-ci.yml")
				if !strings.Contains(body, "ref: '"+fixtureRevision+"'") || !strings.Contains(body, "project: 'team/template'") {
					t.Fatal(body)
				}
			}
		})
	}
}

func TestRealModuleProfileAgreesWithGoSelection(t *testing.T) {
	render := rubyRenderer(t)
	for _, authority := range []bool{true, false} {
		template, _ := fixture(t)
		template.AllowOverrides = authority
		doc, err := template.document([]byte("<%= @configs.fetch('profile_source') %>\nToolingContractProfile.configure(<%= @configs.fetch('contract_groups').inspect %>, <%= @configs.fetch('contracts').inspect %>, <%= @configs.fetch('allow_contract_overrides').inspect %>)\n"), Mapping{}, Mapping{"name": "team-sample"})
		if err != nil {
			t.Fatal(err)
		}
		output, err := render(context.Background(), []Document{doc})
		if err != nil {
			t.Fatal(err)
		}
		script := string(output[0]) + "\nrequire 'json'\ninput=JSON.parse(STDIN.read)\nbegin\n  enabled, overrides=ToolingContractProfile.resolve(input,'sample')\n  STDOUT.write(JSON.generate({'enabled'=>enabled,'overrides'=>overrides}))\nrescue StandardError\n  exit 23\nend\n"
		for _, input := range []Mapping{{}, {"repository": false}, {"package": false}, {"repository": true, "package": true}, {"repository": false, "package": false}, {"types/repo.pp": Mapping{"unmanaged": true}}, {"types/package.pp": Mapping{"unmanaged": true}}, {"repository": "false"}, {"repository": false, "types/repo.pp": Mapping{"unmanaged": true}}, {"types/repo.pp": Mapping{"custom": true}}, {"repositroy": false}, {"template": Mapping{"use": fixtureReference}}, {"ci": Mapping{"provider": "gitlab"}}} {
			selection, goErr := template.Select(input, "sample")
			data, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("ruby", "-e", script)
			cmd.Stdin = strings.NewReader(string(data))
			out, rubyErr := cmd.Output()
			if (goErr == nil) != (rubyErr == nil) {
				t.Fatalf("Go/module profile differs: authority=%v input=%v Go=%v Ruby=%v", authority, input, goErr, rubyErr)
			}
			if goErr != nil {
				continue
			}
			var actual struct {
				Enabled   []string           `json:"enabled"`
				Overrides map[string]Mapping `json:"overrides"`
			}
			if err := json.Unmarshal(out, &actual); err != nil {
				t.Fatal(err)
			}
			if strings.Join(selection.Enabled, ",") != strings.Join(actual.Enabled, ",") || !reflect.DeepEqual(selection.Overrides, actual.Overrides) {
				t.Fatal("expanded profile differs", input, selection, actual)
			}
		}
	}
}
func TestVersionedTeamTemplateUsesSameEngine(t *testing.T) {
	render := rubyRenderer(t)
	root := os.Getenv("PMTOOL_TEST_TEMPLATE")
	if root == "" {
		t.Skip("team-template checkout supplied by template integration job")
	}
	template, err := Load(root, fixtureReference, fixtureRevision)
	if err != nil {
		t.Fatal(err)
	}
	for name := range template.Groups {
		t.Run(name, func(t *testing.T) {
			module := t.TempDir()
			put(t, module, "metadata.json", `{"name":"team-sample","version":"1.2.3"}`)
			put(t, module, "CHANGELOG.md", "## Release 1.2.3\n")
			e := &Engine{Root: module, Template: template, Context: context.Background(), Provider: "gitlab", OriginHost: "git.example.test", Render: render, Out: os.Stdout, Git: func(...string) (string, error) { return "", nil }}
			syncFixture(t, e)
			yamlPut(t, module, ".sync.yml", Mapping{name: false})
			before := snapshot(t, module)
			if err := e.Run(true, true); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, snapshot(t, module)) {
				t.Fatal("team template preview changed files")
			}
			syncFixture(t, e)
			for _, target := range template.Groups[name].Targets {
				path, err := namespacePath(target, "sample")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(module, path)); !os.IsNotExist(err) {
					t.Fatal("disabled team contract remains", path, err)
				}
			}
			yamlPut(t, module, ".sync.yml", Mapping{name: true})
			syncFixture(t, e)
			for _, target := range template.Groups[name].Targets {
				path, err := namespacePath(target, "sample")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(module, path)); err != nil {
					t.Fatal("enabled contract missing", path, err)
				}
			}
			plan, err := e.Plan(false)
			if err != nil || len(plan.Errors) > 0 {
				t.Fatal("team baseline drifted", plan.Errors, err)
			}
		})
	}
}

package templatekit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const fixtureReference = "git.example.test/team/template@v1.2.3"
const fixtureRevision = "0123456789012345678901234567890123456789"

func put(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func yamlPut(t *testing.T, root, name string, data any) {
	t.Helper()
	body, err := yaml.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, name, string(body))
}
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.Type()&os.ModeSymlink != 0 {
			result[rel] = "symlink"
			return nil
		}
		data, err := os.ReadFile(path)
		result[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func requireError(t *testing.T, err error, fragment string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), fragment) {
		t.Fatalf("expected %q, got %v", fragment, err)
	}
}
func fixture(t *testing.T) (*Template, *Engine) {
	t.Helper()
	root := t.TempDir()
	manifest := `schema: 1
files_root: files
defaults: defaults.yml
contracts: [repository, package]
allow_contract_overrides: true
contract_groups:
  repository:
    files: [types/repo.pp, 'lib/puppet/functions/%{namespace}/key.rb']
    types: [repo]
    legacy_optional_files: ['lib/puppet/functions/%{namespace}/key.rb']
  package:
    files: [types/package.pp, spec/type_aliases/package_spec.rb]
    types: [package]
ci:
  gitlab: ci/module.yml
audit:
  metadata:
    semantic_version: true
    forbidden_keys: [template-url, template-ref]
    url_fields: [source, project_page]
    no_self_symlinks: true
    changelog: {file: CHANGELOG.md, heading: 'Release %{version}'}
  tracked_ignored: true
  allow_custom_files: false
  coverage: {file: spec/spec_helper.rb, enabled_key: coverage_report, minimum_key: minimum, minimum: 100}
`
	put(t, root, "template.yml", manifest)
	defaults := Mapping{".gitignore": Mapping{}, "spec/spec_helper.rb": Mapping{"coverage_report": true, "minimum": 100}, "notes.txt": Mapping{}}
	for _, name := range []string{"types/repo.pp", "lib/puppet/functions/%{namespace}/key.rb", "types/package.pp", "spec/type_aliases/package_spec.rb"} {
		defaults[name] = Mapping{}
	}
	yamlPut(t, root, "defaults.yml", defaults)
	for target := range defaults {
		put(t, root, "files/"+target+".erb", target+" @{namespace}\n")
	}
	put(t, root, "ci/module.yml", "stages: [test]\n")
	template, err := Load(root, fixtureReference, fixtureRevision)
	if err != nil {
		t.Fatal(err)
	}
	module := t.TempDir()
	put(t, module, "metadata.json", `{"name":"team-sample","version":"1.2.3"}`)
	put(t, module, "CHANGELOG.md", "## Release 1.2.3\n")
	put(t, module, "domain.txt", "module-owned")
	e := &Engine{Root: module, Template: template, Context: context.Background(), Provider: "gitlab", OriginHost: "git.example.test", Out: &bytes.Buffer{}}
	e.Git = func(...string) (string, error) { return "", nil }
	// Ownership tests inject a deliberately small renderer; ERB has separate real-Ruby tests.
	e.Render = func(_ context.Context, docs []Document) ([][]byte, error) {
		result := make([][]byte, len(docs))
		for i, doc := range docs {
			result[i] = []byte(strings.ReplaceAll(doc.Source, "@{namespace}", str(doc.Config["namespace"])))
		}
		return result, nil
	}
	return template, e
}
func syncFixture(t *testing.T, e *Engine) {
	t.Helper()
	if err := e.Run(true, false); err != nil {
		t.Fatal(err)
	}
}
func TestSyncPreviewConflictsApprovalAndIdempotence(t *testing.T) {
	_, e := fixture(t)
	initial := snapshot(t, e.Root)
	if err := e.Run(true, true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(initial, snapshot(t, e.Root)) {
		t.Fatal("preview wrote files")
	}
	put(t, e.Root, ".gitignore", "local rules")
	before := snapshot(t, e.Root)
	requireError(t, e.Run(true, false), "conflict: .gitignore")
	if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
		t.Fatal("conflict caused partial writes")
	}
	e.Files = ".gitignore"
	syncFixture(t, e)
	installed := snapshot(t, e.Root)
	if installed["domain.txt"] != "module-owned" || !strings.Contains(installed["types/package.pp"], "sample") {
		t.Fatal(installed)
	}
	syncFixture(t, e)
	if !reflect.DeepEqual(installed, snapshot(t, e.Root)) {
		t.Fatal("sync is not idempotent")
	}
	if err := e.Run(false, false); err != nil {
		t.Fatal(err)
	}
	put(t, e.Root, "spec/spec_helper.rb", "changed helper")
	e.Files = "spec/spec_helper.rb"
	before = snapshot(t, e.Root)
	if err := e.Run(true, true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
		t.Fatal("approved preview wrote files")
	}
	put(t, e.Root, ".gitignore", "another conflict")
	before = snapshot(t, e.Root)
	requireError(t, e.Run(true, false), "conflict: .gitignore")
	if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
		t.Fatal("approval bypassed another conflict")
	}
	e.Files += " .gitignore"
	syncFixture(t, e)
	if !reflect.DeepEqual(installed, snapshot(t, e.Root)) {
		t.Fatal("replacement modified unrelated files")
	}
	e.Files = "domain.txt"
	requireError(t, e.Run(true, false), "non-baseline")
}
func TestGroupDisableRemovalReenableAndLegacyProfile(t *testing.T) {
	for _, group := range []string{"repository", "package"} {
		t.Run(group, func(t *testing.T) {
			template, e := fixture(t)
			syncFixture(t, e)
			yamlPut(t, e.Root, ".sync.yml", Mapping{group: false})
			before := snapshot(t, e.Root)
			if err := e.Run(true, true); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
				t.Fatal("removal preview wrote")
			}
			requireError(t, e.Run(false, false), "disabled contract file still exists")
			syncFixture(t, e)
			for _, path := range template.Groups[group].Targets {
				name, err := namespacePath(path, "sample")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(e.Root, name)); !os.IsNotExist(err) {
					t.Fatal("disabled file exists", name, err)
				}
			}
			after := snapshot(t, e.Root)
			syncFixture(t, e)
			if !reflect.DeepEqual(after, snapshot(t, e.Root)) {
				t.Fatal("removal not idempotent")
			}
			yamlPut(t, e.Root, ".sync.yml", Mapping{group: true})
			if err := e.Run(true, true); err != nil {
				t.Fatal(err)
			}
			for _, path := range template.Groups[group].Targets {
				name, err := namespacePath(path, "sample")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(e.Root, name)); !os.IsNotExist(err) {
					t.Fatal("re-enable preview wrote", name)
				}
			}
			syncFixture(t, e)
		})
	}
	_, e := fixture(t)
	yamlPut(t, e.Root, ".sync.yml", Mapping{"types/repo.pp": Mapping{"unmanaged": true}})
	syncFixture(t, e)
	if _, err := os.Stat(filepath.Join(e.Root, "lib/puppet/functions/sample/key.rb")); !os.IsNotExist(err) {
		t.Fatal("legacy disabled group enabled new optional helper")
	}
	put(t, e.Root, "types/repo.pp", "stale contract")
	requireError(t, e.Run(true, false), "disabled contract file still exists")
}
func TestRemovalConflictsRacesAndPathGuards(t *testing.T) {
	for _, scenario := range []string{"modified", "symlink", "parent symlink", "directory", "concurrent"} {
		t.Run(scenario, func(t *testing.T) {
			_, e := fixture(t)
			syncFixture(t, e)
			yamlPut(t, e.Root, ".sync.yml", Mapping{"repository": false})
			path := filepath.Join(e.Root, "types/repo.pp")
			switch scenario {
			case "modified":
				put(t, e.Root, "types/repo.pp", "local change")
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(e.Root, "metadata.json"), path); err != nil {
					t.Fatal(err)
				}
			case "parent symlink":
				if err := os.Rename(filepath.Join(e.Root, "types"), filepath.Join(e.Root, "original-types")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(e.Root, "original-types"), filepath.Join(e.Root, "types")); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "concurrent":
				e.BeforeApply = func() { put(t, e.Root, "types/repo.pp", "concurrent change") }
			}
			before := snapshot(t, e.Root)
			if err := e.Run(true, false); err == nil {
				t.Fatal("unsafe removal accepted")
			}
			if scenario != "concurrent" && !reflect.DeepEqual(before, snapshot(t, e.Root)) {
				t.Fatal("refusal mutated files")
			}
			if scenario == "modified" {
				e.Files = "types/repo.pp"
				if err := e.Run(true, true); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
					t.Fatal("approved preview removed file")
				}
				syncFixture(t, e)
			}
			if scenario == "concurrent" && read(t, e.Root, "lib/puppet/functions/sample/key.rb") == "" {
				t.Fatal("race partially deleted group")
			}
		})
	}
}
func TestInventoryRetirementAndCustomOwnership(t *testing.T) {
	template, e := fixture(t)
	syncFixture(t, e)
	delete(template.Defaults, "notes.txt")
	put(t, e.Root, "notes.txt", "local notes")
	before := snapshot(t, e.Root)
	requireError(t, e.Run(true, false), "locally changed file is preserved")
	if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
		t.Fatal("modified retired file changed")
	}
	e.Files = "notes.txt"
	syncFixture(t, e)
	if _, err := os.Stat(filepath.Join(e.Root, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("retired owned file not removed")
	}
	if read(t, e.Root, "domain.txt") != "module-owned" {
		t.Fatal("unowned file changed")
	}
	template.Defaults["notes.txt"] = Mapping{}
	e.Files = ""
	syncFixture(t, e)
	template.Audit["allow_custom_files"] = true
	yamlPut(t, e.Root, ".sync.yml", Mapping{"notes.txt": Mapping{"unmanaged": true, "custom": true}})
	put(t, e.Root, "notes.txt", "team-custom notes")
	syncFixture(t, e)
	if read(t, e.Root, "notes.txt") != "team-custom notes" {
		t.Fatal("custom file modified")
	}
	var inventory Inventory
	if err := json.Unmarshal([]byte(read(t, e.Root, Lock)), &inventory); err != nil {
		t.Fatal(err)
	}
	if _, ok := inventory.Files["notes.txt"]; ok {
		t.Fatal("custom file still owned")
	}
	delete(template.Defaults, "notes.txt")
	syncFixture(t, e)
	if read(t, e.Root, "notes.txt") != "team-custom notes" {
		t.Fatal("released custom file deleted")
	}
}
func TestAuditPoliciesAndChecksFailClosed(t *testing.T) {
	for _, scenario := range []string{"version", "forbidden", "changelog", "url", "self-fixture", "ignored", "coverage off", "coverage lower", "coverage string", "unmanaged", "custom", "namespace", "CI ref", "drift"} {
		t.Run(scenario, func(t *testing.T) {
			_, e := fixture(t)
			syncFixture(t, e)
			o := Mapping{}
			switch scenario {
			case "version":
				put(t, e.Root, "metadata.json", `{"name":"team-sample","version":"dev"}`)
			case "forbidden":
				put(t, e.Root, "metadata.json", `{"name":"team-sample","version":"1.2.3","template-ref":"v1"}`)
			case "changelog":
				put(t, e.Root, "CHANGELOG.md", "## Release 1.2.30\n")
			case "url":
				put(t, e.Root, "metadata.json", `{"name":"team-sample","version":"1.2.3","source":"https://git@git.example/team/sample"}`)
			case "self-fixture":
				put(t, e.Root, ".fixtures.yml", "fixtures:\n  symlinks:\n    sample: source\n")
			case "ignored":
				e.Git = func(...string) (string, error) { return "Gemfile.lock\nspec/fixtures/manifests/site.pp\n", nil }
			case "coverage off":
				o["spec/spec_helper.rb"] = Mapping{"coverage_report": false}
			case "coverage lower":
				o["spec/spec_helper.rb"] = Mapping{"minimum": 99}
			case "coverage string":
				o["spec/spec_helper.rb"] = Mapping{"minimum": "100"}
			case "unmanaged":
				o["spec/spec_helper.rb"] = Mapping{"unmanaged": true}
			case "custom":
				o["spec/spec_helper.rb"] = Mapping{"custom": true}
			case "namespace":
				o["types/package.pp"] = Mapping{"namespace": "other"}
			case "CI ref":
				o[".gitlab-ci.yml"] = Mapping{"ref": "v9"}
			case "drift":
				put(t, e.Root, "types/package.pp", "local contract")
			}
			if len(o) > 0 {
				yamlPut(t, e.Root, ".sync.yml", o)
			}
			before := snapshot(t, e.Root)
			if err := e.Run(false, false); err == nil {
				t.Fatal("invalid audit passed", scenario)
			}
			if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
				t.Fatal("audit wrote files")
			}
		})
	}
	template, e := fixture(t)
	template.Defaults["spec/support/check.rb"] = Mapping{}
	put(t, template.Root, "files/spec/support/check.rb.erb", "raise 'policy failure'\n")
	template.Audit["checks"] = []any{Mapping{"name": "team-specific check", "path": "spec/support/check.rb", "runtime": "voxbox"}}
	syncFixture(t, e)
	called := 0
	e.Check = func(_ context.Context, root, path, runtime string) error {
		called++
		if root != e.Root || path != "spec/support/check.rb" || runtime != "voxbox" {
			t.Fatal("wrong check plan")
		}
		return errors.New("policy failure")
	}
	requireError(t, e.Run(false, false), "team-specific check: policy failure")
	put(t, e.Root, "spec/support/check.rb", "unverified code")
	requireError(t, e.Run(false, false), "drift")
	if called != 1 {
		t.Fatal("executed an unverified audit script")
	}
}
func TestInventoryValidationAndAtomicReplacement(t *testing.T) {
	for _, body := range []string{`{`, `{}`, `null`, `{"schema":2,"files":{}}`, `{"schema":1,"files":{"../escape":"x"}}`, `{"schema":1,"files":{".sync.yml":"` + strings.Repeat("a", 64) + `"}}`, `{"schema":1,"files":{"notes.txt":"bad"}}`} {
		_, e := fixture(t)
		put(t, e.Root, Lock, body)
		if err := e.Run(true, false); err == nil {
			t.Fatal("invalid inventory accepted", body)
		}
	}
	_, e := fixture(t)
	syncFixture(t, e)
	path := filepath.Join(e.Root, "notes.txt")
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	put(t, e.Root, "notes.txt", "changed")
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	e.Files = "notes.txt"
	syncFixture(t, e)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatal("replacement lost permissions", err)
	}
	matches, err := filepath.Glob(filepath.Join(e.Root, ".sync-*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatal("staging leak", matches, err)
	}
	c := Change{Path: "new.txt", Content: []byte("new")}
	put(t, e.Root, "new.txt", "concurrent")
	requireError(t, e.apply(c), "appeared")
	if read(t, e.Root, "new.txt") != "concurrent" {
		t.Fatal("exclusive create replaced a file")
	}
}
func TestMigratePreservesDomainFixturesAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	put(t, root, "metadata.json", `{"name":"team-sample","version":"1.2.3","template-url":"url","template-ref":"v1"}`)
	put(t, root, ".fixtures.yml", "fixtures:\n  symlinks:\n    sample: source\n  forge_modules:\n    stdlib: puppetlabs/stdlib\n")
	put(t, root, ".sync.yml", "template: {use: git.example.test/team/template@v1}\n")
	if err := Migrate(root); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, root, "metadata.json"), "template-") || strings.Contains(read(t, root, ".fixtures.yml"), "symlinks") || !strings.Contains(read(t, root, ".fixtures.yml"), "forge_modules") {
		t.Fatal("migration changed wrong fields")
	}
	before := snapshot(t, root)
	if err := Migrate(root); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("migration not idempotent")
	}
	if err := os.Remove(filepath.Join(root, "metadata.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, ".sync.yml"), filepath.Join(root, "metadata.json")); err != nil {
		t.Fatal(err)
	}
	requireError(t, Migrate(root), "symlink")
}
func TestRendererFailureDoesNotMutateModule(t *testing.T) {
	_, e := fixture(t)
	before := snapshot(t, e.Root)
	for _, renderer := range []Render{nil, func(context.Context, []Document) ([][]byte, error) { return nil, fmt.Errorf("ERB failure") }, func(context.Context, []Document) ([][]byte, error) { return nil, nil }} {
		e.Render = renderer
		if err := e.Run(true, false); err == nil {
			t.Fatal("bad renderer passed")
		}
		if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
			t.Fatal("render failure wrote files")
		}
	}
}

func TestMigrationReplacementPreservesModeAndConcurrentChanges(t *testing.T) {
	for _, name := range []string{"metadata.json", ".fixtures.yml"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, name)
			put(t, root, name, "original")
			if err := os.Chmod(path, 0640); err != nil {
				t.Fatal(err)
			}
			if err := replaceMigratedFile(root, name, []byte("original"), []byte("updated")); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0640 || read(t, root, name) != "updated" {
				t.Fatal("migration changed permissions or content", err)
			}
			put(t, root, name, "concurrent change")
			requireError(t, replaceMigratedFile(root, name, []byte("updated"), []byte("stale replacement")), "changed during sync")
			if read(t, root, name) != "concurrent change" {
				t.Fatal("migration replaced a concurrent change")
			}
		})
	}
}

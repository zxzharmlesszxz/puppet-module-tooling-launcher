package templatekit

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTemplateGroupsAndSelectionAreTemplateOwned(t *testing.T) {
	template, _ := fixture(t)
	for _, overrides := range []Mapping{{}, {"repository": true, "package": true}, {"repository": false}, {"repository": false, "package": false}} {
		s, err := template.Select(overrides, "another_team")
		if err != nil {
			t.Fatal(err)
		}
		for name, g := range template.Groups {
			want := overrides[name] != false
			if contains(s.Enabled, name) != want {
				t.Fatal("group selection changed", name, s.Enabled)
			}
			for _, target := range g.Targets {
				path, err := namespacePath(target, "another_team")
				if err != nil {
					t.Fatal(err)
				}
				if truth(s.Overrides[path]["unmanaged"]) == want {
					t.Fatal("group expansion inconsistent", path)
				}
			}
		}
	}
	for _, invalid := range []Mapping{
		{"repositroy": false}, {"repository": "false"}, {"repository": nil}, {"types/repo.pp": "bad"},
		{"types/repo.pp": Mapping{"unmanaged": "false"}},
		{"repository": false, "types/repo.pp": Mapping{"unmanaged": true}},
		{"repository": false, "types/repo.pp": Mapping{"custom": true}},
		{"types/package.pp": Mapping{"unmanaged": true}},
		{"lib/puppet/functions/%{namespace}/key.rb": Mapping{}, "lib/puppet/functions/sample/key.rb": Mapping{}},
	} {
		if _, err := template.Select(invalid, "sample"); err == nil {
			t.Fatal("invalid selection accepted", invalid)
		}
	}
	if _, err := template.Select(Mapping{}, "../escape"); err == nil {
		t.Fatal("invalid namespace accepted")
	}
	template.AllowOverrides = false
	for _, invalid := range []Mapping{{"repository": false}, {"repository": true}, {"types/repo.pp": Mapping{"unmanaged": true}}} {
		if _, err := template.Select(invalid, "sample"); err == nil {
			t.Fatal("module overrode template-owned contracts", invalid)
		}
	}
	template.Contracts = []string{"package"}
	s, err := template.Select(Mapping{}, "sample")
	if err != nil || !reflect.DeepEqual(s.Enabled, []string{"package"}) {
		t.Fatal("alternative template profile ignored", s, err)
	}
}
func TestTemplateSchemaAndPolicyRejectUnsafeDeclarations(t *testing.T) {
	for _, scenario := range []string{"schema", "unknown", "reserved", "group name", "overlap", "contract missing", "contracts unknown", "contracts duplicate", "override flag", "audit unknown", "metadata boolean", "metadata array", "coverage", "runtime", "check path", "CI path", "source symlink", "source missing"} {
		t.Run(scenario, func(t *testing.T) {
			template, _ := fixture(t)
			root := template.Root
			m, err := readYAML(filepath.Join(root, "template.yml"), false)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "schema":
				m["schema"] = 2
			case "unknown":
				m["unexpected"] = true
			case "reserved":
				defaults, err := readYAML(filepath.Join(root, "defaults.yml"), false)
				if err != nil {
					t.Fatal(err)
				}
				defaults[".sync.yml"] = Mapping{}
				yamlPut(t, root, "defaults.yml", defaults)
			case "group name":
				mapping(m["contract_groups"])["BadName"] = Mapping{"files": []string{"notes.txt"}}
			case "overlap":
				mapping(mapping(m["contract_groups"])["package"])["files"] = []string{"types/repo.pp"}
			case "contract missing":
				mapping(mapping(m["contract_groups"])["package"])["files"] = []string{"missing.pp"}
			case "contracts unknown":
				m["contracts"] = []string{"missing"}
			case "contracts duplicate":
				m["contracts"] = []string{"package", "package"}
			case "override flag":
				m["allow_contract_overrides"] = "true"
			case "audit unknown":
				mapping(m["audit"])["anything"] = true
			case "metadata boolean":
				mapping(mapping(m["audit"])["metadata"])["semantic_version"] = "true"
			case "metadata array":
				mapping(mapping(m["audit"])["metadata"])["url_fields"] = []any{1}
			case "coverage":
				mapping(mapping(m["audit"])["coverage"])["minimum"] = 101
			case "runtime":
				mapping(m["audit"])["checks"] = []any{Mapping{"name": "unsafe", "path": "notes.txt", "runtime": "sh"}}
			case "check path":
				mapping(m["audit"])["checks"] = []any{Mapping{"name": "unsafe", "path": "domain.rb", "runtime": "ruby"}}
			case "CI path":
				mapping(m["ci"])["gitlab"] = "../module.yml"
			case "source symlink":
				file := filepath.Join(root, "files/notes.txt.erb")
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "defaults.yml"), file); err != nil {
					t.Fatal(err)
				}
			case "source missing":
				if err := os.Remove(filepath.Join(root, "files/notes.txt.erb")); err != nil {
					t.Fatal(err)
				}
			}
			yamlPut(t, root, "template.yml", m)
			if _, err := Load(root, fixtureReference, fixtureRevision); err == nil {
				t.Fatal("invalid template accepted", scenario)
			}
		})
	}
}
func TestPathAndLocalRevisionGuards(t *testing.T) {
	for _, name := range []string{"", "/root", "a/../b", "a//b", "./b", "a/.git/config", "a\\b", "a\x00b", "a\nb"} {
		if err := Path(name); err == nil {
			t.Fatal("unsafe path", name)
		}
	}
	template, _ := fixture(t)
	first, err := Load(template.Root, fixtureReference, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load(template.Root, fixtureReference, "")
	if err != nil || first.Revision != second.Revision {
		t.Fatal("local revision is unstable", err)
	}
	put(t, template.Root, ".git/ignored", "ignored history")
	third, err := Load(template.Root, fixtureReference, "")
	if err != nil || third.Revision != first.Revision {
		t.Fatal("history entered local revision", err)
	}
	put(t, template.Root, "ci/module.yml", "changed CI")
	fourth, err := Load(template.Root, fixtureReference, "")
	if err != nil || fourth.Revision == first.Revision || len(fourth.Revision) != 64 {
		t.Fatal("local revision ignored content", err)
	}
	if err := os.Symlink(filepath.Join(template.Root, "template.yml"), filepath.Join(template.Root, "unsafe-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(template.Root, fixtureReference, ""); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatal("local template followed a link", err)
	}
}

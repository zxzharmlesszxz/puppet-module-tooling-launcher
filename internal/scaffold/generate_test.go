package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestGitHubStarterPersistsExecutablePathAcrossSteps(t *testing.T) {
	files, err := Files(starterOptions())
	if err != nil {
		t.Fatal(err)
	}
	var workflow map[string]any
	if err := yaml.Unmarshal(files[".github/workflows/template-test.yml"], &workflow); err != nil {
		t.Fatal(err)
	}
	steps := workflow["jobs"].(map[string]any)["template-test"].(map[string]any)["steps"].([]any)
	run := steps[1].(map[string]any)["run"].(string)
	_, statement, found := strings.Cut(run, "pmtool version\n")
	if !found {
		t.Fatal("missing path publication after verified install")
	}
	path := filepath.Join(t.TempDir(), "github-path")
	cmd := exec.Command("sh", "-eu", "-c", statement)
	cmd.Env = append(os.Environ(), "GITHUB_PATH="+path, "pmtool_dir=/tmp/fixture directory")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("path publication: %s %v", output, err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "/tmp/fixture directory\n" {
		t.Fatalf("invalid GITHUB_PATH entry: %q %v", body, err)
	}
}

func starterOptions() Options {
	return Options{Name: "team_template", Provider: "both", Version: "v2.0.0", VoxBox: "voxbox:fixture"}
}

func TestStarterProvidersSchemasAndVerifiedCI(t *testing.T) {
	for _, provider := range []string{"gitlab", "github", "both"} {
		t.Run(provider, func(t *testing.T) {
			o := starterOptions()
			o.Provider = provider
			files, err := Files(o)
			if err != nil {
				t.Fatal(err)
			}
			if len(files["moduleroot/.editorconfig.erb"]) == 0 || len(files["test/run.rb"]) == 0 || len(files["docs/SCHEMA.md"]) == 0 {
				t.Fatal("starter assets missing")
			}
			var manifest map[string]any
			if err := yaml.Unmarshal(files["template.yml"], &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest["schema"] != 1 || manifest["name"] != o.Name {
				t.Fatal("invalid manifest", manifest)
			}
			ci := manifest["ci"].(map[string]any)
			if _, ok := ci["gitlab"]; ok != (provider != "github") {
				t.Fatal("wrong GitLab CI selection")
			}
			if _, ok := ci["github"]; ok != (provider != "gitlab") {
				t.Fatal("wrong GitHub CI selection")
			}
			for path, body := range files {
				if strings.Contains(string(body), "PMTOOL_TEMPLATE_NAME") || strings.Contains(string(body), "syneforge") || strings.Contains(string(body), "/Users/") {
					t.Fatalf("nonneutral starter: %s", path)
				}
				if !strings.Contains(path, ".yml") || path == "template.yml" || path == "config_defaults.yml" {
					continue
				}
				var doc map[string]any
				if err := yaml.Unmarshal(body, &doc); err != nil {
					t.Fatalf("%s: %v", path, err)
				}
				if !strings.Contains(string(body), "Digest::SHA256") || !strings.Contains(string(body), "v2.0.0") || strings.Contains(string(body), "curl ") || strings.Contains(string(body), "docker.sock") || strings.Contains(string(body), "forge-publish") || strings.Contains(string(body), "puppet/pdk") || strings.Contains(string(body), "pdk --stage") {
					t.Fatalf("unsafe/incomplete CI: %s", path)
				}
				if strings.HasPrefix(path, ".github/") && doc["on"] == nil {
					t.Fatal("GitHub event mapping missing")
				}
			}
			if _, exists := files["VERSION"]; exists {
				t.Fatal("unexpected VERSION file")
			}
		})
	}
}

func TestGenerationNeverOverwritesOrTraverses(t *testing.T) {
	base := t.TempDir()
	physical, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(physical, "template")
	files, err := Files(starterOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(destination, files); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(destination, "template.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(destination, files); err == nil {
		t.Fatal("existing output accepted")
	}
	after, err := os.ReadFile(filepath.Join(destination, "template.yml"))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("existing output changed")
	}
	link := filepath.Join(physical, "link")
	if err := os.Symlink(destination, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(link, files); err == nil {
		t.Fatal("output symlink followed")
	}
	if _, err := Generate(filepath.Join(link, "child"), files); err == nil {
		t.Fatal("parent symlink followed")
	}
	for _, path := range []string{"../escape", "/absolute", "a/../b", ".git/config", "a\\b"} {
		output := filepath.Join(physical, "unsafe")
		if _, err := Generate(output, map[string][]byte{path: []byte("fixture")}); err == nil {
			t.Fatalf("unsafe path %s", path)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("validation wrote output")
		}
	}
}

func TestStarterArguments(t *testing.T) {
	for _, change := range []func(*Options){func(o *Options) { o.Name = "../bad" }, func(o *Options) { o.Name = "Upper" }, func(o *Options) { o.Provider = "other" }, func(o *Options) { o.Version = "dev" }, func(o *Options) { o.Version = "v1.01.0" }} {
		o := starterOptions()
		change(&o)
		if _, err := Files(o); err == nil {
			t.Fatalf("accepted %+v", o)
		}
	}
}

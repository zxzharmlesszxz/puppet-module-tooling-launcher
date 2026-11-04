package pmtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsAreReadLocallyWithCLIThenModuleThenOriginPriority(t *testing.T) {
	a, d, _ := testApp(t)
	path := filepath.Join(a.cwd, ".sync.yml")
	data := []byte("ci:\n  use: git.example.test/team/pipeline//ci/module.yml@v1\n  provider: github\ntemplate:\n  use: git.example.test/team/template@v2\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s := a.resolveWorkflowSettings(a.cwd, workflowSettings{})
	if s.Provider != "github" || s.Template != "git.example.test/team/template@v2" || s.CI != "git.example.test/team/pipeline//ci/module.yml@v1" {
		t.Fatal(s)
	}
	cli := workflowSettings{Provider: "gitlab", Template: "git.example.test/other/template@v3", CI: "git.example.test/other/pipeline//ci/entry.yml@v4"}
	if got := a.resolveWorkflowSettings(a.cwd, cli); got != cli {
		t.Fatal("CLI priority lost", got)
	}
	if len(d.calls) > 0 || len(d.queries) > 0 {
		t.Fatal("explicit settings required a container or origin query")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatal("settings read rewrote YAML", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	d.origin = "ssh://git@github.com:22/team/puppet-sample.git"
	s = a.resolveWorkflowSettings(a.cwd, workflowSettings{})
	if s.Provider != "github" || s.CI != "" || s.Template != "" || len(d.queries) > 0 {
		t.Fatal("origin fallback or template CI fallback changed", s)
	}
}
func TestSettingsRemainIsolatedBetweenModuleClones(t *testing.T) {
	a, d, _ := testApp(t)
	for _, team := range []string{"first", "second"} {
		root := filepath.Join(a.cwd, team)
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".sync.yml"), []byte("ci: {provider: gitlab}\ntemplate: {use: 'git.example.test/"+team+"/template@v1'}\n"), 0600); err != nil {
			t.Fatal(err)
		}
		s := a.resolveWorkflowSettings(root, workflowSettings{})
		if !strings.Contains(s.Template, "/"+team+"/") {
			t.Fatal("settings leaked between teams", s)
		}
	}
	if len(d.calls) > 0 || len(d.queries) > 0 {
		t.Fatal("settings launched a helper")
	}
}
func TestInvalidSettingsStopBeforeRuntime(t *testing.T) {
	for _, body := range []string{"ci: {provider: other}", "ci: []", "template: {use: 12}", "ci: {token: secret}", "ci: {use: \"bad\\nvalue\"}"} {
		a, d, _ := testApp(t)
		if err := os.WriteFile(filepath.Join(a.cwd, ".sync.yml"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if _, ok := asFailure(recover()); !ok {
					t.Fatal("invalid settings accepted", body)
				}
			}()
			a.resolveWorkflowSettings(a.cwd, workflowSettings{})
		}()
		if len(d.plans) > 0 || len(d.queries) > 0 {
			t.Fatal("invalid settings reached runtime")
		}
	}
}
func TestHostConfigContainsOnlyLauncherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"image":"tooling:local","docker":"docker","socket":"/var/run/docker.sock"}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if loadConfig(path).Image != "tooling:local" {
		t.Fatal("launcher config stopped loading")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatal("loading config rewrote file")
	}
}

func TestSettingsCommandNeedsNoSDKOrHostConfig(t *testing.T) {
	a, d, out := testApp(t)
	if err := os.WriteFile(filepath.Join(a.cwd, ".sync.yml"), []byte("ci: {provider: gitlab}\ntemplate: {use: git.example.test/team/template@v1}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := a.run([]string{"settings"}); code != 0 || len(d.calls) > 0 || len(d.plans) > 0 || !strings.Contains(out.String(), "git.example.test/team/template@v1") {
		t.Fatal("settings command launched a helper", out.String())
	}
	if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
		t.Fatal("settings created host image configuration")
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleSettingsResolutionIsReadOnlyAndUsesPinnedImage(t *testing.T) {
	a, d, _ := testApp(t)
	d.moduleSettings = map[string]workflowSettings{a.cwd: {CI: "module-ci", Provider: "github"}}
	a.getenv = func(key string) string {
		if key == "GITHUB_TOKEN" {
			return "secret-token"
		}
		return ""
	}
	p := a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"pipeline-jobs", "--ci", "cli-ci"})
	query := strings.Join(d.calls[0], " ")
	if !strings.Contains(query, "--network none") || !strings.Contains(query, "readonly") || !strings.Contains(query, imageID+" settings") {
		t.Fatal("module settings query is not isolated or pinned")
	}
	for _, forbidden := range []string{"TOKEN", "docker.sock", "secret-token", "config.json"} {
		if strings.Contains(query, forbidden) {
			t.Fatal("settings query received credentials or host configuration")
		}
	}
	argv := strings.Join(p.Args, " ")
	if !strings.Contains(argv, "--ci cli-ci") || !strings.Contains(argv, "--provider github") || !strings.Contains(argv, "--env GITHUB_TOKEN") || strings.Contains(argv, "GITLAB_TOKEN") {
		t.Fatal("CLI/module settings did not determine the effective runtime and token")
	}
	if strings.Count(argv, "--ci ") != 1 || strings.Count(argv, "--provider ") != 1 {
		t.Fatal("effective flags duplicated")
	}
}

func TestModuleSettingsUseExplicitCloneAndDoNotLeakBetweenTeams(t *testing.T) {
	a, d, _ := testApp(t)
	other := filepath.Join(a.cwd, "other clone")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	d.moduleSettings = map[string]workflowSettings{a.cwd: {CI: "team_a-ci", Provider: "gitlab"}, other: {CI: "team_b-ci", Provider: "github"}}
	for _, test := range []struct{ path, ci string }{{other, "team_b-ci"}, {a.cwd, "team_a-ci"}} {
		p := a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"audit", "--module", test.path})
		if !strings.Contains(strings.Join(p.Args, " "), "--ci "+test.ci) {
			t.Fatal("settings came from another clone/team")
		}
	}
}

func TestSettingsQueryPermissionsAndValidation(t *testing.T) {
	for _, permission := range []profile{{}, {Module: true}, {Module: true, ReadOnly: true, Token: true}, {Module: true, ReadOnly: true, Socket: true}} {
		a, d, _ := testApp(t)
		m := capabilities()
		m.Commands["settings"] = permission
		expectFailure(t, "read-only module settings", func() { a.makePlan(config{Docker: "docker"}, imageID, m, []string{"audit"}) })
		if len(d.calls) != 0 {
			t.Fatal("unsafe settings command executed")
		}
	}
	for _, invalid := range []struct{ data, message string }{{`{"ci":"x","provider":"other"}`, "provider"}, {`{"ci":"x","provider":"github","token":"secret"}`, "unknown field"}} {
		a, d, _ := testApp(t)
		d.settingsJSON = invalid.data
		expectFailure(t, invalid.message, func() { a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"audit"}) })
	}
}

func TestHostConfigContainsOnlyLauncherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"image":"tooling:local","docker":"docker","socket":"/var/run/docker.sock"}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if loadConfig(path).Image != "tooling:local" {
		t.Fatal("minimal launcher config stopped loading")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatal("loading config rewrote the file")
	}
}

func TestMetadataCommandsDoNotResolveModuleSettings(t *testing.T) {
	a, d, _ := testApp(t)
	a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"docs", "ci"})
	if len(d.calls) != 0 {
		t.Fatal("metadata command queried module YAML")
	}
}

func TestTemplateSelectionUsesModuleAndCLI(t *testing.T) {
	a, d, _ := testApp(t)
	d.moduleSettings = map[string]workflowSettings{a.cwd: {Provider: "gitlab", Template: "github.com/team/template@v1"}}
	p := a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"audit"})
	if !strings.Contains(strings.Join(p.Args, " "), "--template github.com/team/template@v1") {
		t.Fatal("module template did not reach the runtime")
	}
	p = a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"audit", "--template", "github.com/team/template@v2"})
	if !strings.Contains(strings.Join(p.Args, " "), "--template github.com/team/template@v2") {
		t.Fatal("CLI template did not override module selection")
	}
}

func TestTemplateCIFallbackStaysInRuntime(t *testing.T) {
	a, d, _ := testApp(t)
	d.moduleSettings = map[string]workflowSettings{a.cwd: {Provider: "github", Template: "github.com/team/template@v1"}}
	p := a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"audit"})
	argv := strings.Join(p.Args, " ")
	if strings.Contains(argv, "--ci ") || !strings.Contains(argv, "--template github.com/team/template@v1") {
		t.Fatal("launcher must leave template CI resolution to the runtime")
	}
	if !strings.Contains(strings.Join(d.calls[0], " "), "--network none") {
		t.Fatal("settings query must remain offline")
	}
}

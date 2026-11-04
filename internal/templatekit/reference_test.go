package templatekit

import (
	"reflect"
	"strings"
	"testing"
)

func TestImmutableTemplateReferences(t *testing.T) {
	for _, value := range []string{fixtureReference, "github.com/team/template.git//profiles/base@v2", "git.example.test/team/template@" + fixtureRevision} {
		r, err := ParseReference(value)
		if err != nil || r.Repository == "" || r.Version == "" {
			t.Fatal(value, r, err)
		}
	}
	for _, value := range []string{"", "https://git.example.test/team/template@v1", "git@git.example.test:team/template@v1", "git.example.test/team/template@main", "git.example.test/team/template@HEAD", "git.example.test/team/template@v1;touch", "git.example.test/team/template//../escape@v1", "git.example.test/team/../template@v1", "git.example.test/team/template?token=secret@v1"} {
		if _, err := ParseReference(value); err == nil {
			t.Fatal("mutable/unsafe template accepted", value)
		}
	}
}
func TestCIReferenceAndTemplateRevisionDefaults(t *testing.T) {
	ci, err := ParseCI("git.example.test/team/platform/pipelines//nested/module.yaml@v1.2.3", "gitlab", "git.example.test")
	if err != nil || ci.Repository != "team/platform/pipelines" || ci.File != "nested/module.yaml" {
		t.Fatal(ci, err)
	}
	gh, err := ParseCI("github.com/team/pipelines/.github/workflows/module.yml@v2", "github", "github.com")
	if err != nil || gh.Repository != "team/pipelines" || gh.File != ".github/workflows/module.yml" {
		t.Fatal(gh, err)
	}
	template, _ := fixture(t)
	target, config, err := template.CIConfig("gitlab", "git.example.test", "", "")
	if err != nil || target != ".gitlab-ci.yml" || config["ref"] != fixtureRevision || config["project"] != "team/template" {
		t.Fatal(target, config, err)
	}
	template.Reference = "git.example.test/team/template//base@v1"
	_, config, err = template.CIConfig("gitlab", "git.example.test", "", "")
	if err != nil || config["file"] != "/base/ci/module.yml" {
		t.Fatal(config, err)
	}
	_, config, err = template.CIConfig("gitlab", "git.example.test", "git.example.test/other/pipeline//ci/entry.yml@v3", "")
	if err != nil || config["ref"] != "v3" || config["project"] != "other/pipeline" {
		t.Fatal("explicit CI lost priority", config, err)
	}
	template.Reference = "github.com/team/template@v1"
	_, config, err = template.CIConfig("gitlab", "git.example.test", "", "")
	if err != nil || config["remote"] != "https://raw.githubusercontent.com/team/template/"+fixtureRevision+"/ci/module.yml" {
		t.Fatal("cross-host GitLab include invalid", config, err)
	}
	template.CI = Mapping{"github": ".github/workflows/module.yml"}
	target, config, err = template.CIConfig("github", "github.com", "", "")
	if err != nil || target != GitHubCI || config["ref"] != fixtureRevision {
		t.Fatal(target, config, err)
	}
	if _, _, err := template.CIConfig("gitlab", "git.example.test", "", ""); err == nil {
		t.Fatal("missing provider CI silently accepted")
	}
	if _, _, err := template.CIConfig("gitlab", "git.example.test", "git.example.test/team/pipelines//module.yml@v1", ""); err != nil {
		t.Fatal("explicit CI incorrectly requires template provider CI", err)
	}
	if _, _, err := template.CIConfig("gitlab", "git.example.test", "other.example.test/team/pipelines//ci/module.yml@v1", ""); err == nil {
		t.Fatal("cross-server project include accepted")
	}
}
func TestCIRefusalsAndLegacyProjectCompatibility(t *testing.T) {
	for _, row := range []struct{ value, provider string }{
		{"github.com/team/pipelines/.github/workflows/sub/module.yml@v1", "github"},
		{"git.example.test/team/pipelines//../module.yml@v1", "gitlab"},
		{"git.example.test/team/pipelines//module.yml@main", "gitlab"},
		{"git.example.test/team/pipelines//module.yml@HEAD", "gitlab"},
		{"https://git.example.test/team/pipelines//module.yml@v1", "gitlab"},
		{"git.example.test/team/pipelines//module.txt@v1", "gitlab"},
		{"git.example.test/team/pipelines//module.yml@v1", "unknown"},
	} {
		if _, err := ParseCI(row.value, row.provider, "git.example.test"); err == nil {
			t.Fatal("invalid CI accepted", row)
		}
	}
	template, _ := fixture(t)
	if _, _, err := template.CIConfig("github", "github.com", "", "team/pipelines"); err == nil {
		t.Fatal("legacy GitLab project produced a GitHub caller in a GitLab file")
	}
	_, config, err := template.CIConfig("gitlab", "git.example.test", "", "team/pipelines")
	if err != nil || !reflect.DeepEqual(config, Mapping{"unmanaged": false, "project": "team/pipelines", "file": "/ci/puppet-module.yml", "ref": "v0.6.0"}) {
		t.Fatal(config, err)
	}
	for _, legacy := range []string{"/team/pipelines", "../pipelines", "team/../pipelines", "team/pipelines;touch", "team/pipelines\nsecret"} {
		if _, _, err := template.CIConfig("gitlab", "git.example.test", "", legacy); err == nil {
			t.Fatal("unsafe legacy project", legacy)
		}
	}
	ci, err := ParseCI("team/pipelines/.github/workflows/module.yml@"+strings.Repeat("a", 64), "github", "github.com")
	if err != nil || len(ci.Ref) != 64 {
		t.Fatal("full SHA rejected", err)
	}
}

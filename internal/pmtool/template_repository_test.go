package pmtool

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type templateInitGitDriver struct {
	fakeDriver
	failAt int
}

func (d *templateInitGitDriver) capturePlan(p plan) ([]byte, error) {
	d.queries = append(d.queries, p)
	if p.Docker != "git" || len(p.Args) == 0 {
		return nil, errors.New("unexpected external template tool")
	}
	allowed := p.Args[0] == "--version" || p.Args[0] == "init" || len(p.Args) > 1 && p.Args[0] == "remote" && p.Args[1] == "add"
	if !allowed {
		return nil, errors.New("unexpected Git operation; no network or commit is allowed")
	}
	if len(d.queries) == d.failAt {
		return nil, errors.New("fixture Git unavailable")
	}
	return (realDriver{}).capturePlan(p)
}

func templateOriginArgs(output, origin string) []string {
	return []string{"template-init", "--name", "team", "--output", output, "--origin", origin, "--launcher-version", "v2.0.0"}
}

func TestTemplateOriginValidatesWithoutLeakingCredentials(t *testing.T) {
	for _, origin := range []string{
		"https://github.com/team/template.git",
		"http://git.example.test/team/nested/template.git",
		"ssh://git@git.example.test:777/team/template.git",
		"git@github.com:team/template.git",
	} {
		if err := validateTemplateOrigin(origin); err != nil {
			t.Fatalf("valid template origin refused: %v", err)
		}
	}
	for _, origin := range []string{
		"", "../repository", "/tmp/repository", "file:///tmp/repository", "ext::command",
		"https://fixture-secret@git.example.test/team/template.git",
		"https://user:fixture-secret@git.example.test/team/template.git",
		"ssh://git:fixture-secret@git.example.test/team/template.git",
		"git:fixture-secret@git.example.test:team/template.git",
		"https://git.example.test/team/template.git?fixture-secret",
		"https://git.example.test/team/template.git?",
		"https://git.example.test/team/template.git#fixture-secret",
		"https://git.example.test/team/../template.git",
		"https://git.example.test/team/%2e%2e/template.git",
		"https://git.example.test/team//template.git",
		"https://git.example.test/team/.git/template.git",
		"https://git.example.test/template.git",
		"https://git.example.test/team/template.git\nother",
		"https://git.example.test/team/a b.git",
		"https://git.example\u00a0.test/team/template.git",
		"https://git.example.test/team/tem\u2028plate.git",
		"--upload-pack=command",
	} {
		a, driver, _ := testApp(t)
		expectFailure(t, "origin must be", func() { a.run(templateOriginArgs("new-template", origin)) })
		if err := validateTemplateOrigin(origin); err == nil || strings.Contains(err.Error(), "fixture-secret") {
			t.Fatal("invalid origin accepted or credentials disclosed")
		}
		if _, err := os.Lstat(filepath.Join(a.cwd, "new-template")); !os.IsNotExist(err) {
			t.Fatal("invalid origin created output")
		}
		if len(driver.calls)+len(driver.plans)+len(driver.queries) != 0 {
			t.Fatal("invalid origin contacted external tools")
		}
	}
}

func TestTemplateOriginPreviewNeedsNoGitAndShowsPlannedRemote(t *testing.T) {
	a, driver, out := testApp(t)
	origin := "https://git.example.test/team/template.git"
	args := append([]string{"--executor", "native", "--dry-run"}, templateOriginArgs("preview", origin)...)
	if code := a.run(args); code != 0 {
		t.Fatal("preview failed")
	}
	var preview struct{ Origin string }
	if err := json.Unmarshal(out.Bytes(), &preview); err != nil || preview.Origin != origin {
		t.Fatal("preview lost the planned origin", err)
	}
	if _, err := os.Lstat(filepath.Join(a.cwd, "preview")); !os.IsNotExist(err) {
		t.Fatal("preview created files or Git metadata")
	}
	if len(driver.calls)+len(driver.plans)+len(driver.queries) != 0 {
		t.Fatal("preview required Git, Docker or network")
	}
}

func TestTemplateOriginCreatesUnbornMainAndPreservesParentRepository(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	a := newOriginApp(t, "https://git.example.test/team/parent.git")
	driver := &templateInitGitDriver{}
	a.driver = driver
	parentConfig := filepath.Join(a.cwd, ".git", "config")
	before, err := os.ReadFile(parentConfig)
	if err != nil {
		t.Fatal(err)
	}
	origin := "ssh://git@git.example.test:777/team/template.git"
	output := filepath.Join(a.cwd, "new template")
	t.Run("ignore caller Git environment", func(t *testing.T) {
		global := filepath.Join(t.TempDir(), "gitconfig")
		if err := os.WriteFile(global, []byte("[init]\n\tdefaultBranch = wrong\n[core]\n\tbare = true\n"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GIT_CONFIG_GLOBAL", global)
		t.Setenv("GIT_DIR", filepath.Join(a.cwd, ".git"))
		t.Setenv("GIT_WORK_TREE", a.cwd)
		if code := a.run(templateOriginArgs(output, origin)); code != 0 {
			t.Fatal("template generation failed")
		}
	})
	if t.Failed() {
		return
	}
	if originGit(t, output, "symbolic-ref", "HEAD") != "refs/heads/main" || originGit(t, output, "rev-list", "--all") != "" || originGit(t, output, "ls-files") != "" {
		t.Fatal("template acquired a commit, tag, staging or the wrong branch")
	}
	if originGit(t, output, "config", "--local", "--get-all", "remote.origin.url") != origin {
		t.Fatal("origin URL changed")
	}
	if originGit(t, output, "config", "--local", "--get", "core.bare") != "false" {
		t.Fatal("template is not an ordinary clone")
	}
	after, err := os.ReadFile(parentConfig)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("parent repository configuration changed", err)
	}
	if len(driver.queries) != 3 || len(driver.calls)+len(driver.plans) != 0 {
		t.Fatal("template initialization ran unexpected tools")
	}
	if _, err := os.Lstat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
		t.Fatal("template initialization created image config")
	}
	expectFailure(t, "new directory", func() { a.run(templateOriginArgs(output, "https://other.example.test/team/template.git")) })
	if originGit(t, output, "config", "--local", "--get-all", "remote.origin.url") != origin {
		t.Fatal("existing template origin was replaced")
	}
}

func TestTemplateOriginGitFailuresPreserveGeneratedFiles(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		a, _, _ := testApp(t)
		a.driver = &templateInitGitDriver{failAt: failAt}
		message := "Git is required"
		if failAt > 1 {
			message = "files are preserved"
		}
		expectFailure(t, message, func() { a.run(templateOriginArgs("generated", "https://git.example.test/team/template.git")) })
		_, err := os.Stat(filepath.Join(a.cwd, "generated", "template.yml"))
		if failAt == 1 && !os.IsNotExist(err) || failAt > 1 && err != nil {
			t.Fatal("Git failure has incorrect output preservation", err)
		}
	}
}

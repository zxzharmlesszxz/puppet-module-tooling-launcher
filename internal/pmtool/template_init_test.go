package pmtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateInitIsLocalAndNeverOverwrites(t *testing.T) {
	a, driver, out := testApp(t)
	physical, err := filepath.EvalSymlinks(a.cwd)
	if err != nil {
		t.Fatal(err)
	}
	a.cwd = physical
	args := []string{"template-init", "--name", "team", "--output", "new-template", "--provider", "gitlab", "--launcher-version", "v1.1.0"}
	if code := a.run(append([]string{"--dry-run"}, args...)); code != 0 || !strings.Contains(out.String(), "template.yml") {
		t.Fatal("invalid preview")
	}
	if _, err := os.Stat(filepath.Join(a.cwd, "new-template")); !os.IsNotExist(err) {
		t.Fatal("preview created output")
	}
	if code := a.run(args); code != 0 || !strings.Contains(out.String(), "template-test") {
		t.Fatal("generation failed or missing guidance")
	}
	if len(driver.calls)+len(driver.plans)+len(driver.queries) != 0 {
		t.Fatal("generation called Git, Docker or network")
	}
	if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
		t.Fatal("generation created image config")
	}
	expectFailure(t, "new directory", func() { a.run(args) })
	if code := a.run([]string{"--executor", "native", "template-init", "--help"}); code != 0 {
		t.Fatal("local help failed")
	}
}

func TestNewFeatureCompletionIsLocal(t *testing.T) {
	a, driver, out := testApp(t)
	a.run([]string{"__complete", "template-init", "--provider", ""})
	if !strings.Contains(out.String(), "both") {
		t.Fatal("missing both-provider choice")
	}
	out.Reset()
	a.run([]string{"__complete", "pipeline-logs", "--"})
	for _, flag := range []string{"--pipeline", "--job", "--failed", "--tail", "--output"} {
		if !strings.Contains(out.String(), flag) {
			t.Fatalf("missing %s", flag)
		}
	}
	if len(driver.calls) != 0 {
		t.Fatal("completion called external commands")
	}
}

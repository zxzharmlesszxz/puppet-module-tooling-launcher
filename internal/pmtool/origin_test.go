package pmtool

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func originGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal("Git origin fixture failed", err)
	}
	return strings.TrimSpace(string(output))
}

func newOriginApp(t *testing.T, origin string) app {
	t.Helper()
	a, _, _ := testApp(t)
	a.driver = realDriver{}
	a.templateOperation = nil
	originGit(t, a.cwd, "init", "--initial-branch=main")
	originGit(t, a.cwd, "remote", "add", "origin", origin)
	return a
}

func TestProviderInferenceUsesConfiguredOriginWithRunnerInclude(t *testing.T) {
	for _, host := range []string{"git.example.test", "github.com"} {
		t.Run(host, func(t *testing.T) {
			origin := "https://" + host + "/team/puppet-sample.git"
			a := newOriginApp(t, origin)
			include := filepath.Join(a.cwd, ".git", "runner-config")
			originGit(t, a.cwd, "config", "--file", include,
				"url.https://gitlab-ci-token:fixture-ci-token@"+host+"/.insteadOf", "https://"+host+"/")
			originGit(t, a.cwd, "config", "--local", "include.path", include)
			if !strings.Contains(originGit(t, a.cwd, "remote", "get-url", "origin"), "fixture-ci-token@") {
				t.Fatal("runner include did not rewrite the transport URL")
			}
			settings := a.resolveWorkflowSettings(a.cwd, workflowSettings{})
			provider := "gitlab"
			if host == "github.com" {
				provider = "github"
			}
			configured, err := a.configuredOrigin(a.cwd)
			if err != nil || configured != origin || settings.Provider != provider {
				t.Fatal("runner credentials changed provider inference", settings.Provider, err)
			}
		})
	}
}

func TestTemplateAuditUsesConfiguredOriginAndRetainsTemplateCredentialScope(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, explicitProvider := range []string{"", "gitlab"} {
			a := newOriginApp(t, "https://git.example.test/team/puppet-sample.git")
			originGit(t, a.cwd, "config", "--local",
				"url.https://gitlab-ci-token:fixture-ci-token@git.example.test/.insteadOf", "https://git.example.test/")
			path := filepath.Join(a.cwd, ".sync.yml")
			body := []byte("template:\n  use: 'git.example.test/team/template@v1.0.0'\n")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			environment := map[string]string{"TOOLING_TEMPLATE_TOKEN": "fixture-template-token", "TOOLING_TEMPLATE_TOKEN_HOST": "other.example.test"}
			a.getenv = func(key string) string { return environment[key] }
			configPath := filepath.Join(a.cwd, ".git", "config")
			before, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			err = a.executeTemplate(config{}, "", operationOptions{Module: a.cwd, Settings: workflowSettings{Provider: explicitProvider}}, "audit", native)
			if err == nil || !strings.Contains(err.Error(), "TOOLING_TEMPLATE_TOKEN_HOST must match") {
				t.Fatal("audit stopped at rewritten origin or bypassed template credential scope", err)
			}
			if strings.Contains(err.Error(), "fixture-ci-token") || strings.Contains(err.Error(), "fixture-template-token") {
				t.Fatal("audit failure exposed credentials")
			}
			after, err := os.ReadFile(configPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("audit rewrote Git credentials or origin", err)
			}
			after, err = os.ReadFile(path)
			if err != nil || !bytes.Equal(body, after) {
				t.Fatal("failed audit changed module settings", err)
			}
		}
	}
}

func TestProviderInferenceRejectsStoredOriginCredentialsMaskedByRewrite(t *testing.T) {
	a := newOriginApp(t, "https://user:fixture-stored-secret@git.example.test/team/puppet-sample.git")
	originGit(t, a.cwd, "config", "--local", "url.https://git.example.test/.insteadOf", "https://user:fixture-stored-secret@git.example.test/")
	if strings.Contains(originGit(t, a.cwd, "remote", "get-url", "origin"), "fixture-stored-secret") {
		t.Fatal("fixture did not conceal stored credentials in the transport URL")
	}
	expectFailure(t, "origin must not contain a password, query or fragment", func() {
		a.resolveWorkflowSettings(a.cwd, workflowSettings{})
	})
}

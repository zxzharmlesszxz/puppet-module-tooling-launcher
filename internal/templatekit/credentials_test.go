package templatekit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateCredentialOverrideHasPriority(t *testing.T) {
	env := credentialEnvironment()
	c, err := NewCredentials("git.example.test/team/template", func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveTemplateCredentials(t.Context(), t.TempDir(), c, func(context.Context, string, Credentials) (Credentials, error) {
		t.Fatal("explicit credentials consulted host helper")
		return Credentials{}, nil
	})
	if err != nil || got != c {
		t.Fatal("explicit override changed", err)
	}
	env["TOOLING_TEMPLATE_TOKEN_HOST"] = "wrong.example.test"
	if _, err := NewCredentials(c.Repository, func(k string) string { return env[k] }); err == nil {
		t.Fatal("mismatching override accepted")
	}
}
func helperFixture(t *testing.T, response string, usePath bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	global := filepath.Join(root, "global.gitconfig")
	helper := filepath.Join(root, "helper")
	marker := filepath.Join(root, "operations")
	query := filepath.Join(root, "request")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> " + shellQuote(marker) + "\ncat > " + shellQuote(query) + "\nprintf '%s' " + shellQuote(response) + "\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	config := "[credential]\n\thelper = !" + shellQuote(helper) + "\n\tuseHttpPath = false\n"
	if usePath {
		config = strings.ReplaceAll(config, "false", "true")
	}
	if err := os.WriteFile(global, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	if err := exec.Command("git", "-c", "core.hooksPath=/dev/null", "init", "--quiet", root).Run(); err != nil {
		t.Fatal(err)
	}
	return root, marker
}
func TestStoredTemplateCredentialsUseExactURLAndNeverStore(t *testing.T) {
	for _, usePath := range []bool{false, true} {
		t.Run(map[bool]string{false: "host-wide", true: "path-scoped"}[usePath], func(t *testing.T) {
			root, marker := helperFixture(t, "username=stored-user\npassword=stored-secret\n", usePath)
			c, err := NewCredentials("git.example.test/team/template", func(string) string { return "" })
			if err != nil {
				t.Fatal(err)
			}
			got, err := resolveTemplateCredentials(t.Context(), root, c, nil)
			if err != nil || got.Username != "stored-user" || got.Token != "stored-secret" || got.Repository != c.Repository {
				t.Fatal("stored credential resolution failed", err)
			}
			operations, err := os.ReadFile(marker)
			if err != nil || string(operations) != "get\n" {
				t.Fatal("helper performed a persistence operation", err)
			}
			query, err := os.ReadFile(filepath.Join(root, "request"))
			if err != nil || !strings.Contains(string(query), "host=git.example.test") {
				t.Fatal("helper lost selected host", err)
			}
			if usePath && !strings.Contains(string(query), "path=team/template.git") {
				t.Fatal("helper lost selected repository path")
			}
			if got.Redact(got.Token) != "[REDACTED]" {
				t.Fatal("helper credential is not redacted")
			}
		})
	}
}
func TestStoredTemplateCredentialsRejectForeignContext(t *testing.T) {
	for _, response := range []string{
		"protocol=http\nusername=u\npassword=secret\n",
		"host=elsewhere.example.test\nusername=u\npassword=secret\n",
		"path=team/other.git\nusername=u\npassword=secret\n",
	} {
		t.Run(strings.Split(response, "\n")[0], func(t *testing.T) {
			root, _ := helperFixture(t, response, true)
			c, err := NewCredentials("git.example.test/team/template", func(string) string { return "" })
			if err != nil {
				t.Fatal(err)
			}
			got, lookupErr := lookupGitCredentials(t.Context(), root, c)
			if lookupErr == nil && got.Token != "" {
				t.Fatal("foreign helper credential accepted")
			}
			if lookupErr != nil && strings.Contains(lookupErr.Error(), "secret") {
				t.Fatal("helper credential disclosed", lookupErr)
			}
		})
	}
}
func TestMissingStoredCredentialsRemainAnonymousWithoutPrompts(t *testing.T) {
	root, _ := helperFixture(t, "", true)
	marker := filepath.Join(root, "askpass-called")
	askpass := filepath.Join(root, "askpass")
	if err := os.WriteFile(askpass, []byte("#!/bin/sh\ntouch "+shellQuote(marker)+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_ASKPASS", askpass)
	t.Setenv("SSH_ASKPASS", askpass)
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	c, err := NewCredentials("git.example.test/team/template", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	got, err := lookupGitCredentials(t.Context(), root, c)
	if err != nil || got.Token != "" {
		t.Fatal("missing helper unexpectedly authenticated", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("caller askpass was executed", err)
	}
}
func TestHelperEnvironmentRemovesTokensAndTracing(t *testing.T) {
	for _, key := range []string{"GITLAB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN", "CI_JOB_TOKEN", "TOOLING_TEMPLATE_TOKEN", "FORGE_TOKEN", "FORGE_PUBLISH_TOKEN", "GIT_TRACE", "GIT_TRACE_CURL", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_DIR", "GIT_ASKPASS", "SSH_ASKPASS"} {
		t.Setenv(key, "MUST_NOT_REACH_HELPER")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/user-selected/config")
	env := credentialLookupEnvironment()
	for _, entry := range env {
		if strings.Contains(entry, "MUST_NOT_REACH_HELPER") {
			t.Fatal("credential lookup inherited unrelated secrets or tracing")
		}
	}
	if !contains(env, "GIT_CONFIG_GLOBAL=/user-selected/config") {
		t.Fatal("user Git configuration override lost")
	}
}
func TestHelperProtocolRejectsMalformedAndOversizedResponses(t *testing.T) {
	for _, body := range []string{"password=a\npassword=b\n", "password=secret\r\n", "password=secret\x00\n", "not-a-field\n"} {
		if _, err := parseCredentialOutput(body); err == nil {
			t.Fatal("malformed helper response accepted")
		}
	}
	var output credentialOutput
	if _, err := output.Write(make([]byte, 64<<10+1)); err == nil {
		t.Fatal("oversized helper output accepted")
	}
}

func TestRunGitFetchAutomaticallyUsesStoredHelperAndRedactsErrors(t *testing.T) {
	root, marker := helperFixture(t, "username=stored-user\npassword=stored-secret\n", true)
	actualGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	wrapper := filepath.Join(directory, "git")
	body := "#!/bin/sh\nfor argument do\n  if [ \"$argument\" = fetch ]; then\n    printf 'fetch token=%s\\n' \"$PMTOOL_TEMPLATE_TOKEN\" >&2\n    exit 1\n  fi\ndone\nexec " + shellQuote(actualGit) + " \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	c, err := NewCredentials("git.example.test/team/template", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	_, err = RunGit(t.Context(), root, c, "fetch", "https://git.example.test/team/template.git", "refs/tags/v1.0.0")
	if err == nil || strings.Contains(err.Error(), "stored-secret") || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatal("fetch did not use/redact stored helper credentials", err)
	}
	operations, readErr := os.ReadFile(marker)
	if readErr != nil || string(operations) != "get\n" {
		t.Fatal("host credential helper performed unexpected operations", readErr)
	}
}

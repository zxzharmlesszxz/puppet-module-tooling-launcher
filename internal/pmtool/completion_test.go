package pmtool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionGenerationIsLocalAndPreservesScript(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		a, driver, output := testApp(t)
		a.tty, a.color = true, "always"
		if a.run([]string{"--color", "always", "completion", shell}) != 0 || len(driver.calls) != 0 {
			t.Fatal("completion generation contacted Docker")
		}
		expected := bashCompletion
		if shell == "zsh" {
			expected = zshCompletion
		}
		if output.String() != expected || strings.Contains(output.String(), "\x1b[") {
			t.Fatal("completion script was rendered or modified")
		}
		if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
			t.Fatal("completion generation wrote configuration")
		}
	}
	a, _, _ := testApp(t)
	expectFailure(t, "unsupported completion shell", func() { a.run([]string{"completion", "fish"}) })
}

func TestCompletionLocalChoicesDoNotNeedDocker(t *testing.T) {
	for _, test := range []struct {
		words []string
		want  string
	}{
		{[]string{"completion", "z"}, "zsh\n"},
		{[]string{"config", "i"}, "init\n"},
		{[]string{"ai", "export", "--format", "s"}, "skill\n"},
		{[]string{"audit", "--provider", "g"}, "github\ngitlab\n"},
		{[]string{"--color=a"}, "--color=always\n--color=auto\n"},
	} {
		a, driver, output := testApp(t)
		a.run(append([]string{"__complete"}, test.words...))
		if output.String() != test.want || len(driver.calls) != 0 {
			t.Fatalf("wrong local completion for %v: %q", test.words, output.String())
		}
	}
}

func TestWorkflowCompletionIsLocalAndIndependentOfImage(t *testing.T) {
	a, driver, output := testApp(t)
	a.run([]string{"__complete", "--image", "tooling:custom", "ch"})
	if output.String() != "check\n" || len(driver.calls) != 0 || len(driver.plans) != 0 {
		t.Fatal("workflow completion contacted Docker or lost launcher commands")
	}
	if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
		t.Fatal("completion created configuration")
	}
}

func TestCompletionDoesNotRequireInstalledImage(t *testing.T) {
	a, driver, output := testApp(t)
	driver.missing = true
	a.run([]string{"__complete", "co"})
	if output.String() != "completion\nconfig\n" || len(driver.calls) != 0 {
		t.Fatal("completion depended on an installed image")
	}
	a, _, output = testApp(t)
	a.run([]string{"__complete", "audit", "--"})
	if !strings.Contains(output.String(), "--module\n") || strings.Contains(output.String(), "--host-module") {
		t.Fatal("completion exposed launcher-managed flags")
	}
}

func TestCompletionScriptsParseInAvailableShells(t *testing.T) {
	for _, test := range []struct{ shell, script string }{{"bash", bashCompletion}, {"zsh", zshCompletion}} {
		t.Run(test.shell, func(t *testing.T) {
			if _, err := exec.LookPath(test.shell); err != nil {
				t.Skip("shell is not installed")
			}
			cmd := exec.Command(test.shell, "-n")
			cmd.Stdin = strings.NewReader(test.script)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("invalid completion syntax: %v: %s", err, output)
			}
		})
	}
}

func TestBashCompletionInvokesCallbackWithoutEvaluation(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "completion.bash")
	if err := os.WriteFile(script, []byte(bashCompletion), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "fake pmtool")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'check\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", `source "$1"; COMP_WORDS=("$2" ch); COMP_CWORD=1; _pmtool; printf '%s\n' "${COMPREPLY[@]}"`, "completion-test", script, binary)
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "check\n" {
		t.Fatalf("completion callback or executable path with spaces failed: %v: %s", err, output)
	}
}

func TestCompletionRespectsConfigAndDoesNotSuggestFlagValuesAsCommands(t *testing.T) {
	a, driver, output := testApp(t)
	path := filepath.Join(t.TempDir(), "custom.json")
	createConfig(path, config{Image: "registry.example/tooling:v2", Docker: "custom-docker"})
	a.run([]string{"__complete", "--config", path, "ch"})
	if output.String() != "check\n" || len(driver.calls) != 0 {
		t.Fatal("completion unnecessarily queried the selected image")
	}
	a, driver, output = testApp(t)
	a.run([]string{"__complete", "--image", ""})
	if output.Len() != 0 || len(driver.calls) != 0 {
		t.Fatal("completion interpreted an image value as a command")
	}
}

func TestCompletionFiltersFlagsByCommandAndCompletesDocSections(t *testing.T) {
	for _, row := range []struct{ command, want, forbidden string }{
		{"release-tag", "--module", "--stage"},
		{"pipeline-logs", "--tail", "--version"},
		{"audit", "--template", "--name"},
		{"module-new", "--name", "--watch"},
		{"template-test", "--module", "--title"},
	} {
		a, d, out := testApp(t)
		a.run([]string{"__complete", row.command, "--"})
		if !strings.Contains(out.String(), row.want+"\n") || strings.Contains(out.String(), row.forbidden+"\n") {
			t.Fatal("invalid command options suggested", row.command, out.String())
		}
		if len(d.calls)+len(d.plans)+len(d.queries) != 0 {
			t.Fatal("flag completion required external tools")
		}
	}
	a, _, out := testApp(t)
	a.run([]string{"__complete", "docs", "--section", "Resp"})
	if out.String() != "Responsibilities\n" {
		t.Fatal("default docs section completion failed", out.String())
	}
}
func TestZshCompletionInvokesQuotedCallback(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "completion.zsh")
	if err := os.WriteFile(script, []byte(zshCompletion), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "fake pmtool")
	body := "#!/bin/sh\n[ \"$1\" = __complete ] && [ \"$2\" = docs ] && [ \"$3\" = to ] || exit 1\nprintf 'tokens\\n'\n"
	if err := os.WriteFile(binary, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "zsh", "-f", "-c", `compdef() { :; }; compadd() { shift; printf '%s\n' "$@"; }; source "$1"; words=("$2" docs to); CURRENT=3; _pmtool`, "completion-test", script, binary)
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "tokens\n" {
		t.Fatalf("zsh callback failed with quoted executable path: %v: %s", err, output)
	}
}

func TestBashCompletionPreservesMultipleCallbackArguments(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "completion.bash")
	if err := os.WriteFile(script, []byte(bashCompletion), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "fake pmtool")
	body := "#!/bin/sh\n[ \"$1\" = __complete ] && [ \"$2\" = docs ] && [ \"$3\" = to ] && [ \"$#\" -eq 3 ] || exit 1\nprintf 'tokens\\n'\n"
	if err := os.WriteFile(binary, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "bash", "-c", `source "$1"; COMP_WORDS=("$2" docs to); COMP_CWORD=2; _pmtool; printf '%s\n' "${COMPREPLY[@]}"`, "completion-test", script, binary)
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "tokens\n" {
		t.Fatalf("bash callback lost multiple arguments: %v: %s", err, output)
	}
}

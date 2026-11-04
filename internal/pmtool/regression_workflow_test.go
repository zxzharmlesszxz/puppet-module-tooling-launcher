package pmtool

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMigratedEveryValidationFailureStopsLaterStages(t *testing.T) {
	for stop := range 6 {
		t.Run(strconv.Itoa(stop+1), func(t *testing.T) {
			a, d, out := testApp(t)
			d.results = make([]int, stop+1)
			d.results[stop] = 23 + stop
			code := a.run([]string{"check"})
			if code != 23+stop || len(d.plans) != stop+1 {
				t.Fatal("later steps ran or failure code changed", code, len(d.plans))
			}
			if !strings.Contains(out.String(), "[FAIL]") || strings.Contains(out.String(), "[OK] check completed") || !strings.Contains(out.String(), "[NEXT] Fix the reported failure") {
				t.Fatal(out.String())
			}
			for _, call := range d.calls {
				if strings.Contains(strings.Join(call, " "), "-- hint ") {
					t.Fatal("guidance still executed in Ruby")
				}
			}
		})
	}
}
func TestMigratedValidationSuccessAndSignalSummaries(t *testing.T) {
	for _, status := range []int{0, 29, 143} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			a, d, out := testApp(t)
			d.status = status
			code := a.run([]string{"release-checks"})
			if code != status || len(d.plans) != 1 {
				t.Fatal(code, d.plans)
			}
			want := "[OK]"
			if status != 0 {
				want = "[FAIL]"
			}
			if !strings.Contains(out.String(), "[RUN]") || !strings.Contains(out.String(), want) || !strings.Contains(out.String(), "[NEXT]") {
				t.Fatal("silent result or missing recovery", out.String())
			}
		})
	}
}
func TestMigratedLiteralShellQuotingRoundTrip(t *testing.T) {
	for _, value := range []string{"O'Reilly $(touch SHOULD_NOT_EXIST)", "double \"quotes\" and `command` ; echo $HOME | cat & > file < input", "backslash\\value\ttab\nnewline", "Привіт [REL] * ? ~ ! #", ""} {
		t.Run(value, func(t *testing.T) {
			dir := t.TempDir()
			args := []string{"pmtool", "mr-create", "--module", "/module with spaces", "--title", value}
			script := "set -- " + quotedCommand(args) + "; printf '%s\\0' \"$@\""
			cmd := exec.Command("bash", "-c", script)
			cmd.Dir = dir
			output, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			expected := []byte(strings.Join(args, "\x00") + "\x00")
			if !bytes.Equal(output, expected) {
				t.Fatal("shell quoting changed an argument")
			}
			if _, err := os.Stat(filepath.Join(dir, "SHOULD_NOT_EXIST")); !os.IsNotExist(err) {
				t.Fatal("title executed a shell command")
			}
		})
	}
}
func TestMigratedCatalogPrivilegesAndAliases(t *testing.T) {
	m := launcherManifest()
	for name, p := range m.Commands {
		if p.Socket {
			t.Fatal("nested socket permission", name)
		}
		if name == "help" || name == "docs" || name == "manifest" {
			if p.Module || p.Token || p.Author || p.TemplateToken {
				t.Fatal("metadata privileges", name)
			}
		}
	}
	for alias, target := range map[string]string{"bootstrap-local": "sync", "bootstrap-local-noop": "sync-noop", "audit-local": "audit", "pdk-local": "pdk", "pdk-convert-local": "pdk-convert", "pdk-convert-local-noop": "pdk-convert-noop"} {
		if m.Commands[alias] != m.Commands[target] {
			t.Fatal("alias privileges drifted", alias)
		}
	}
}
func TestMigratedGuidanceHasNoContainerOrCredentialDependency(t *testing.T) {
	a, d, out := testApp(t)
	a.getenv = func(key string) string {
		switch key {
		case "GITLAB_TOKEN", "GITHUB_TOKEN", "TOOLING_TEMPLATE_TOKEN":
			return "NEVER_PRINT_SECRET"
		}
		return ""
	}
	a.workflowHint([]string{"release-prepare", "--version", "1.2.3", "--ci", "github.com/team/ci/.github/workflows/build.yml@v1"}, "release-prepare", true)
	if len(d.calls) != 0 || len(d.plans) != 0 || !strings.Contains(out.String(), "pmtool sync") || strings.Contains(out.String(), "NEVER_PRINT_SECRET") {
		t.Fatal("guidance requires SDK/credentials", out.String())
	}
}

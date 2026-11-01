package main

import (
	"strings"
	"testing"
)

func TestTerminalDocsAndRawExport(t *testing.T) {
	a, _, _ := testApp(t)
	markdown := "# Довідка\n\nДовгий абзац українською мовою для перевірки перенесення тексту у вузькому терміналі.\n\n## Підготовка\n\n```sh\nprintf '%s' 'keep  spaces'\n# not a heading\n```\n\n## Реліз\n\nRelease notes.\n"
	if a.document(markdown, "") != markdown {
		t.Fatal("redirected docs changed Markdown")
	}
	contents := a.document(markdown, "list")
	if !strings.Contains(contents, "Підготовка") || strings.Contains(contents, "not a heading") {
		t.Fatal("contents parsed a heading inside code")
	}
	part := a.document(markdown, "Підготовка")
	if !strings.Contains(part, "printf '%s' 'keep  spaces'") || strings.Contains(part, "Release notes.") {
		t.Fatal("section selection damaged code or included sibling")
	}
	a.tty = true
	a.getenv = func(key string) string {
		if key == "COLUMNS" {
			return "40"
		}
		return ""
	}
	rendered := a.document(markdown, "")
	if !strings.Contains(rendered, "\x1b[1;33m") || strings.Contains(rendered, "```") || !strings.Contains(rendered, "printf '%s' 'keep  spaces'") {
		t.Fatal("terminal docs lost formatting or literal code")
	}
	for _, line := range wrapText("Довгий абзац українською мовою для перевірки перенесення", 20) {
		if len([]rune(line)) > 20 {
			t.Fatal("prose wrapping counted UTF-8 bytes")
		}
	}
	expectFailure(t, "expected one match", func() { a.document(markdown, "missing") })
}

func TestColorPolicyAndDockerPropagation(t *testing.T) {
	for _, scenario := range []struct {
		name, mode, noColor, term string
		tty, expected             bool
	}{
		{"terminal", "auto", "", "xterm", true, true},
		{"pipe", "auto", "", "xterm", false, false},
		{"forced", "always", "", "xterm", false, true},
		{"disabled", "never", "", "xterm", true, false},
		{"NO_COLOR", "always", "1", "xterm", true, false},
		{"dumb", "auto", "", "dumb", true, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a, _, output := testApp(t)
			a.tty, a.color = scenario.tty, scenario.mode
			a.getenv = func(key string) string {
				if key == "NO_COLOR" {
					return scenario.noColor
				}
				if key == "TERM" {
					return scenario.term
				}
				return ""
			}
			a.notice("FAIL", "command failed", "1;31")
			if strings.Contains(output.String(), "\x1b[") != scenario.expected {
				t.Fatal("unexpected ANSI policy")
			}
			p := a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"audit"})
			if (p.Environment["TOOLING_COLOR"] == "always") != scenario.expected {
				t.Fatal("runtime color mode drifted from launcher")
			}
		})
	}
}

func TestLocalSectionHelpAndInvalidColor(t *testing.T) {
	a, d, output := testApp(t)
	a.run([]string{"docs", "launcher", "--section", "list"})
	if output.Len() == 0 || len(d.calls) != 0 {
		t.Fatal("local documentation contents contacted Docker")
	}
	expectFailure(t, "color must be", func() { a.run([]string{"--color", "invalid", "help"}) })
}

func TestTerminalWidthIsForwardedWithoutSecrets(t *testing.T) {
	a, _, _ := testApp(t)
	a.columns = 72
	p := a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"audit"})
	if p.Environment["COLUMNS"] != "72" {
		t.Fatal("runtime did not receive terminal width")
	}
}

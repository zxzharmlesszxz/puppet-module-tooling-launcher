package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIHelpIsLocal(t *testing.T) {
	for _, args := range [][]string{{"ai"}, {"ai", "help"}, {"ai", "export", "--help"}} {
		a, d, output := testApp(t)
		a.run(args)
		if len(d.calls) != 0 || !strings.Contains(output.String(), "ai export") {
			t.Fatal("AI help contacted Docker or lacks usage")
		}
		if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
			t.Fatal("AI help created config")
		}
	}
}

func TestExportFormatsUseSelectedImageWithoutMountsOrCredentials(t *testing.T) {
	for _, format := range []string{"agents", "skill", "prompt"} {
		t.Run(format, func(t *testing.T) {
			a, d, output := testApp(t)
			a.tty, a.color = true, "always"
			a.getenv = func(key string) string {
				if key == "GITLAB_TOKEN" {
					return "secret-token"
				}
				return ""
			}
			d.document = "# Workflow from selected image\n\nUse the current clone.\n"
			a.run([]string{"--image", "tooling:local", "ai", "export", "--format", format})
			body := output.String()
			if !strings.Contains(body, d.document) || !strings.Contains(body, imageID) || !strings.Contains(body, "pipeline-jobs") || strings.Contains(body, "\x1b[") || strings.Contains(body, "secret-token") {
				t.Fatal("export lost image provenance, altered Markdown, or disclosed credentials")
			}
			if strings.HasPrefix(body, "---\n") != (format == "skill") {
				t.Fatal("skill frontmatter leaked to another format or is missing")
			}
			if format == "skill" && !strings.Contains(body, "name: puppet-module-workflow\n") {
				t.Fatal("skill has no discoverable name")
			}
			if len(d.plans) != 0 || len(d.calls) != 3 {
				t.Fatal("export ran a module action")
			}
			query := strings.Join(d.calls[2], " ")
			if !strings.Contains(query, "--network none "+imageID+" docs ai") || strings.Contains(query, "--mount") || strings.Contains(query, "--env") {
				t.Fatal("export received module/token/socket or used a different image")
			}
			if _, err := os.Stat(filepath.Join(a.cwd, "AGENTS.md")); !os.IsNotExist(err) {
				t.Fatal("stdout export silently installed repository instructions")
			}
		})
	}
}

func TestExportCreatesExplicitDestinationAndNeverReplacesIt(t *testing.T) {
	a, _, output := testApp(t)
	path := filepath.Join(a.cwd, "new-skill", "SKILL.md")
	a.run([]string{"--image", "tooling:local", "ai", "export", "--format", "skill", "--output", "new-skill/SKILL.md"})
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(data), "---\nname:") || !strings.Contains(output.String(), "[OK]") {
		t.Fatal("explicit skill export failed")
	}
	if files, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".puppet-tooling-ai-*")); err != nil || len(files) != 0 {
		t.Fatal("export left staging files")
	}
	expectFailure(t, "refusing to overwrite", func() {
		a.run([]string{"--image", "tooling:local", "ai", "export", "--output", path})
	})
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatal("existing instructions changed")
	}
}

func TestAtomicPublishRefusesAConcurrentDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(path, []byte("original instructions"), 0644); err != nil {
		t.Fatal(err)
	}
	expectFailure(t, "file exists", func() { writeNewInstructions(path, []byte("replacement")) })
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original instructions" {
		t.Fatal("atomic publish replaced concurrent instructions")
	}
	assertNoInstructionTemps(t, filepath.Dir(path))
}

func TestExportRefusesSymlinkAndInvalidFormatBeforeDocker(t *testing.T) {
	a, d, _ := testApp(t)
	path := filepath.Join(a.cwd, "AGENTS.md")
	if err := os.Symlink("missing-target", path); err != nil {
		t.Fatal(err)
	}
	expectFailure(t, "refusing to overwrite", func() {
		a.run([]string{"ai", "export", "--output", path})
	})
	expectFailure(t, "AI format must be", func() {
		a.run([]string{"ai", "export", "--format", "other"})
	})
	if len(d.calls) != 0 {
		t.Fatal("invalid export contacted Docker")
	}
}

func TestExportDryRunDoesNotCreateDestination(t *testing.T) {
	a, d, output := testApp(t)
	path := filepath.Join(a.cwd, "not-created", "AGENTS.md")
	a.run([]string{"--image", "tooling:local", "--dry-run", "ai", "export", "--output", path})
	var preview struct {
		Format, Output, Image string
		Write                 bool
	}
	if err := json.Unmarshal(output.Bytes(), &preview); err != nil || preview.Write || preview.Image != imageID || preview.Output != path {
		t.Fatal("AI dry-run is not clean export metadata")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) || len(d.plans) != 0 {
		t.Fatal("AI dry-run wrote the destination or executed a module action")
	}
}

func TestOldRuntimeAndEmptyDocsLeaveNoOutputFile(t *testing.T) {
	for _, document := range []string{"", "not Markdown"} {
		a, d, _ := testApp(t)
		d.document = document
		expectFailure(t, "nonempty Markdown", func() {
			a.run([]string{"--image", "tooling:local", "ai", "export", "--output", "AGENTS.md"})
		})
		if _, err := os.Stat(filepath.Join(a.cwd, "AGENTS.md")); !os.IsNotExist(err) {
			t.Fatal("invalid instructions created output")
		}
	}
	a, d, _ := testApp(t)
	d.docsError = errors.New("unknown document: ai")
	expectFailure(t, "image providing docs ai", func() {
		a.run([]string{"--image", "tooling:local", "ai", "export"})
	})
}

func TestExportRefusesPrivilegedDocsProfile(t *testing.T) {
	a, d, _ := testApp(t)
	m := capabilities()
	m.Commands["docs"] = profile{Socket: true}
	expectFailure(t, "without a module, token or socket", func() {
		a.exportAI(config{Docker: "docker"}, imageID, m, aiOptions{format: "agents"}, "-", false)
	})
	if len(d.calls) != 0 {
		t.Fatal("privileged docs profile was executed")
	}
}

func assertNoInstructionTemps(t *testing.T, directory string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(directory, ".puppet-tooling-ai-*"))
	if err != nil || len(paths) != 0 {
		t.Fatalf("temporary instruction files remain: %v (%v)", paths, err)
	}
}

func TestInstructionPublishCleansTemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	writeNewInstructions(path, []byte("instructions"))
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "instructions" {
		t.Fatalf("published instructions unavailable: %v", err)
	}
	assertNoInstructionTemps(t, filepath.Dir(path))
}

func TestExportConsoleWriteFailures(t *testing.T) {
	for _, args := range [][]string{
		{"--image", "tooling:local", "ai", "export"},
		{"--image", "tooling:local", "--dry-run", "ai", "export"},
	} {
		a, _, _ := testApp(t)
		a.stdout = failingWriter{}
		expectFailure(t, "output unavailable", func() { a.run(args) })
	}
}

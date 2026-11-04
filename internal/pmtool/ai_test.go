package pmtool

import (
	"encoding/json"
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

func TestExportFormatsUseLauncherDocsWithoutDocker(t *testing.T) {
	for _, format := range []string{"agents", "skill", "prompt"} {
		t.Run(format, func(t *testing.T) {
			a, d, output := testApp(t)
			a.tty, a.color = true, "always"
			a.run([]string{"--image", "unavailable:local", "ai", "export", "--format", format})
			body := output.String()
			if !strings.Contains(body, "pipeline-jobs") || !strings.Contains(body, "Launcher version:") || strings.Contains(body, "\x1b[") {
				t.Fatal("export lost launcher provenance or rendered Markdown")
			}
			if strings.HasPrefix(body, "---\n") != (format == "skill") {
				t.Fatal("incorrect frontmatter")
			}
			if len(d.plans) != 0 || len(d.calls) != 0 {
				t.Fatal("AI export contacted Docker")
			}
			if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
				t.Fatal("export created host config")
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
	if err := json.Unmarshal(output.Bytes(), &preview); err != nil || preview.Write || preview.Image != "" || preview.Output != path {
		t.Fatal("AI dry-run is not clean export metadata")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) || len(d.plans) != 0 {
		t.Fatal("AI dry-run wrote the destination or executed a module action")
	}
}

func TestInstructionRendererRefusesEmptyOrNonMarkdownDocs(t *testing.T) {
	for _, document := range []string{"", "not Markdown"} {
		expectFailure(t, "nonempty Markdown", func() { renderInstructions(document, launcherManifest(), "agents") })
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

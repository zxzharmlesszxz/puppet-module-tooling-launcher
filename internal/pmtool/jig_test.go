package pmtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovedToolchainCannotBeSelected(t *testing.T) {
	a, _, _ := testApp(t)
	m := launcherManifest()
	for name := range m.Commands {
		if strings.HasPrefix(name, "pdk") {
			t.Fatal("retired command remains", name)
		}
	}
	for _, command := range []string{"pdk", "pdk-local", "pdk-convert", "pdk-convert-noop", "pdk-convert-local", "pdk-convert-local-noop"} {
		expectFailure(t, "unsupported validation", func() { a.sequence(a.parseOperation([]string{command})) })
	}
	expectFailure(t, "unsupported runtime option", func() { a.parseOperation([]string{"check", "--pdk-image", "ignored"}) })
	for _, name := range m.Options {
		if strings.Contains(name, "pdk") {
			t.Fatal("retired image option remains")
		}
	}
}

func TestJigPlansKeepNoOverwritePreviewAndEntryPoint(t *testing.T) {
	a, _, _ := testApp(t)
	if err := os.Mkdir(filepath.Join(a.cwd, "manifests"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"module-new", "module-build", "module-convert", "module-convert-noop"} {
		arguments := []string{command}
		if command == "module-new" {
			arguments = append(arguments, "--name", "sample")
		}
		steps := a.sequence(a.parseOperation(arguments)).Steps
		if len(steps) != 1 || steps[0].Tool != "jig" {
			t.Fatal("jig operation used a different backend")
		}
		step := steps[0]
		joined := strings.Join(step.Command, " ")
		if !strings.Contains(joined, "--entrypoint jig "+voxboxImage) || strings.Contains(joined, "--force") || step.Permission.Token || step.Permission.Socket || step.Permission.TemplateToken {
			t.Fatal("jig isolation or no-overwrite contract changed", joined)
		}
		if strings.Contains(joined, "--dry-run") != (command == "module-convert-noop") {
			t.Fatal("conversion preview lost its no-write flag")
		}
		if nativeStep(step).Docker != "jig" {
			t.Fatal("native build does not use jig")
		}
	}
	if err := os.WriteFile(filepath.Join(a.cwd, ".sync.lock.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	expectFailure(t, "initial onboarding", func() { a.sequence(a.parseOperation([]string{"module-convert"})) })
	for _, name := range []string{"", "../outside", "Upper", "a/b", "--force"} {
		args := []string{"module-new"}
		if name != "" {
			args = append(args, "--name", name)
		}
		expectFailure(t, "module-new requires", func() { a.parseOperation(args) })
	}
	if err := os.Symlink("missing-target", filepath.Join(a.cwd, "existing")); err != nil {
		t.Fatal(err)
	}
	expectFailure(t, "must not exist", func() { a.parseOperation([]string{"module-new", "--name", "existing"}) })
}

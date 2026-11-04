package pmtool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type sourceCIStep struct{ ID, Name, Run string }

func sourceWorkflowSteps(t *testing.T, job string) []sourceCIStep {
	t.Helper()
	body, err := os.ReadFile("../../.github/workflows/launcher.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []sourceCIStep
		}
	}
	if err := yaml.Unmarshal(body, &workflow); err != nil {
		t.Fatal(err)
	}
	steps := workflow.Jobs[job].Steps
	if len(steps) == 0 {
		t.Fatalf("missing workflow job: %s", job)
	}
	return steps
}

func sourceWorkflowStep(t *testing.T, job, name string) string {
	t.Helper()
	for _, step := range sourceWorkflowSteps(t, job) {
		if step.ID == name || step.Name == name {
			return step.Run
		}
	}
	t.Fatalf("missing workflow step: %s", name)
	return ""
}

func TestCommitCIEmbedsTheExactSourceVersion(t *testing.T) {
	script := sourceWorkflowStep(t, "build", "version")
	sha := strings.Repeat("a", 40)
	for _, test := range []struct{ kind, name, expected string }{
		{"branch", "main", "dev-" + sha},
		{"branch", "feature/example", "dev-" + sha},
		{"tag", "v1.2.3", "v1.2.3"},
	} {
		t.Run(test.kind+"/"+test.name, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "output")
			command := exec.Command("sh", "-eu", "-c", script)
			command.Env = append(os.Environ(), "REF_TYPE="+test.kind, "REF_NAME="+test.name, "SOURCE_SHA="+sha, "GITHUB_OUTPUT="+outputPath)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("version step failed: %v: %s", err, output)
			}
			data, err := os.ReadFile(outputPath)
			if err != nil || string(data) != "version="+test.expected+"\n" {
				t.Fatalf("wrong CI artifact version: %s: %v", data, err)
			}
		})
	}
}

func TestTagCIRequiresTheSuccessfulCurrentMainCommit(t *testing.T) {
	script := sourceWorkflowStep(t, "build", "Verify tagged main and its commit CI")
	for _, scenario := range []string{"success", "different main", "changed main", "failed main CI"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			programs := map[string]string{
				"git": "#!/bin/sh\ntest \"$*\" = 'rev-parse HEAD' || exit 2\nprintf '%s\\n' abc\n",
				"gh": `#!/bin/sh
set -eu
if [ "$TEST_SCENARIO" = 'different main' ] || { [ "$TEST_SCENARIO" = 'changed main' ] && [ -e "$TEST_ROOT/waited" ]; }; then
  printf '%s\n' changed
else
  printf '%s\n' abc
fi
`,
				"make": `#!/bin/sh
set -eu
test "$*" = 'release-ci-wait RELEASE_REF=main'
printf '%s\n' waited > "$TEST_ROOT/waited"
test "$TEST_SCENARIO" != 'failed main CI'
`,
			}
			for name, body := range programs {
				if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.Command("sh", "-eu", "-c", script)
			command.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_ROOT="+root, "TEST_SCENARIO="+scenario, "GH_REPO=example/launcher", "GH_TOKEN=")
			output, err := command.CombinedOutput()
			if (err == nil) != (scenario == "success") {
				t.Fatalf("unexpected tag preflight: %v: %s", err, output)
			}
			_, waited := os.Stat(filepath.Join(root, "waited"))
			if scenario == "different main" && waited == nil {
				t.Fatal("tag preflight waited on CI for a different main commit")
			}
		})
	}
}

func TestStablePublicationFollowsSuccessfulDraftUploads(t *testing.T) {
	var script string
	for _, step := range sourceWorkflowSteps(t, "release") {
		if step.Run != "" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("missing release publication script")
	}
	for _, failUpload := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "upload failure"}[failUpload], func(t *testing.T) {
			root := t.TempDir()
			body := `#!/bin/sh
set -eu
case "$1 $2" in
  'release create')
    case " $* " in *' --draft '*) ;; *) exit 2 ;; esac
    printf '%s\n' draft >> "$TEST_ROOT/calls"
    test "$TEST_FAIL_UPLOAD" != 1
    ;;
  'release edit')
    case " $* " in *' --draft=false '*) ;; *) exit 2 ;; esac
    printf '%s\n' public >> "$TEST_ROOT/calls"
    ;;
  *) exit 2 ;;
esac
`
			if err := os.WriteFile(filepath.Join(root, "gh"), []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("sh", "-eu", "-c", script)
			command.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_ROOT="+root, "TEST_FAIL_UPLOAD="+map[bool]string{false: "0", true: "1"}[failUpload], "RELEASE_TAG=v1.2.3", "GH_TOKEN=")
			output, err := command.CombinedOutput()
			if (err != nil) != failUpload {
				t.Fatalf("unexpected publication status: %v: %s", err, output)
			}
			calls, err := os.ReadFile(filepath.Join(root, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			expected := "draft\npublic\n"
			if failUpload {
				expected = "draft\n"
			}
			if string(calls) != expected {
				t.Fatalf("publication order: %s", calls)
			}
		})
	}
}

package pmtool

import (
	"strings"
	"testing"
)

func TestHostWorkflowStopsBeforeLaterStagesOnFailure(t *testing.T) {
	a, d, output := testApp(t)
	d.results = []int{0, 19}
	if code := a.run([]string{"--image", "tooling:local", "check"}); code != 19 || len(d.plans) != 2 {
		t.Fatal("check lost the failure code or continued after failure")
	}
	if !strings.Contains(output.String(), "[FAIL]") || strings.Contains(output.String(), "[OK] check completed") {
		t.Fatal("failure reported success")
	}
	for _, p := range d.plans {
		if strings.Contains(strings.Join(p.Args, " "), "docker.sock") {
			t.Fatal("socket forwarded")
		}
	}
}

func TestHostReleasePreviewDoesNotRequireDocker(t *testing.T) {
	a, d, output := testApp(t)
	if a.run([]string{"--dry-run", "release-submit"}) != 0 || len(d.calls) != 0 || len(d.plans) != 0 {
		t.Fatal("host release still requires a container")
	}
	if !strings.Contains(output.String(), `"executor": "host"`) {
		t.Fatal(output.String())
	}
}

func TestAllWorkflowDocumentationIsLocal(t *testing.T) {
	for _, name := range []string{"usage", "onboarding", "ai", "ci", "templates"} {
		a, d, output := testApp(t)
		if a.run([]string{"docs", name}) != 0 || output.Len() == 0 || len(d.calls) != 0 {
			t.Fatalf("docs %s require Docker", name)
		}
	}
}

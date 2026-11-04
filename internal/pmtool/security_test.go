package pmtool

import (
	"strings"
	"testing"
)

func TestLauncherOwnsCommandPermissions(t *testing.T) {
	for command, permission := range launcherManifest().Commands {
		if !completionName.MatchString(command) || permission.Socket {
			t.Fatalf("unsafe launcher capability: %q", command)
		}
	}
	for _, option := range launcherManifest().Options {
		if !completionName.MatchString(option) {
			t.Fatalf("unsafe option: %q", option)
		}
	}
}

func TestToolStagesDoNotReceiveCredentialsOrSocket(t *testing.T) {
	a, _, _ := testApp(t)
	a.getenv = func(string) string { return "private-token" }
	p := a.toolPlan(config{Docker: "docker"}, workflowStep{Command: []string{"docker", "run", "--rm", "tool-image", "validate"}}, false)
	if len(p.Input) != 0 || len(p.Environment) != 0 || strings.Contains(strings.Join(p.Args, " "), "private-token") {
		t.Fatal("external validator inherited workflow source or secrets")
	}
	expectFailure(t, "Docker socket", func() {
		a.toolPlan(config{Docker: "docker"}, workflowStep{Command: []string{"docker", "run", "--volume", "/var/run/docker.sock:/var/run/docker.sock"}}, false)
	})
}

package hostflow

import (
	"strings"
	"testing"
)

func TestRequiredInputNamesAreAggregatedBeforeActions(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		e := &Engine{Project: Project{Provider: provider}, Git: &Git{}}
		err := e.ValidateInputs(Options{Command: "release-prepare"}, true, false)
		token := "GITLAB_TOKEN"
		if provider == "github" {
			token = "GITHUB_TOKEN"
		}
		if err == nil || !strings.Contains(err.Error(), "--version") || !strings.Contains(err.Error(), token) {
			t.Fatal(err)
		}
		if err := e.ValidateInputs(Options{Command: "status"}, false, false); err != nil {
			t.Fatal("local read required credentials", err)
		}
	}
}
func TestRequiredBranchAndInvalidTokenPreflight(t *testing.T) {
	e := &Engine{Git: &Git{Token: "token-not-printed\n"}}
	err := e.ValidateInputs(Options{Command: "policy"}, true, false)
	if err == nil || strings.Contains(err.Error(), "token-not-printed") {
		t.Fatal("bad token accepted or disclosed", err)
	}
	for _, command := range []string{"bootstrap-prepare", "cleanup", "cleanup-noop"} {
		if err := e.ValidateInputs(Options{Command: command}, false, false); err == nil || !strings.Contains(err.Error(), "--branch") {
			t.Fatal(err)
		}
	}
}

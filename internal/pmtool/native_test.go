package pmtool

import (
	"reflect"
	"testing"
)

func TestNativePDKAndVoxBoxReusePlannedArguments(t *testing.T) {
	cases := []struct {
		tool                string
		arguments, expected []string
		executable          string
	}{
		{"pdk", []string{"validate"}, []string{"validate"}, "pdk"},
		{"pdk", []string{"test", "unit"}, []string{"test", "unit"}, "pdk"},
		{"voxbox", []string{"spec"}, []string{"exec", "rake", "-f", "/opt/voxbox/Rakefile", "spec"}, "bundle"},
	}
	for _, test := range cases {
		command := append([]string{"docker", "run", "--rm", "--volume", "/clone:/repo", "--workdir", "/repo", "tool-image"}, test.arguments...)
		p := nativeStep(workflowStep{Command: command, Tool: test.tool})
		if p.Docker != test.executable || !reflect.DeepEqual(p.Args, test.expected) || len(p.Input) != 0 {
			t.Fatal("native command differs from planned validation")
		}
	}
}

func TestNativeExecutorDoesNotEnableRemoteMutations(t *testing.T) {
	a, d, _ := testApp(t)
	expectFailure(t, "validation commands only", func() { a.run([]string{"--executor", "native", "release-tag"}) })
	if len(d.calls) != 0 || len(d.plans) != 0 {
		t.Fatal("native release contacted a tool")
	}
}

func TestNativeWorkflowOptionsAreParsedInGo(t *testing.T) {
	a, _, _ := testApp(t)
	o := a.parseOperation([]string{"audit", "--ci=team/ci@v1", "--template", "git.example/template/repo@v2", "--provider", "github"})
	if o.Settings.CI != "team/ci@v1" || o.Settings.Template != "git.example/template/repo@v2" || o.Settings.Provider != "github" {
		t.Fatal(o)
	}
	expectFailure(t, "managed by the launcher", func() { a.parseOperation([]string{"audit", "--host-module=/other"}) })
}

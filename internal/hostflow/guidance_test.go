package hostflow

import (
	"strings"
	"testing"
	"time"
)

func TestMigratedGuidanceCommandTransitions(t *testing.T) {
	c := GuidanceContext{Branch: "release/1.2.3", Version: "1.2.3", Dirty: true}
	for command, next := range map[string]string{"release-prepare": "sync", "migrate": "sync", "sync-noop": "sync", "sync": "check", "main-sync": "check", "audit": "check", "pdk-convert-noop": "pdk-convert", "pdk-convert": "migrate", "pdk": "voxbox", "voxbox": "release-checks", "release-commit": "release-push", "release-submit": "mr", "release-push": "mr", "mr-create": "mr-merge", "mr-merge": "main-sync", "mr": "check", "policy-noop": "policy", "policy": "status", "policy-delete-master": "status", "bootstrap-commit": "bootstrap-push", "bootstrap-push": "mr", "bootstrap-commit-noop": "bootstrap-commit", "release-commit-noop": "release-commit", "release-push-noop": "release-push", "release-tag-noop": "release-tag", "bootstrap-push-noop": "bootstrap-push", "cleanup-noop": "cleanup"} {
		t.Run(command, func(t *testing.T) {
			_, args := Guidance(Options{Command: command, Branch: c.Branch}, "/module with spaces", c, true)
			if len(args) < 2 || args[1] != next {
				t.Fatal(args)
			}
			if args[2] != "--module" || args[3] != "/module with spaces" {
				t.Fatal("clone argument lost")
			}
		})
	}
}
func TestMigratedGuidanceAfterChecks(t *testing.T) {
	for _, command := range []string{"check", "release-checks"} {
		for _, kind := range []string{"release", "bootstrap", "main", "feature"} {
			for _, dirty := range []bool{false, true} {
				for _, pushed := range []bool{false, true} {
					t.Run(command+"/"+kind+map[bool]string{false: "/clean", true: "/dirty"}[dirty]+map[bool]string{false: "/local", true: "/pushed"}[pushed], func(t *testing.T) {
						branch := kind
						if kind == "release" || kind == "bootstrap" {
							branch += "/1.2.3"
						}
						message, args := Guidance(Options{Command: command}, "/module", GuidanceContext{Branch: branch, Version: "1.2.3", Dirty: dirty, Pushed: pushed}, true)
						want := ""
						switch kind {
						case "main":
							if dirty {
								if !strings.Contains(message, "pending main changes") {
									t.Fatal(message)
								}
							} else {
								want = "release-tag"
							}
						case "release", "bootstrap":
							want = kind + "-push"
							if dirty {
								want = "bootstrap-commit"
								if kind == "release" {
									want = "release-submit"
								}
							} else if pushed {
								want = "mr"
							}
						case "feature":
							if message == "" {
								t.Fatal("unknown branch got no review guidance")
							}
						}
						if want != "" && (len(args) < 2 || args[1] != want) {
							t.Fatal(message, args)
						}
						if want == "" && len(args) > 0 {
							t.Fatal("unsafe branch advances workflow", args)
						}
						if strings.Contains(strings.Join(args, " "), "-noop") {
							t.Fatal("optional preview became mandatory")
						}
					})
				}
			}
		}
	}
}
func TestMigratedGuidanceRecoveryAndCanary(t *testing.T) {
	c := GuidanceContext{Branch: "main", Version: "1.2.3"}
	message, args := Guidance(Options{Command: "release-tag"}, "/module", c, false)
	if !strings.Contains(message, "preserve") || !strings.Contains(strings.Join(args, " "), "pipeline-jobs --module /module --ref v1.2.3 --watch 5") {
		t.Fatal(message, args)
	}
	message, args = Guidance(Options{Command: "release-tag"}, "/module", c, true)
	if !strings.Contains(message, "forge-publish succeeded") || !strings.Contains(message, "canary") || args[1] != "cleanup-noop" {
		t.Fatal(message, args)
	}
	message, _ = Guidance(Options{Command: "cleanup"}, "/module", c, true)
	if !strings.Contains(message, "tags are preserved") {
		t.Fatal(message)
	}
	message, args = Guidance(Options{Command: "pipeline-jobs", Ref: "main"}, "/module", c, true)
	if !strings.HasPrefix(message, "If every required job succeeded") || args[1] != "release-tag" {
		t.Fatal(message, args)
	}
	_, args = Guidance(Options{Command: "pipeline-jobs", Ref: "main", Watch: time.Second}, "/module", GuidanceContext{Branch: "release/work"}, true)
	if len(args) > 0 {
		t.Fatal("non-main clone was told to tag")
	}
}
func TestMigratedGuidanceRetainsExplicitSettingsAndLiteralValues(t *testing.T) {
	o := Options{Command: "mr-create", Title: "O'Reilly $(never run)", Branch: "release/1.2.3", Settings: Settings{CI: "github.com/team/pipelines/.github/workflows/build.yml@v1", Template: "git.example/team/template@v1", Provider: "github"}, CIProject: "legacy/ci"}
	_, args := Guidance(o, "/module with spaces", GuidanceContext{Branch: "main"}, false)
	all := strings.Join(args, "\x00")
	for _, value := range []string{o.Title, o.Branch, o.Settings.CI, o.Settings.Template, o.Settings.Provider, "/module with spaces"} {
		if !strings.Contains(all, value) {
			t.Fatal(value, args)
		}
	}
	if strings.Contains(all, "--ci-project") {
		t.Fatal("explicit CI duplicated by legacy setting")
	}
	o.Settings.CI = ""
	_, args = Guidance(o, "/module", GuidanceContext{}, false)
	if !strings.Contains(strings.Join(args, " "), "--ci-project legacy/ci") {
		t.Fatal(args)
	}
	o.Command = "release-commit-noop"
	o.Files = "metadata.json CHANGELOG.md"
	_, args = Guidance(o, "/module", GuidanceContext{}, true)
	if args[len(args)-1] != o.Files {
		t.Fatal("file selection split/lost", args)
	}
}
func TestMigratedGuidanceLoadsCloneWithoutChanges(t *testing.T) {
	e, _, _, remote := fixture(t)
	before := snapshotRepository(t, e, remote)
	c := e.guidanceContext()
	if c.Branch != "main" || c.Version != "1.0.0" || c.Dirty || !c.Pushed {
		t.Fatal(c)
	}
	unchangedRepository(t, e, remote, before)
	prepareCommit(t, e, false)
	before = snapshotRepository(t, e, remote)
	c = e.guidanceContext()
	if !c.Dirty || c.Pushed || c.Branch != "release/1.0.1" {
		t.Fatal(c)
	}
	unchangedRepository(t, e, remote, before)
}

func TestMigratedPartialSuccessGuidanceDoesNotReplayEarlierMutation(t *testing.T) {
	e, _, _, _ := fixture(t)
	for _, recovery := range []string{"release-push", "main-sync", "pipeline-jobs"} {
		e.Recovery = recovery
		e.RecoveryRef = ""
		if recovery == "pipeline-jobs" {
			e.RecoveryRef = "v1.0.0"
		}
		_, args := e.Advice(Options{Command: "release-submit", Files: "metadata.json CHANGELOG.md"}, false)
		if len(args) < 2 || args[1] != recovery {
			t.Fatal(args)
		}
		if strings.Contains(strings.Join(args, " "), "--files") {
			t.Fatal("recovery repeated commit selection", args)
		}
	}
}

package hostflow

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMigratedCleanupRefusalsPreserveRefs(t *testing.T) {
	for scenario := range strings.FieldsSeq("dirty unfinished checked_out main_changed open_mr missing_mr wrong_tip disagreement missing_tag lightweight unmerged failed_pipeline missing_pipeline wrong_pipeline_sha failed_publish protected symbolic_tracking missing_token invalid_branch wrong_version api_error") {
		t.Run(scenario, func(t *testing.T) {
			e, api, remote := cleanupFixture(t)
			head := command(t, e.Project.Root, "rev-parse", "HEAD")
			branch := "release/1.0.0"
			switch scenario {
			case "dirty":
				put(t, e.Project.Root, "dirty", "keep")
			case "unfinished":
				put(t, e.Project.Root, ".git/BISECT_START", head)
			case "checked_out":
				command(t, e.Project.Root, "switch", branch)
			case "main_changed":
				command(t, e.Project.Root, "commit", "--allow-empty", "-m", "unpublished main")
			case "open_mr":
				api.requests = func(state, _, _ string) ([]Request, error) {
					if state == "opened" {
						return []Request{{ID: 3}}, nil
					}
					return nil, nil
				}
			case "missing_mr":
				api.requests = func(string, string, string) ([]Request, error) { return nil, nil }
			case "wrong_tip", "unmerged":
				api.requests = func(state, source, target string) ([]Request, error) {
					if state == "merged" {
						r := Request{SHA: head, MergeSHA: head, Source: source, Target: target}
						if scenario == "wrong_tip" {
							r.SHA = strings.Repeat("f", 40)
						} else {
							r.MergeSHA = strings.Repeat("f", 40)
						}
						return []Request{r}, nil
					}
					return nil, nil
				}
			case "disagreement":
				command(t, e.Project.Root, "commit", "--allow-empty", "-m", "other")
				command(t, e.Project.Root, "branch", "-f", branch, "HEAD")
				command(t, e.Project.Root, "reset", "--hard", head)
			case "missing_tag":
				command(t, remote, "update-ref", "-d", "refs/tags/v1.0.0")
			case "lightweight":
				command(t, remote, "update-ref", "refs/tags/v1.0.0", head)
			case "failed_pipeline", "missing_pipeline", "wrong_pipeline_sha":
				api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
					if scenario == "missing_pipeline" {
						return nil, nil
					}
					p := Pipeline{ID: 8, Ref: q.Ref, SHA: q.SHA, Status: "success"}
					if scenario == "failed_pipeline" {
						p.Status = "running"
					} else {
						p.SHA = "wrong"
					}
					return []Pipeline{p}, nil
				}
			case "failed_publish":
				api.jobs = func(int64) ([]Job, error) { return []Job{{Name: "forge-publish", Status: "failed"}}, nil }
			case "protected":
				api.branch = func(name string) (Branch, error) { return Branch{Name: name, SHA: head, Protected: true}, nil }
			case "symbolic_tracking":
				command(t, e.Project.Root, "symbolic-ref", "refs/remotes/origin/"+branch, "refs/heads/main")
			case "missing_token":
				e.Git.Token = ""
			case "invalid_branch":
				branch = "main"
			case "wrong_version":
				command(t, e.Project.Root, "branch", "-m", "release/1.0.0", "release/1.0.1")
				command(t, e.Project.Root, "push", "origin", ":refs/heads/release/1.0.0", "refs/heads/release/1.0.1")
				branch = "release/1.0.1"
			case "api_error":
				api.requests = func(string, string, string) ([]Request, error) { return nil, errFixtureAPI }
			}
			before := snapshotForDetached(t, e, remote)
			for _, noop := range []bool{true, false} {
				if err := e.cleanup(Options{Branch: branch, Noop: noop}); err == nil {
					t.Fatal("unsafe cleanup accepted")
				}
				after := snapshotForDetached(t, e, remote)
				if !reflect.DeepEqual(before, after) {
					t.Fatal("refused cleanup removed or changed refs")
				}
			}
		})
	}
}
func TestMigratedCleanupStaleRefsAndBootstrap(t *testing.T) {
	for _, bootstrap := range []bool{false, true} {
		t.Run(map[bool]string{false: "release", true: "bootstrap"}[bootstrap], func(t *testing.T) {
			e, api, remote := cleanupFixture(t)
			branch := "release/1.0.0"
			head := command(t, e.Project.Root, "rev-parse", "HEAD")
			if bootstrap {
				command(t, e.Project.Root, "branch", "-m", branch, "bootstrap/contracts")
				branch = "bootstrap/contracts"
				command(t, e.Project.Root, "push", "origin", ":refs/heads/release/1.0.0", "refs/heads/"+branch)
			}
			command(t, e.Project.Root, "branch", "unrelated")
			command(t, remote, "branch", "unrelated", "main")
			command(t, remote, "update-ref", "-d", "refs/heads/"+branch)
			api.branch = func(string) (Branch, error) { return Branch{}, ErrNotFound }
			api.requests = func(state, source, target string) ([]Request, error) {
				if state == "merged" {
					return []Request{{ID: 9, SHA: head, MergeSHA: head, Source: source, Target: target}}, nil
				}
				return nil, nil
			}
			tag := command(t, remote, "rev-parse", "v1.0.0")
			if err := e.cleanup(Options{Branch: branch}); err != nil {
				t.Fatal(err)
			}
			if err := e.cleanup(Options{Branch: branch}); err != nil {
				t.Fatal(err)
			}
			if command(t, remote, "rev-parse", "v1.0.0") != tag || command(t, e.Project.Root, "rev-parse", "unrelated") != head || command(t, remote, "rev-parse", "unrelated") != head {
				t.Fatal("cleanup touched tags/unrelated branches")
			}
		})
	}
}
func TestMigratedCleanupLocalRaceAndRemoteLease(t *testing.T) {
	for _, kind := range []string{"local race", "remote lease"} {
		t.Run(kind, func(t *testing.T) {
			e, api, remote := cleanupFixture(t)
			head := command(t, e.Project.Root, "rev-parse", "HEAD")
			command(t, e.Project.Root, "switch", "-c", "new-tip")
			command(t, e.Project.Root, "commit", "--allow-empty", "-m", "new source")
			tip := command(t, e.Project.Root, "rev-parse", "HEAD")
			command(t, e.Project.Root, "switch", "main")
			command(t, e.Project.Root, "push", "origin", "refs/heads/new-tip")
			jobs := 0
			api.jobs = func(int64) ([]Job, error) {
				jobs++
				if kind == "local race" && jobs == 2 {
					command(t, e.Project.Root, "update-ref", "refs/heads/release/1.0.0", tip)
				}
				return []Job{{Name: "forge-publish", Status: "success"}}, nil
			}
			if kind == "remote lease" {
				e.Git.OnCommand = func(args []string) {
					if len(args) > 0 && args[0] == "push" {
						command(t, remote, "update-ref", "refs/heads/release/1.0.0", tip)
					}
				}
			}
			if err := e.cleanup(Options{Branch: "release/1.0.0"}); err == nil {
				t.Fatal("concurrent branch change was deleted")
			}
			expected := head
			if kind == "local race" {
				expected = tip
			}
			if command(t, e.Project.Root, "rev-parse", "refs/heads/release/1.0.0") != expected {
				t.Fatal("failed cleanup removed local branch")
			}
			if command(t, remote, "show-ref", "--verify", "refs/heads/release/1.0.0") == "" {
				t.Fatal("failed lease deleted remote branch")
			}
		})
	}
}
func TestMigratedCleanupRejectsOtherWorktree(t *testing.T) {
	e, _, remote := cleanupFixture(t)
	command(t, e.Project.Root, "worktree", "add", filepath.Join(t.TempDir(), "other"), "release/1.0.0")
	before := snapshotRepository(t, e, remote)
	expectError(t, e.cleanup(Options{Branch: "release/1.0.0"}), "worktree")
	unchangedRepository(t, e, remote, before)
}

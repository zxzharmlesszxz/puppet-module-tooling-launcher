package hostflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var errFixtureAPI = errors.New("fixture API unavailable")

func publicationFixture(t *testing.T) (*Engine, *fakeHosting, string, string, string) {
	t.Helper()
	e, api, _, remote := fixture(t)
	command(t, e.Project.Root, "tag", "-a", "v1.0.0", "-m", "tag")
	command(t, e.Project.Root, "push", "origin", "refs/tags/v1.0.0")
	return e, api, remote, command(t, e.Project.Root, "rev-parse", "HEAD"), command(t, e.Project.Root, "rev-parse", "v1.0.0")
}
func TestMigratedPublicationStateMachine(t *testing.T) {
	statuses := strings.Fields("created waiting_for_resource preparing pending running scheduled success failed canceled skipped manual unknown missing wrong_sha wrong_ref wrong_id disappeared changed_tag changed_local_tag api_error interrupted missing_job failed_job paginated duplicate_job")
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			e, api, remote, head, object := publicationFixture(t)
			sleeps, lists, details := 0, 0, 0
			api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
				lists++
				if q.Ref != "v1.0.0" || q.SHA != head || q.Source != "push" {
					t.Fatal(q)
				}
				if status == "missing" || lists < 3 {
					return nil, nil
				}
				p := Pipeline{ID: 60, Ref: q.Ref, SHA: q.SHA, Status: "success"}
				if status == "success" || status == "paginated" {
					return []Pipeline{p}, nil
				}
				switch status {
				case "wrong_sha":
					p.SHA = "other"
				case "wrong_ref":
					p.Ref = "other"
				default:
					p.Status = "running"
				}
				return []Pipeline{p}, nil
			}
			api.pipeline = func(id int64) (Pipeline, error) {
				details++
				if id != 60 {
					t.Fatal("pipeline ID switched")
				}
				p := Pipeline{ID: id, Ref: "v1.0.0", SHA: head, Status: status}
				switch status {
				case "wrong_id":
					p.ID = 61
				case "disappeared":
					return Pipeline{}, ErrNotFound
				case "api_error":
					return Pipeline{}, errFixtureAPI
				case "interrupted":
					return Pipeline{}, context.Canceled
				case "missing_job", "failed_job", "duplicate_job", "changed_tag", "changed_local_tag":
					p.Status = "success"
				default:
					if active(status) {
						p.Status = "success"
					}
				}
				return p, nil
			}
			api.jobs = func(int64) ([]Job, error) {
				switch status {
				case "missing_job":
					return nil, nil
				case "failed_job":
					return []Job{{Name: "forge-publish", Status: "failed"}}, nil
				case "duplicate_job":
					return []Job{{Name: "forge-publish", Status: "success"}, {Name: "forge-publish", Status: "success"}}, nil
				}
				return []Job{{Name: "forge-publish", Status: "success"}}, nil
			}
			e.Sleep = func(_ context.Context, delay time.Duration) error {
				if delay != 5*time.Second {
					t.Fatal(delay)
				}
				sleeps++
				if sleeps == 3 && (status == "changed_tag" || status == "changed_local_tag") {
					command(t, e.Project.Root, "tag", "-a", "replacement", "-m", "replacement")
					replacement := command(t, e.Project.Root, "rev-parse", "replacement")
					if status == "changed_tag" {
						command(t, e.Project.Root, "push", "--force", "origin", "refs/tags/replacement:refs/tags/v1.0.0")
					} else {
						command(t, e.Project.Root, "update-ref", "refs/tags/v1.0.0", replacement)
					}
				}
				return nil
			}
			err := e.waitPublication("v1.0.0", head, object)
			if active(status) || status == "success" || status == "paginated" {
				if err != nil {
					t.Fatal(err)
				}
				expected := 2
				if active(status) {
					expected = 3
				}
				if sleeps != expected {
					t.Fatal("wrong creation/running wait count", sleeps)
				}
			} else if err == nil {
				t.Fatal("invalid publication accepted")
			}
			if status == "missing" && sleeps != 12 {
				t.Fatal("missing pipeline wait is not bounded at 60 seconds", sleeps)
			}
			if details > 0 && lists != 3 {
				t.Fatal("publication switched back to a pipeline list")
			}
			if command(t, remote, "tag", "--list") != "v1.0.0" {
				t.Fatal("publication removed remote tag")
			}
		})
	}
}
func TestMigratedMainWaitRejectsChangesAndLateUnrelatedCI(t *testing.T) {
	for _, noop := range []bool{true, false} {
		for _, kind := range []string{"branch", "mixed active", "matching active", "local tag", "remote tag", "interrupt", "late CI"} {
			if kind == "late CI" && noop {
				continue
			}
			t.Run(map[bool]string{true: "preview", false: "apply"}[noop]+"/"+kind, func(t *testing.T) {
				e, api, _, remote := fixture(t)
				head := command(t, e.Project.Root, "rev-parse", "HEAD")
				waiting := true
				sleeps := 0
				api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
					if q.Status != "" {
						if kind == "mixed active" {
							if q.Status == "created" {
								return []Pipeline{{ID: 1, Ref: "main", SHA: head, Status: "created"}}, nil
							}
							if q.Status == "running" {
								return []Pipeline{{ID: 2, Ref: "other", SHA: "other", Status: "running"}}, nil
							}
						}
						if kind == "matching active" && waiting && q.Status == "running" {
							return []Pipeline{{ID: 1, Ref: "main", SHA: head, Status: "running"}}, nil
						}
						if kind == "late CI" {
							if _, ok := e.Git.Optional("rev-parse", "--verify", "refs/tags/v1.0.0"); ok {
								return []Pipeline{{ID: 2, Ref: "other", SHA: "other", Status: q.Status}}, nil
							}
						}
						return nil, nil
					}
					p := Pipeline{ID: 3, Ref: q.Ref, SHA: head, Status: "success"}
					if waiting && kind != "late CI" {
						p.Status = "running"
					}
					return []Pipeline{p}, nil
				}
				e.Sleep = func(_ context.Context, _ time.Duration) error {
					sleeps++
					waiting = false
					switch kind {
					case "branch":
						command(t, e.Project.Root, "switch", "-c", "other")
					case "local tag":
						command(t, e.Project.Root, "tag", "-a", "v1.0.0", "-m", "concurrent")
					case "remote tag":
						command(t, e.Project.Root, "push", "origin", "HEAD:refs/tags/v1.0.0")
					case "interrupt":
						return context.Canceled
					}
					return nil
				}
				err := e.tag(Options{Noop: noop})
				if kind == "matching active" {
					if err != nil || sleeps != 1 {
						t.Fatal(err, sleeps)
					}
					return
				}
				if err == nil {
					t.Fatal("changed state accepted")
				}
				if kind == "late CI" && command(t, e.Project.Root, "tag", "--list") != "v1.0.0" {
					t.Fatal("late CI failure lost local tag")
				}
				if kind != "remote tag" && command(t, remote, "tag", "--list") != "" {
					t.Fatal("tag pushed despite wait failure")
				}
				if kind == "mixed active" && sleeps != 0 {
					t.Fatal("unrelated active pipeline was waited for")
				}
			})
		}
	}
}
func TestMigratedMergeWaitMatrixBothCommands(t *testing.T) {
	for _, cmd := range []string{"mr", "mr-merge"} {
		for _, status := range strings.Fields("success failed canceled skipped manual missing changed api_error interrupted") {
			t.Run(cmd+"/"+status, func(t *testing.T) {
				e, api, _, remote := fixture(t)
				command(t, e.Project.Root, "switch", "-c", "release/1.0.1")
				head := command(t, e.Project.Root, "rev-parse", "HEAD")
				r := Request{ID: 7, State: "opened", Source: "release/1.0.1", Target: "main", SHA: head}
				api.requests = func(string, string, string) ([]Request, error) { return []Request{r}, nil }
				api.request = func(int64) (Request, error) { return r, nil }
				polls, sleeps, merged := 0, 0, 0
				api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
					if q.Ref != r.Source || q.SHA != head || q.Source != "push" {
						t.Fatal(q)
					}
					polls++
					if polls == 1 {
						return []Pipeline{{ID: 42, Ref: q.Ref, SHA: head, Status: "running"}}, nil
					}
					if status == "api_error" {
						return nil, errFixtureAPI
					}
					if status == "missing" {
						return nil, nil
					}
					return []Pipeline{{ID: 42, Ref: q.Ref, SHA: head, Status: status}}, nil
				}
				e.Sleep = func(context.Context, time.Duration) error {
					sleeps++
					if status == "changed" {
						r.SHA = "changed"
					}
					if status == "interrupted" {
						return context.Canceled
					}
					return nil
				}
				api.merge = func(id int64, pin string) (Request, error) {
					if id != 7 || pin != head {
						t.Fatal("merge did not pin reviewed head")
					}
					merged++
					r.State = "merged"
					return r, nil
				}
				before := command(t, remote, "for-each-ref")
				err := e.Run(Options{Command: cmd})
				if status == "success" {
					if err != nil || merged != 1 {
						t.Fatal(err, merged)
					}
					if cmd == "mr" && command(t, e.Project.Root, "branch", "--show-current") != "main" {
						t.Fatal("combined mr did not synchronize main")
					}
				} else if err == nil || merged != 0 {
					t.Fatal("unsafe MR was merged", err, merged)
				}
				if sleeps != 1 {
					t.Fatal("merge wait cadence changed")
				}
				if command(t, remote, "for-each-ref") != before {
					t.Fatal("Git refs changed outside fake merge")
				}
			})
		}
	}
}
func TestMigratedMergeCreationReuseAndFailures(t *testing.T) {
	for _, kind := range []string{"new", "existing", "ambiguous", "create failure"} {
		t.Run(kind, func(t *testing.T) {
			e, api, _, _ := fixture(t)
			command(t, e.Project.Root, "switch", "-c", "release/1.0.1")
			head := command(t, e.Project.Root, "rev-parse", "HEAD")
			r := Request{ID: 7, State: "opened", Source: "release/1.0.1", Target: "main", SHA: head}
			creates, merges := 0, 0
			api.requests = func(string, string, string) ([]Request, error) {
				switch kind {
				case "existing":
					return []Request{r}, nil
				case "ambiguous":
					return []Request{r, r}, nil
				}
				return nil, nil
			}
			api.create = func(title, branch string) (Request, error) {
				creates++
				if title != "O'Reilly $(never execute)" || branch != r.Source {
					t.Fatal("literal title/branch lost")
				}
				if kind == "create failure" {
					return Request{}, errFixtureAPI
				}
				return r, nil
			}
			api.request = func(int64) (Request, error) { return r, nil }
			api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
				return []Pipeline{{ID: 42, Ref: q.Ref, SHA: head, Status: "success"}}, nil
			}
			api.merge = func(int64, string) (Request, error) { merges++; r.State = "merged"; return r, nil }
			err := e.Run(Options{Command: "mr", Title: "O'Reilly $(never execute)"})
			if kind == "new" || kind == "existing" {
				if err != nil || merges != 1 {
					t.Fatal(err, merges)
				}
				expected := 1
				if kind == "existing" {
					expected = 0
				}
				if creates != expected {
					t.Fatal("request creation not idempotent", creates)
				}
			} else if err == nil || merges != 0 {
				t.Fatal("lookup/create failure merged a request")
			}
		})
	}
}

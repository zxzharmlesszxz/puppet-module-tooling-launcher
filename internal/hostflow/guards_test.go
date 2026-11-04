package hostflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBootstrapAndMissingMainSynchronization(t *testing.T) {
	e, _, _, remote := fixture(t)
	command(t, e.Project.Root, "switch", "-c", "legacy")
	command(t, e.Project.Root, "branch", "-D", "main")
	if err := e.prepare(Options{Branch: "bootstrap/contracts"}, true); err != nil {
		t.Fatal(err)
	}
	put(t, e.Project.Root, "bootstrap", "x")
	if err := e.commit(Options{}, true); err != nil {
		t.Fatal(err)
	}
	if err := e.push(Options{}, true); err != nil {
		t.Fatal(err)
	}
	if command(t, remote, "rev-parse", "refs/heads/bootstrap/contracts") != command(t, e.Project.Root, "rev-parse", "HEAD") {
		t.Fatal("bootstrap push differed")
	}
}
func TestPrepareVersionAndRefGuards(t *testing.T) {
	for _, version := range []string{"0.9.0", "1.0.0", "01.0.1", "1.2", "v1.0.1"} {
		t.Run(version, func(t *testing.T) {
			e, _, _, _ := fixture(t)
			if err := e.prepare(Options{Version: version}, false); err == nil {
				t.Fatal("invalid version accepted")
			}
			if command(t, e.Project.Root, "branch", "--show-current") != "main" {
				t.Fatal("branch changed on refusal")
			}
		})
	}
	for _, kind := range []string{"local branch", "remote branch", "local tag", "remote tag"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _, _ := fixture(t)
			switch kind {
			case "local branch":
				command(t, e.Project.Root, "branch", "release/1.0.1")
			case "remote branch":
				command(t, e.Project.Root, "push", "origin", "HEAD:refs/heads/release/1.0.1")
			case "local tag":
				command(t, e.Project.Root, "tag", "v1.0.1")
			case "remote tag":
				command(t, e.Project.Root, "tag", "v1.0.1")
				command(t, e.Project.Root, "push", "origin", "refs/tags/v1.0.1")
				command(t, e.Project.Root, "tag", "-d", "v1.0.1")
			}
			if err := e.prepare(Options{Version: "1.0.1"}, false); err == nil {
				t.Fatal("existing ref ignored")
			}
		})
	}
}
func TestTagLocalAndRemoteIdentityGuards(t *testing.T) {
	for _, kind := range []string{"lightweight", "wrong commit", "remote exists"} {
		t.Run(kind, func(t *testing.T) {
			e, api, _, _ := fixture(t)
			greenMain(e, api)
			switch kind {
			case "lightweight":
				command(t, e.Project.Root, "tag", "v1.0.0")
			case "wrong commit":
				command(t, e.Project.Root, "tag", "-a", "v1.0.0", "-m", "tag")
				put(t, e.Project.Root, "next", "x")
				command(t, e.Project.Root, "add", ".")
				command(t, e.Project.Root, "commit", "-m", "next")
				command(t, e.Project.Root, "push", "origin", "main")
			case "remote exists":
				command(t, e.Project.Root, "tag", "-a", "v1.0.0", "-m", "tag")
				command(t, e.Project.Root, "push", "origin", "refs/tags/v1.0.0")
			}
			if err := e.tag(Options{Noop: true}); err == nil {
				t.Fatal("tag identity guard bypassed")
			}
		})
	}
}
func TestTagWaitDetectsConcurrentChanges(t *testing.T) {
	for _, kind := range []string{"head", "origin", "tag", "remote"} {
		t.Run(kind, func(t *testing.T) {
			e, api, _, _ := fixture(t)
			sha := command(t, e.Project.Root, "rev-parse", "HEAD")
			waiting := true
			api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
				if q.Status != "" {
					return nil, nil
				}
				status := "success"
				if waiting {
					status = "running"
				}
				return []Pipeline{{ID: 1, Ref: "main", SHA: sha, Status: status}}, nil
			}
			e.Sleep = func(context.Context, time.Duration) error {
				waiting = false
				switch kind {
				case "head":
					command(t, e.Project.Root, "commit", "--allow-empty", "-m", "changed")
				case "origin":
					command(t, e.Project.Root, "remote", "set-url", "origin", "https://other.invalid/x/puppet-sample.git")
				case "tag":
					command(t, e.Project.Root, "tag", "-a", "v1.0.0", "-m", "race")
				case "remote":
					command(t, e.Project.Root, "commit", "--allow-empty", "-m", "changed")
					command(t, e.Project.Root, "push", "origin", "main")
					command(t, e.Project.Root, "reset", "--hard", sha)
				}
				return nil
			}
			if err := e.tag(Options{Noop: true}); err == nil {
				t.Fatal("concurrent mutation not detected")
			}
		})
	}
}
func cleanupFixture(t *testing.T) (*Engine, *fakeHosting, string) {
	t.Helper()
	e, api, _, remote := fixture(t)
	sha := command(t, e.Project.Root, "rev-parse", "HEAD")
	command(t, e.Project.Root, "branch", "release/1.0.0")
	command(t, e.Project.Root, "push", "origin", "refs/heads/release/1.0.0")
	command(t, e.Project.Root, "tag", "-a", "v1.0.0", "-m", "tag")
	command(t, e.Project.Root, "push", "origin", "refs/tags/v1.0.0")
	greenMain(e, api)
	api.requests = func(state, source, target string) ([]Request, error) {
		if state == "merged" {
			return []Request{{ID: 1, SHA: sha, MergeSHA: sha, Source: source, Target: target, State: "merged"}}, nil
		}
		return nil, nil
	}
	api.branch = func(name string) (Branch, error) { return Branch{Name: name, SHA: sha}, nil }
	return e, api, remote
}
func TestCleanupGuardMatrix(t *testing.T) {
	for _, kind := range []string{"open", "protected", "unpublished", "missing job", "unmerged", "ref disagreement", "worktree", "unannotated", "missing tag"} {
		t.Run(kind, func(t *testing.T) {
			e, api, remote := cleanupFixture(t)
			sha := command(t, e.Project.Root, "rev-parse", "HEAD")
			switch kind {
			case "open":
				base := api.requests
				api.requests = func(state, source, target string) ([]Request, error) {
					if state == "opened" {
						return []Request{{ID: 2}}, nil
					}
					return base(state, source, target)
				}
			case "protected":
				api.branch = func(name string) (Branch, error) { return Branch{Name: name, SHA: sha, Protected: true}, nil }
			case "unpublished":
				api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
					return []Pipeline{{ID: 1, Ref: q.Ref, SHA: sha, Status: "failed"}}, nil
				}
			case "missing job":
				api.jobs = func(int64) ([]Job, error) { return nil, nil }
			case "unmerged":
				api.requests = func(string, string, string) ([]Request, error) { return nil, nil }
			case "ref disagreement":
				command(t, e.Project.Root, "commit", "--allow-empty", "-m", "next")
				command(t, e.Project.Root, "branch", "-f", "release/1.0.0", "HEAD")
				command(t, e.Project.Root, "reset", "--hard", sha)
			case "worktree":
				command(t, e.Project.Root, "worktree", "add", filepath.Join(t.TempDir(), "work"), "release/1.0.0")
			case "unannotated":
				command(t, remote, "update-ref", "refs/tags/v1.0.0", sha)
			case "missing tag":
				command(t, remote, "update-ref", "-d", "refs/tags/v1.0.0")
			}
			err := e.cleanup(Options{Branch: "release/1.0.0"})
			if err == nil {
				t.Fatal("unsafe cleanup accepted")
			}
			if command(t, remote, "show-ref", "--verify", "refs/heads/release/1.0.0") == "" {
				t.Fatal("guard deleted branch")
			}
		})
	}
}
func TestCleanupRevalidatesBeforeDeletion(t *testing.T) {
	e, api, remote := cleanupFixture(t)
	queries := 0
	base := api.jobs
	api.jobs = func(id int64) ([]Job, error) {
		queries++
		if queries == 2 {
			command(t, e.Project.Root, "tag", "-a", "other", "-m", "other")
			command(t, e.Project.Root, "push", "--force", "origin", "refs/tags/other:refs/tags/v1.0.0")
		}
		if base != nil {
			return base(id)
		}
		return []Job{{Name: "forge-publish", Status: "success"}}, nil
	}
	err := e.cleanup(Options{Branch: "release/1.0.0"})
	if err == nil {
		t.Fatal("tag race not detected")
	}
	if command(t, remote, "show-ref", "--verify", "refs/heads/release/1.0.0") == "" {
		t.Fatal("deleted branch after race")
	}
}
func TestPathsMetadataAndAuthorGuards(t *testing.T) {
	e, _, _, _ := fixture(t)
	if err := e.prepare(Options{Version: "1.0.1"}, false); err != nil {
		t.Fatal(err)
	}
	bump(t, e, "1.0.1")
	for _, path := range []string{"../metadata.json", "/metadata.json", ".git/config", "metadata.json/../CHANGELOG.md", "-bad"} {
		if _, _, err := e.selectedFiles(path); err == nil {
			t.Fatal(path)
		}
	}
	put(t, e.Project.Root, "metadata.json", `{"name":"wrong-bar-sample","version":"1.0.1"}`)
	if _, err := e.metadata(""); err == nil {
		t.Fatal("wrong module accepted")
	}
	bump(t, e, "1.0.1")
	put(t, e.Project.Root, "CHANGELOG.md", "no release")
	expectError(t, e.commit(Options{}, false), "missing Release")
	command(t, e.Project.Root, "config", "--unset", "user.name")
	command(t, e.Project.Root, "config", "--unset", "user.email")
	e.Getenv = func(string) string { return "" }
	if _, err := e.author(); err == nil {
		t.Fatal("missing author accepted")
	}
}
func TestGitOutputRedactsTokenAndRemovesAskpass(t *testing.T) {
	e, _, _, _ := fixture(t)
	var path string
	e.Git.OnCommand = func(args []string) { path = strings.Join(args, " ") }
	if _, err := e.Git.Run("status", "--short"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(path, e.Git.Token) {
		t.Fatal("token included in argv")
	}
	if e.Git.redact("error test-token") != "error [REDACTED]" {
		t.Fatal("token unredacted")
	}

}

func TestCanceledGitDoesNotStartAChild(t *testing.T) {
	e, _, _, _ := fixture(t)
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	e.Git.Context = ctx
	if _, err := e.Git.Run("status"); err == nil {
		t.Fatal("canceled Git was executed")
	}
}
func TestCommitRequiresChangelogDiffAgainstHEAD(t *testing.T) {
	e, _, _, _ := fixture(t)
	put(t, e.Project.Root, "CHANGELOG.md", "## Release 1.0.1\n")
	command(t, e.Project.Root, "add", ".")
	command(t, e.Project.Root, "commit", "-m", "future header")
	command(t, e.Project.Root, "push", "origin", "main")
	if err := e.prepare(Options{Version: "1.0.1"}, false); err != nil {
		t.Fatal(err)
	}
	bump(t, e, "1.0.1")
	put(t, e.Project.Root, "CHANGELOG.md", "## Release 1.0.1\ntransient staged text\n")
	command(t, e.Project.Root, "add", "CHANGELOG.md")
	put(t, e.Project.Root, "CHANGELOG.md", "## Release 1.0.1\n")
	expectError(t, e.commit(Options{Noop: true}, false), "must both change")
}

func TestNextPreservesCleanupReviewAndExplicitOptions(t *testing.T) {
	e, _, _, _ := fixture(t)
	for _, test := range []struct {
		options Options
		want    string
	}{{Options{Command: "release-tag"}, "cleanup-noop"}, {Options{Command: "mr-create", Branch: "release/1.0.1"}, "--branch release/1.0.1"}, {Options{Command: "release-commit-noop", Files: "metadata.json CHANGELOG.md"}, "--files metadata.json CHANGELOG.md"}} {
		got := strings.Join(e.Next(test.options, true), " ")
		if !strings.Contains(got, test.want) {
			t.Fatal(got)
		}
	}
}
func TestCommitPreviewRespectsExplicitColor(t *testing.T) {
	e, _, out, _ := fixture(t)
	if err := e.prepare(Options{Version: "1.0.1"}, false); err != nil {
		t.Fatal(err)
	}
	bump(t, e, "1.0.1")
	e.Color = "always"
	if err := e.commit(Options{Noop: true}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\x1b[") {
		t.Fatal("diff lost requested color")
	}
}

func TestCleanupBlocksOpenRequestsToAnyTarget(t *testing.T) {
	e, api, _ := cleanupFixture(t)
	api.requests = func(state, source, target string) ([]Request, error) {
		if state == "opened" {
			if target != "" {
				t.Fatal("open-request guard omitted other target branches")
			}
			return []Request{{ID: 3, Source: source, Target: "maintenance"}}, nil
		}
		return nil, nil
	}
	expectError(t, e.cleanup(Options{Branch: "release/1.0.0"}), "open merge requests")
}
func TestEverySuccessfulHostOperationHasGuidance(t *testing.T) {
	e, _, _, _ := fixture(t)
	for _, name := range commands {
		message, next := e.Advice(Options{Command: name}, true)
		if message == "" && len(next) == 0 {
			t.Fatal(name)
		}
	}
	next := strings.Join(e.Next(Options{Command: "pipeline-jobs", Ref: "main"}, true), " ")
	if !strings.Contains(next, "release-tag") {
		t.Fatal(next)
	}
}

package hostflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeHosting struct {
	jobLog    func(int64) ([]byte, error)
	pipelines func(PipelineQuery) ([]Pipeline, error)
	pipeline  func(int64) (Pipeline, error)
	jobs      func(int64) ([]Job, error)
	branch    func(string) (Branch, error)
	requests  func(string, string, string) ([]Request, error)
	request   func(int64) (Request, error)
	merge     func(int64, string) (Request, error)
	review    func(Request) (ReviewStatus, error)
	create    func(string, string) (Request, error)
}

func (f *fakeHosting) Pipelines(_ context.Context, q PipelineQuery) ([]Pipeline, error) {
	if f.pipelines != nil {
		return f.pipelines(q)
	}
	return nil, nil
}
func (f *fakeHosting) Pipeline(_ context.Context, id int64) (Pipeline, error) {
	if f.pipeline != nil {
		return f.pipeline(id)
	}
	return Pipeline{}, ErrNotFound
}
func (f *fakeHosting) Jobs(_ context.Context, id int64) ([]Job, error) {
	if f.jobs != nil {
		return f.jobs(id)
	}
	return []Job{{Name: "forge-publish", Status: "success"}}, nil
}
func (f *fakeHosting) JobLog(_ context.Context, id int64) ([]byte, error) {
	if f.jobLog != nil {
		return f.jobLog(id)
	}
	return nil, ErrNotFound
}
func (f *fakeHosting) Branch(_ context.Context, name string) (Branch, error) {
	if f.branch != nil {
		return f.branch(name)
	}
	return Branch{}, ErrNotFound
}
func (f *fakeHosting) Requests(_ context.Context, state, source, target string) ([]Request, error) {
	if f.requests != nil {
		return f.requests(state, source, target)
	}
	return nil, nil
}
func (f *fakeHosting) Request(_ context.Context, id int64) (Request, error) {
	if f.request != nil {
		return f.request(id)
	}
	return Request{}, ErrNotFound
}
func (f *fakeHosting) Review(_ context.Context, r Request) (ReviewStatus, error) {
	if f.review != nil {
		return f.review(r)
	}
	return ReviewStatus{SHA: r.SHA}, nil
}

func (f *fakeHosting) MergeRequest(_ context.Context, id int64, sha string) (Request, error) {
	if f.merge != nil {
		return f.merge(id, sha)
	}
	return Request{}, fmt.Errorf("unexpected merge")
}
func (f *fakeHosting) CreateRequest(_ context.Context, title, branch string) (Request, error) {
	if f.create != nil {
		return f.create(title, branch)
	}
	return Request{}, fmt.Errorf("unexpected request creation")
}
func (f *fakeHosting) Policy(context.Context, bool, bool, io.Writer, PolicyLevels) error { return nil }
func command(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}
func put(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) (*Engine, *fakeHosting, *bytes.Buffer, string) {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(t.TempDir(), "puppet-sample.git")
	command(t, root, "init", "--initial-branch=main")
	command(t, root, "config", "user.name", "Fixture")
	command(t, root, "config", "user.email", "fixture@example.invalid")
	command(t, root, "config", "commit.gpgsign", "false")
	command(t, root, "config", "tag.gpgsign", "false")
	put(t, root, "metadata.json", `{"name":"sample-sample","version":"1.0.0"}`)
	put(t, root, "CHANGELOG.md", "## Release 1.0.0\n")
	command(t, root, "add", ".")
	command(t, root, "commit", "-m", "initial")
	command(t, root, "init", "--bare", remote)
	command(t, root, "remote", "add", "origin", remote)
	command(t, root, "push", "--set-upstream", "origin", "main")
	api, out := &fakeHosting{}, &bytes.Buffer{}
	env := func(key string) string {
		switch key {
		case "GIT_AUTHOR_NAME":
			return "Fixture"
		case "GIT_AUTHOR_EMAIL":
			return "fixture@example.invalid"
		}
		return ""
	}
	e := &Engine{Project: Project{Root: root, Origin: remote, Remote: remote, Name: "puppet-sample", ID: "team/puppet-sample", Provider: "gitlab"}, Git: &Git{Root: root, Context: testContext(t), Token: "test-token", Getenv: env}, API: api, Context: testContext(t), Out: out, Getenv: env, Sleep: func(context.Context, time.Duration) error { return nil }}
	return e, api, out, remote
}
func expectError(t *testing.T, err error, part string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), part) {
		t.Fatalf("want error %q; got %v", part, err)
	}
}
func bump(t *testing.T, e *Engine, version string) {
	t.Helper()
	put(t, e.Project.Root, "metadata.json", `{"name":"sample-sample","version":"`+version+`"}`)
	put(t, e.Project.Root, "CHANGELOG.md", "## Release "+version+"\n")
}
func TestPrepareAndCommitGuards(t *testing.T) {
	e, _, out, _ := fixture(t)
	if err := e.prepare(Options{Version: "1.0.1"}, false); err != nil {
		t.Fatal(err)
	}
	if got := command(t, e.Project.Root, "branch", "--show-current"); got != "release/1.0.1" {
		t.Fatal(got)
	}
	bump(t, e, "1.0.1")
	put(t, e.Project.Root, "a file.txt", "hello\n")
	put(t, e.Project.Root, "-leading.txt", "literal path")
	put(t, e.Project.Root, "ключ.txt", "unicode")
	before := command(t, e.Project.Root, "rev-parse", "HEAD")
	index := command(t, e.Project.Root, "diff", "--cached", "--name-only")
	if err := e.commit(Options{Noop: true}, false); err != nil {
		t.Fatal(err)
	}
	if command(t, e.Project.Root, "rev-parse", "HEAD") != before || command(t, e.Project.Root, "diff", "--cached", "--name-only") != index {
		t.Fatal("preview mutated repository")
	}
	if !strings.Contains(out.String(), "a file.txt") {
		t.Fatal(out.String())
	}
	expectError(t, e.commit(Options{Files: "metadata.json CHANGELOG.md"}, false), "outside selected")
	if err := e.commit(Options{}, false); err != nil {
		t.Fatal(err)
	}
	expectError(t, e.commit(Options{}, false), "exceed HEAD")
}
func TestStagedDeletionAndPushPreview(t *testing.T) {
	e, _, _, remote := fixture(t)
	put(t, e.Project.Root, "remove.txt", "old")
	command(t, e.Project.Root, "add", ".")
	command(t, e.Project.Root, "commit", "-m", "file")
	command(t, e.Project.Root, "push", "origin", "main")
	if err := e.prepare(Options{Version: "1.0.1"}, false); err != nil {
		t.Fatal(err)
	}
	bump(t, e, "1.0.1")
	command(t, e.Project.Root, "rm", "remove.txt")
	if err := e.commit(Options{}, false); err != nil {
		t.Fatal(err)
	}
	if err := e.push(Options{Noop: true}, false); err != nil {
		t.Fatal(err)
	}
	if got := command(t, remote, "for-each-ref", "refs/heads/release/"); got != "" {
		t.Fatal("preview pushed a ref")
	}
	if err := e.push(Options{}, false); err != nil {
		t.Fatal(err)
	}
	if command(t, remote, "rev-parse", "refs/heads/release/1.0.1") != command(t, e.Project.Root, "rev-parse", "HEAD") {
		t.Fatal("wrong push SHA")
	}
}
func TestGitStateGuards(t *testing.T) {
	for _, kind := range []string{"dirty", "conflict", "wrong origin", "missing main", "merge"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _, _ := fixture(t)
			switch kind {
			case "dirty":
				put(t, e.Project.Root, "dirty", "x")
			case "conflict":
				put(t, e.Project.Root, ".git/MERGE_HEAD", strings.Repeat("0", 40))
			case "wrong origin":
				command(t, e.Project.Root, "remote", "set-url", "origin", "https://other.invalid/team/puppet-sample.git")
			case "missing main":
				command(t, e.Project.Root, "switch", "-c", "other")
				command(t, e.Project.Root, "branch", "-D", "main")
			case "merge":
				put(t, e.Project.Root, ".git/rebase-apply/next", "1")
			}
			if err := e.guard(true, true, false); err == nil {
				t.Fatal("unsafe repository accepted")
			}
		})
	}
}
func TestMainSyncRefusesDivergence(t *testing.T) {
	e, _, _, _ := fixture(t)
	put(t, e.Project.Root, "local", "x")
	command(t, e.Project.Root, "add", ".")
	command(t, e.Project.Root, "commit", "-m", "local")
	before := command(t, e.Project.Root, "rev-parse", "HEAD")
	expectError(t, e.syncMain(), "outside")
	if command(t, e.Project.Root, "rev-parse", "HEAD") != before {
		t.Fatal("rewrote divergent HEAD")
	}
}
func greenMain(e *Engine, f *fakeHosting) {
	f.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
		if q.Status != "" {
			return nil, nil
		}
		sha, _ := e.head()
		return []Pipeline{{ID: 1, Ref: q.Ref, SHA: sha, Status: "success"}}, nil
	}
}
func TestTagPublicationAndCleanup(t *testing.T) {
	e, api, _, remote := fixture(t)
	if err := e.prepare(Options{Version: "1.0.1"}, false); err != nil {
		t.Fatal(err)
	}
	bump(t, e, "1.0.1")
	if err := e.commit(Options{}, false); err != nil {
		t.Fatal(err)
	}
	if err := e.push(Options{}, false); err != nil {
		t.Fatal(err)
	}
	sha := command(t, e.Project.Root, "rev-parse", "HEAD")
	command(t, e.Project.Root, "switch", "main")
	command(t, e.Project.Root, "merge", "--ff-only", "release/1.0.1")
	command(t, e.Project.Root, "push", "origin", "main")
	greenMain(e, api)
	if err := e.tag(Options{Noop: true}); err != nil {
		t.Fatal(err)
	}
	if command(t, e.Project.Root, "tag", "--list") != "" {
		t.Fatal("tag preview created tag")
	}
	if err := e.tag(Options{}); err != nil {
		t.Fatal(err)
	}
	object := command(t, remote, "rev-parse", "v1.0.1")
	if object == sha {
		t.Fatal("tag was lightweight")
	}
	api.requests = func(state, source, target string) ([]Request, error) {
		if state == "merged" {
			return []Request{{ID: 1, SHA: sha, MergeSHA: sha, Source: source, Target: target, State: "merged"}}, nil
		}
		return nil, nil
	}
	api.branch = func(name string) (Branch, error) {
		refs, err := e.remoteRefs("refs/heads/" + name)
		if err != nil {
			return Branch{}, err
		}
		if refs["refs/heads/"+name] == "" {
			return Branch{}, ErrNotFound
		}
		return Branch{Name: name, SHA: sha}, nil
	}
	if err := e.cleanup(Options{Branch: "release/1.0.1", Noop: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.cleanup(Options{Branch: "release/1.0.1"}); err != nil {
		t.Fatal(err)
	}
	if command(t, remote, "rev-parse", "v1.0.1") != object {
		t.Fatal("cleanup moved tag")
	}
	if command(t, remote, "for-each-ref", "refs/heads/release/") != "" {
		t.Fatal("remote branch survived")
	}
	if err := e.cleanup(Options{Branch: "release/1.0.1"}); err != nil {
		t.Fatal(err)
	}
}
func TestTagRefusals(t *testing.T) {
	for _, status := range []string{"failed", "canceled", "skipped", "manual", "unknown", "missing", "other running", "changed head"} {
		t.Run(status, func(t *testing.T) {
			e, api, _, remote := fixture(t)
			greenMain(e, api)
			base := api.pipelines
			api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
				if status == "other running" && q.Status == "running" {
					return []Pipeline{{ID: 7, Ref: "release/other", SHA: "other", Status: "running"}}, nil
				}
				if q.Ref == "main" {
					if status == "missing" {
						return nil, nil
					}
					p, _ := base(q)
					p[0].Status = status
					if status == "changed head" {
						p[0].SHA = "other"
					}
					return p, nil
				}
				return base(q)
			}
			if err := e.tag(Options{}); err == nil {
				t.Fatal("unsafe tag accepted")
			}
			if got := command(t, remote, "tag", "--list"); got != "" {
				t.Fatal("tag pushed despite refusal")
			}
		})
	}
}
func TestTagWaitRevalidatesState(t *testing.T) {
	e, api, _, _ := fixture(t)
	sha := command(t, e.Project.Root, "rev-parse", "HEAD")
	api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
		if q.Status != "" {
			return nil, nil
		}
		return []Pipeline{{ID: 1, Ref: "main", SHA: sha, Status: "running"}}, nil
	}
	e.Sleep = func(context.Context, time.Duration) error { put(t, e.Project.Root, "mutation", "x"); return nil }
	expectError(t, e.tag(Options{}), "not clean")
}
func TestPublicationFailuresPreserveTag(t *testing.T) {
	for _, kind := range []string{"failed", "missing job", "duplicate job", "wrong sha", "disappears", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			e, api, _, remote := fixture(t)
			greenMain(e, api)
			base := api.pipelines
			api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
				p, err := base(q)
				if strings.HasPrefix(q.Ref, "v") {
					switch kind {
					case "failed":
						p[0].Status = "failed"
					case "wrong sha":
						p[0].SHA = "wrong"
					case "disappears", "cancel":
						p[0].Status = "running"
					}
				}
				return p, err
			}
			if kind == "missing job" {
				api.jobs = func(int64) ([]Job, error) { return nil, nil }
			}
			if kind == "duplicate job" {
				api.jobs = func(int64) ([]Job, error) {
					return []Job{{Name: "forge-publish", Status: "success"}, {Name: "forge-publish", Status: "success"}}, nil
				}
			}
			if kind == "cancel" {
				e.Sleep = func(context.Context, time.Duration) error { return context.Canceled }
			}
			if err := e.tag(Options{}); err == nil || !strings.Contains(err.Error(), "tag is preserved") {
				t.Fatalf("%v", err)
			}
			if command(t, remote, "tag", "--list") != "v1.0.0" {
				t.Fatal("publication failure removed tag")
			}
		})
	}
}
func TestMergeHeadChangeAndRecovery(t *testing.T) {
	e, api, _, _ := fixture(t)
	command(t, e.Project.Root, "switch", "-c", "release/1.0.1")
	sha := command(t, e.Project.Root, "rev-parse", "HEAD")
	r := Request{ID: 3, SHA: sha, Source: "release/1.0.1", Target: "main", State: "opened"}
	api.requests = func(string, string, string) ([]Request, error) { return []Request{r}, nil }
	api.request = func(int64) (Request, error) { return r, nil }
	api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
		return []Pipeline{{ID: 4, Ref: q.Ref, SHA: sha, Status: "running"}}, nil
	}
	e.Sleep = func(context.Context, time.Duration) error { r.SHA = "changed"; return nil }
	expectError(t, e.request(Options{}, false, true), "source pipeline identity changed")
	r.SHA = sha
	api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
		return []Pipeline{{ID: 4, Ref: q.Ref, SHA: sha, Status: "success"}}, nil
	}
	merged := false
	api.merge = func(_ int64, pin string) (Request, error) {
		if pin != sha {
			t.Fatal("merge SHA not pinned")
		}
		merged = true
		put(t, e.Project.Root, "dirty", "x")
		r.State = "merged"
		return r, nil
	}
	expectError(t, e.Run(Options{Command: "mr"}), "merge succeeded; retry main-sync")
	if !merged {
		t.Fatal("not merged")
	}
}
func TestCommitIsPreservedWhenPushFails(t *testing.T) {
	e, _, _, remote := fixture(t)
	if err := e.prepare(Options{Version: "1.0.1"}, false); err != nil {
		t.Fatal(err)
	}
	bump(t, e, "1.0.1")
	before := command(t, e.Project.Root, "rev-parse", "HEAD")
	put(t, remote, "hooks/pre-receive", "#!/bin/sh\nexit 1\n")
	if err := os.Chmod(filepath.Join(remote, "hooks/pre-receive"), 0700); err != nil {
		t.Fatal(err)
	}
	expectError(t, e.Run(Options{Command: "release-submit"}), "commit is preserved")
	if command(t, e.Project.Root, "rev-parse", "HEAD") == before {
		t.Fatal("commit was lost")
	}
}
func TestSettingsAndParsing(t *testing.T) {
	root := t.TempDir()
	put(t, root, ".sync.yml", "ci:\n  provider: gitlab\n  use: team/ci@v1\ntemplate:\n  use: team/template@v2\n")
	s, err := ReadSettings(root, Settings{Provider: "github"})
	if err != nil || s.Provider != "github" || s.CI != "team/ci@v1" {
		t.Fatal(s, err)
	}
	for _, content := range []string{"[]", "ci: nope", "ci: {use: 42}", "ci: {provider: nope}", "template: {bad: value}"} {
		put(t, root, ".sync.yml", content)
		if _, err := ReadSettings(root, Settings{}); err == nil {
			t.Fatal(content)
		}
	}
	for _, args := range [][]string{{"release-tag", "--host-module", "x"}, {"mr", "--watch", "-1"}, {"mr", "unexpected"}} {
		if _, err := Parse(args, root); err == nil {
			t.Fatal(args)
		}
	}
}
func TestProjectEndpointAndCredentialGuards(t *testing.T) {
	env := func(string) string { return "" }
	for _, origin := range []string{"https://user:secret@host/team/puppet-x.git", "https://host/team/puppet-x.git?token=secret", "https://host/team/../puppet-x", "https://host/team/x"} {
		if _, err := ResolveProject("/tmp", origin, Settings{}, env); err == nil {
			t.Fatal(origin)
		}
	}
	p, err := ResolveProject("/tmp", "ssh://git@host:777/team/puppet-x.git", Settings{}, env)
	if err != nil || p.Remote != "https://host/team/puppet-x.git" {
		t.Fatal(p, err)
	}
	_, err = ResolveProject("/tmp", "https://host/team/puppet-x.git", Settings{}, func(string) string { return "https://other/api/v4" })
	expectError(t, err, "origin host")
}
func TestCanceledWait(t *testing.T) {
	e, _, _, _ := fixture(t)
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	e.Context = ctx
	e.Sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	if !errors.Is(e.pause("wait", time.Second), context.Canceled) {
		t.Fatal("cancellation lost")
	}
}
func TestOptionsJSONDoesNotIncludeCredentials(t *testing.T) {
	o, _ := Parse([]string{"release-tag"}, t.TempDir())
	body, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("Token")) {
		t.Fatal(string(body))
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

func TestPipelineWatchPinsIdentityAndReportsFailures(t *testing.T) {
	for _, kind := range []string{"success", "changed", "failed"} {
		t.Run(kind, func(t *testing.T) {
			e, api, out, _ := fixture(t)
			api.branch = func(string) (Branch, error) { return Branch{SHA: "abc"}, nil }
			api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
				return []Pipeline{{ID: 7, Ref: q.Ref, SHA: q.SHA, Status: "running"}}, nil
			}
			api.pipeline = func(id int64) (Pipeline, error) {
				p := Pipeline{ID: id, Ref: "main", SHA: "abc", Status: "success"}
				switch kind {
				case "changed":
					p.ID = 8
				case "failed":
					p.Status = "failed"
				}
				return p, nil
			}
			err := e.Run(Options{Command: "pipeline-jobs", Ref: "main", Watch: time.Second})
			if kind == "success" {
				if err != nil || !strings.Contains(out.String(), "forge-publish") {
					t.Fatal(err, out)
				}
			} else if err == nil {
				t.Fatal("invalid pipeline accepted")
			}
		})
	}
}

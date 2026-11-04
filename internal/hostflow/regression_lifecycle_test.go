package hostflow

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type repositorySnapshot struct {
	Head, Branch, Refs, RemoteRefs string
	Index, Config                  []byte
}

func snapshotRepository(t *testing.T, e *Engine, remote string) repositorySnapshot {
	t.Helper()
	return snapshotRepositoryWithBranch(t, e, remote, command(t, e.Project.Root, "symbolic-ref", "HEAD"))
}
func snapshotRepositoryWithBranch(t *testing.T, e *Engine, remote, branch string) repositorySnapshot {
	t.Helper()
	s := repositorySnapshot{Head: command(t, e.Project.Root, "rev-parse", "HEAD"), Branch: branch, Refs: command(t, e.Project.Root, "for-each-ref"), RemoteRefs: command(t, remote, "for-each-ref")}
	var err error
	s.Index, err = os.ReadFile(filepath.Join(e.Project.Root, ".git/index"))
	if err != nil {
		t.Fatal(err)
	}
	s.Config, err = os.ReadFile(filepath.Join(e.Project.Root, ".git/config"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func unchangedRepository(t *testing.T, e *Engine, remote string, before repositorySnapshot) {
	t.Helper()
	after := snapshotRepository(t, e, remote)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("refused/preview operation mutated repository state\nbefore: %+v\nafter: %+v", before, after)
	}
}
func prepareCommit(t *testing.T, e *Engine, bootstrap bool) {
	t.Helper()
	o := Options{Version: "1.0.1", Branch: "bootstrap/contracts"}
	if err := e.prepare(o, bootstrap); err != nil {
		t.Fatal(err)
	}
	if bootstrap {
		put(t, e.Project.Root, "manifest.pp", "class example {}\n")
	} else {
		bump(t, e, "1.0.1")
	}
}
func TestMigratedCommitRefusalPreservesIndexAndHEAD(t *testing.T) {
	for _, bootstrap := range []bool{false, true} {
		for _, scenario := range []string{"wrong branch", "missing changelog", "staged outside", "untracked outside", "unsafe path", "missing file selection", "unfinished operation", "detached"} {
			t.Run(strings.Join([]string{map[bool]string{false: "release", true: "bootstrap"}[bootstrap], scenario}, "/"), func(t *testing.T) {
				e, _, _, remote := fixture(t)
				prepareCommit(t, e, bootstrap)
				files := "metadata.json CHANGELOG.md"
				if bootstrap {
					files = "manifest.pp"
				}
				switch scenario {
				case "wrong branch":
					command(t, e.Project.Root, "switch", "-c", "other")
				case "missing changelog":
					put(t, e.Project.Root, "CHANGELOG.md", "wrong release")
				case "staged outside", "untracked outside":
					put(t, e.Project.Root, "unrelated", "preserve")
					if scenario == "staged outside" {
						command(t, e.Project.Root, "add", "unrelated")
					}
				case "unsafe path":
					files += " ../escape"
				case "missing file selection":
					files = "absent-file"
				case "unfinished operation":
					put(t, e.Project.Root, ".git/CHERRY_PICK_HEAD", command(t, e.Project.Root, "rev-parse", "HEAD"))
				case "detached":
					command(t, e.Project.Root, "checkout", "--detach")
				}
				before := snapshotForDetached(t, e, remote)
				for _, noop := range []bool{true, false} {
					if err := e.commit(Options{Files: files, Noop: noop}, bootstrap); err == nil {
						t.Fatal("invalid commit accepted")
					}
					after := snapshotForDetached(t, e, remote)
					if !reflect.DeepEqual(before, after) {
						t.Fatal("failed commit changed refs/index/config")
					}
				}
			})
		}
	}
}
func snapshotForDetached(t *testing.T, e *Engine, remote string) repositorySnapshot {
	t.Helper()
	branch, exists := e.Git.Optional("symbolic-ref", "HEAD")
	if exists {
		return snapshotRepository(t, e, remote)
	}
	return snapshotRepositoryWithBranch(t, e, remote, branch)
}
func TestMigratedBootstrapCommitWithoutMainAndAutomaticSelection(t *testing.T) {
	e, _, _, remote := fixture(t)
	prepareCommit(t, e, true)
	command(t, e.Project.Root, "branch", "-D", "main")
	put(t, e.Project.Root, ".git/info/exclude", "ignored-file\n")
	put(t, e.Project.Root, "ignored-file", "preserve")
	put(t, e.Project.Root, "file with spaces", "include")
	before := snapshotRepository(t, e, remote)
	for range 2 {
		if err := e.commit(Options{Noop: true}, true); err != nil {
			t.Fatal(err)
		}
		unchangedRepository(t, e, remote, before)
	}
	if err := e.commit(Options{}, true); err != nil {
		t.Fatal(err)
	}
	files := command(t, e.Project.Root, "show", "--pretty=", "--name-only", "HEAD")
	if strings.Contains(files, "ignored-file") || !strings.Contains(files, "file with spaces") || !strings.Contains(files, "manifest.pp") {
		t.Fatal(files)
	}
	if command(t, e.Project.Root, "log", "-1", "--format=%s") != "[UPD] Bootstrap module 1.0.0" {
		t.Fatal("wrong bootstrap message")
	}
	if command(t, remote, "for-each-ref") != before.RemoteRefs {
		t.Fatal("commit published refs")
	}
}
func TestMigratedPushGuardsAndExactRefPublication(t *testing.T) {
	for _, bootstrap := range []bool{false, true} {
		t.Run(map[bool]string{false: "release", true: "bootstrap"}[bootstrap], func(t *testing.T) {
			e, _, _, remote := fixture(t)
			prepareCommit(t, e, bootstrap)
			if err := e.commit(Options{}, bootstrap); err != nil {
				t.Fatal(err)
			}
			command(t, e.Project.Root, "tag", "-a", "unrelated", "-m", "never push")
			command(t, e.Project.Root, "config", "push.followTags", "true")
			command(t, e.Project.Root, "config", "remote.origin.mirror", "true")
			before := snapshotRepository(t, e, remote)
			if err := e.push(Options{Noop: true}, bootstrap); err != nil {
				t.Fatal(err)
			}
			unchangedRepository(t, e, remote, before)
			branch, _ := e.branch()
			for range 2 {
				if err := e.push(Options{}, bootstrap); err != nil {
					t.Fatal(err)
				}
			}
			if command(t, e.Project.Root, "rev-parse", "@{upstream}") != before.Head {
				t.Fatal("upstream not set")
			}
			if got := command(t, remote, "for-each-ref", "--format=%(refname)"); got != "refs/heads/"+branch+"\nrefs/heads/main" && got != "refs/heads/main\nrefs/heads/"+branch {
				t.Fatal("push affected other refs:", got)
			}
		})
		for _, scenario := range []string{"dirty", "wrong branch", "origin", "changelog", "diverged", "detached", "operation", "missing token"} {
			t.Run(map[bool]string{false: "release", true: "bootstrap"}[bootstrap]+"/"+scenario, func(t *testing.T) {
				e, _, _, remote := fixture(t)
				prepareCommit(t, e, bootstrap)
				if err := e.commit(Options{}, bootstrap); err != nil {
					t.Fatal(err)
				}
				branch, _ := e.branch()
				switch scenario {
				case "dirty":
					put(t, e.Project.Root, "dirty", "x")
				case "wrong branch":
					command(t, e.Project.Root, "switch", "-c", "other")
				case "origin":
					command(t, e.Project.Root, "remote", "set-url", "origin", remote+"-wrong")
				case "changelog":
					put(t, e.Project.Root, "CHANGELOG.md", "invalid")
					command(t, e.Project.Root, "commit", "-am", "invalid changelog")
				case "diverged":
					command(t, e.Project.Root, "switch", "-c", "competing", "main")
					command(t, e.Project.Root, "commit", "--allow-empty", "-m", "competing")
					command(t, e.Project.Root, "push", "origin", "HEAD:refs/heads/"+branch)
					command(t, e.Project.Root, "switch", branch)
				case "detached":
					command(t, e.Project.Root, "checkout", "--detach")
				case "operation":
					put(t, e.Project.Root, ".git/REVERT_HEAD", command(t, e.Project.Root, "rev-parse", "HEAD"))
				case "missing token":
					e.Git.Token = ""
				}
				before := snapshotForDetached(t, e, remote)
				for _, noop := range []bool{true, false} {
					if err := e.push(Options{Noop: noop}, bootstrap); err == nil {
						t.Fatal("unsafe push accepted")
					}
					if !reflect.DeepEqual(before, snapshotForDetached(t, e, remote)) {
						t.Fatal("refused push changed local/remote state")
					}
				}
			})
		}
	}
}
func TestMigratedMainSyncIsIdempotentAndDoesNotPublish(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "missing"}[missing], func(t *testing.T) {
			e, _, _, remote := fixture(t)
			command(t, e.Project.Root, "switch", "-c", "old-work")
			if missing {
				command(t, e.Project.Root, "branch", "-D", "main")
			}
			command(t, e.Project.Root, "tag", "-a", "local-only", "-m", "keep")
			beforeRemote := command(t, remote, "for-each-ref")
			for range 2 {
				if err := e.syncMain(); err != nil {
					t.Fatal(err)
				}
			}
			if command(t, e.Project.Root, "branch", "--show-current") != "main" || command(t, e.Project.Root, "tag", "--list") != "local-only" || command(t, remote, "for-each-ref") != beforeRemote {
				t.Fatal("main-sync mutated unexpected state")
			}
			if _, ok := e.Git.Optional("rev-parse", "--verify", "refs/heads/old-work"); !ok {
				t.Fatal("deleted another branch")
			}
		})
	}
}
func TestMigratedAllGitOperationMarkers(t *testing.T) {
	for _, marker := range strings.Fields("MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD rebase-merge rebase-apply BISECT_START") {
		t.Run(marker, func(t *testing.T) {
			e, _, _, remote := fixture(t)
			if strings.HasPrefix(marker, "rebase-") {
				put(t, e.Project.Root, ".git/"+marker+"/state", "1")
			} else {
				put(t, e.Project.Root, ".git/"+marker, "1")
			}
			before := snapshotRepository(t, e, remote)
			expectError(t, e.syncMain(), "unfinished Git operation")
			unchangedRepository(t, e, remote, before)
		})
	}
}
func TestMigratedExistingAnnotatedTagIsReused(t *testing.T) {
	e, api, _, remote := fixture(t)
	command(t, e.Project.Root, "tag", "-a", "v1.0.0", "-m", "pre-existing")
	object := command(t, e.Project.Root, "rev-parse", "v1.0.0")
	greenMain(e, api)
	before := snapshotRepository(t, e, remote)
	if err := e.tag(Options{Noop: true}); err != nil {
		t.Fatal(err)
	}
	unchangedRepository(t, e, remote, before)
	if err := e.tag(Options{}); err != nil {
		t.Fatal(err)
	}
	if command(t, remote, "rev-parse", "v1.0.0") != object {
		t.Fatal("existing annotated tag recreated")
	}
}
func TestMigratedNoColorPreviewOverridesGitConfig(t *testing.T) {
	e, _, out, _ := fixture(t)
	prepareCommit(t, e, false)
	put(t, e.Project.Root, "untracked", "x")
	command(t, e.Project.Root, "config", "color.ui", "always")
	e.Color = "never"
	if err := e.commit(Options{Noop: true}, false); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte("\x1b[")) {
		t.Fatal("disabled color was ignored")
	}
}
func TestMigratedMainWaitActiveAndTerminalMatrix(t *testing.T) {
	for _, noop := range []bool{true, false} {
		for _, status := range strings.Fields("created waiting_for_resource preparing pending running scheduled failed canceled skipped manual unknown missing api_error wrong_sha") {
			t.Run(map[bool]string{true: "preview", false: "apply"}[noop]+"/"+status, func(t *testing.T) {
				e, api, _, remote := fixture(t)
				head := command(t, e.Project.Root, "rev-parse", "HEAD")
				polls, sleeps := 0, 0
				api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
					if q.Status != "" {
						return nil, nil
					}
					if q.Ref != "main" {
						return []Pipeline{{ID: 8, Ref: q.Ref, SHA: head, Status: "success"}}, nil
					}
					polls++
					current := status
					if polls > 1 && active(status) {
						current = "success"
					}
					if current == "missing" {
						return nil, nil
					}
					if current == "api_error" {
						return nil, errFixtureAPI
					}
					sha := head
					if current == "wrong_sha" {
						sha = "wrong"
					}
					return []Pipeline{{ID: 7, Ref: "main", SHA: sha, Status: current}}, nil
				}
				e.Sleep = func(_ context.Context, delay time.Duration) error {
					if delay != 5*time.Second {
						t.Fatal(delay)
					}
					sleeps++
					return nil
				}
				err := e.tag(Options{Noop: noop})
				if active(status) {
					if err != nil || sleeps != 1 {
						t.Fatal(err, sleeps)
					}
				} else {
					if err == nil {
						t.Fatal("terminal/missing CI accepted")
					}
					if command(t, e.Project.Root, "tag", "--list") != "" || command(t, remote, "tag", "--list") != "" {
						t.Fatal("CI refusal created/pushed a tag")
					}
				}
			})
		}
	}
}

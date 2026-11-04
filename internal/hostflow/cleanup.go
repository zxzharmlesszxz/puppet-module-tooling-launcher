package hostflow

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

type cleanupPlan struct{ Branch, Main, Tip, Tag, TagObject, TagSHA, Local, Remote, Tracking, Merge string }

func (e *Engine) cleanupPlan(branch string) (cleanupPlan, error) {
	p := cleanupPlan{Branch: branch}
	if err := e.guard(true, true, true); err != nil {
		return p, err
	}
	current, err := e.branch()
	if err != nil {
		return p, err
	}
	if current != "main" {
		return p, fmt.Errorf("switch to main before cleanup")
	}
	if err := e.validateBranch(branch); err != nil {
		return p, err
	}
	if after, ok := strings.CutPrefix(branch, "release/"); ok {
		if err := validVersion(after); err != nil {
			return p, err
		}
	} else if !strings.HasPrefix(branch, "bootstrap/") {
		return p, fmt.Errorf("cleanup accepts only release/X.Y.Z or bootstrap/<name>")
	}
	worktrees, err := e.Git.Run("worktree", "list", "--porcelain")
	if err != nil {
		return p, err
	}
	if slices.Contains(strings.Split(worktrees, "\n"), "branch refs/heads/"+branch) {
		return p, fmt.Errorf("branch is checked out in a worktree")
	}
	main, err := e.head()
	if err != nil {
		return p, err
	}
	p.Main = main
	refs, err := e.remoteRefs("refs/heads/main", "refs/heads/"+branch)
	if err != nil {
		return p, err
	}
	if refs["refs/heads/main"] != main {
		return p, fmt.Errorf("local main must match remote main")
	}
	p.Remote = refs["refs/heads/"+branch]
	for i, ref := range []string{"refs/heads/" + branch, "refs/remotes/origin/" + branch} {
		if _, symbolic := e.Git.Optional("symbolic-ref", "--quiet", ref); symbolic {
			return p, fmt.Errorf("refusing symbolic cleanup ref")
		}
		sha, exists := e.Git.Optional("rev-parse", "--verify", ref)
		if exists {
			if i == 0 {
				p.Local = sha
			} else {
				p.Tracking = sha
			}
		}
	}
	for _, sha := range []string{p.Local, p.Remote, p.Tracking} {
		if sha != "" {
			if p.Tip != "" && p.Tip != sha {
				return p, fmt.Errorf("cleanup refs disagree")
			}
			p.Tip = sha
		}
	}
	open, err := e.API.Requests(e.Context, "opened", branch, "")
	if err != nil {
		return p, err
	}
	if len(open) > 0 {
		return p, fmt.Errorf("branch has open merge requests")
	}
	merged, err := e.API.Requests(e.Context, "merged", branch, "main")
	if err != nil {
		return p, err
	}
	var candidates []Request
	for _, r := range merged {
		if p.Tip == "" || r.SHA == p.Tip {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return p, fmt.Errorf("matching merged request is missing")
	}
	request := candidates[0]
	if request.SHA == "" || request.MergeSHA == "" {
		return p, fmt.Errorf("merged request lacks source or merge SHA")
	}
	p.Merge = request.MergeSHA
	m, err := e.metadata(request.SHA)
	if err != nil {
		return p, err
	}
	if strings.HasPrefix(branch, "release/") && branch != "release/"+m.Version {
		return p, fmt.Errorf("merged request version differs from release branch")
	}
	p.Tag = "v" + m.Version
	refs, err = e.remoteRefs("refs/tags/"+p.Tag, "refs/tags/"+p.Tag+"^{}")
	if err != nil {
		return p, err
	}
	p.TagObject, p.TagSHA = refs["refs/tags/"+p.Tag], refs["refs/tags/"+p.Tag+"^{}"]
	if p.TagObject == "" || p.TagSHA == "" {
		return p, fmt.Errorf("published annotated release tag is missing")
	}
	for _, pair := range [][2]string{{request.SHA, p.TagSHA}, {request.MergeSHA, p.TagSHA}, {p.TagSHA, main}} {
		if err := e.ancestor(pair[0], pair[1]); err != nil {
			return p, err
		}
	}
	pipeline, err := e.latest(PipelineQuery{Ref: p.Tag, SHA: p.TagSHA, Source: "push"})
	if err != nil {
		return p, err
	}
	if pipeline.Ref != p.Tag || pipeline.SHA != p.TagSHA {
		return p, fmt.Errorf("release pipeline identity changed")
	}
	if err := e.published(pipeline, false); err != nil {
		return p, err
	}
	if p.Remote != "" {
		remote, err := e.API.Branch(e.Context, branch)
		if err != nil {
			return p, err
		}
		if remote.Protected {
			return p, fmt.Errorf("cleanup branch is protected")
		}
		if remote.SHA != p.Remote {
			return p, fmt.Errorf("remote branch changed")
		}
	} else {
		_, err := e.API.Branch(e.Context, branch)
		if err == nil {
			return p, fmt.Errorf("remote branch appeared")
		}
		if !errors.Is(err, ErrNotFound) {
			return p, err
		}
	}
	live, err := e.remoteRefs("refs/heads/main", "refs/heads/"+branch, "refs/tags/"+p.Tag, "refs/tags/"+p.Tag+"^{}")
	if err != nil {
		return p, err
	}
	if live["refs/heads/main"] != p.Main || live["refs/heads/"+branch] != p.Remote || live["refs/tags/"+p.Tag] != p.TagObject || live["refs/tags/"+p.Tag+"^{}"] != p.TagSHA {
		return p, fmt.Errorf("cleanup refs changed during API validation")
	}
	if err := e.guard(true, true, true); err != nil {
		return p, err
	}
	current, err = e.branch()
	if err != nil {
		return p, err
	}
	head, err := e.head()
	if err != nil {
		return p, err
	}
	if current != "main" || head != p.Main {
		return p, fmt.Errorf("local main changed during API validation")
	}
	for ref, expected := range map[string]string{"refs/heads/" + branch: p.Local, "refs/remotes/origin/" + branch: p.Tracking} {
		if _, symbolic := e.Git.Optional("symbolic-ref", "--quiet", ref); symbolic {
			return p, fmt.Errorf("cleanup ref became symbolic")
		}
		sha, _ := e.Git.Optional("rev-parse", "--verify", ref)
		if sha != expected {
			return p, fmt.Errorf("local cleanup refs changed during API validation")
		}
	}
	return p, nil
}
func (e *Engine) cleanup(o Options) error {
	if o.Branch == "" {
		return fmt.Errorf("cleanup requires explicit --branch")
	}
	before, err := e.cleanupPlan(o.Branch)
	if err != nil {
		return err
	}
	if before.Local == "" && before.Remote == "" && before.Tracking == "" {
		e.row("%s already clean; %s preserved", o.Branch, before.Tag)
		return nil
	}
	if o.Noop {
		e.row("would remove %s (remote=%s local=%s tracking=%s); preserve %s", o.Branch, before.Remote, before.Local, before.Tracking, before.Tag)
		return nil
	}
	after, err := e.cleanupPlan(o.Branch)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("cleanup state changed during revalidation")
	}
	if before.Remote != "" {
		ref := "refs/heads/" + o.Branch
		if _, err := e.Git.Run("push", "--porcelain", "--no-mirror", "--no-follow-tags", "--recurse-submodules=no", "--force-with-lease="+ref+":"+before.Remote, e.Project.Remote, ":"+ref); err != nil {
			return err
		}
	}
	if before.Local != "" {
		sha, err := e.Git.Text("rev-parse", "refs/heads/"+o.Branch)
		if err != nil {
			return err
		}
		if sha != before.Local {
			return fmt.Errorf("local branch changed before deletion")
		}
		if _, err := e.Git.Run("branch", "--delete", o.Branch); err != nil {
			return err
		}
	}
	if before.Tracking != "" {
		ref := "refs/remotes/origin/" + o.Branch
		sha, exists := e.Git.Optional("rev-parse", "--verify", ref)
		if exists {
			if sha != before.Tracking {
				return fmt.Errorf("tracking branch changed before deletion")
			}
			if _, err := e.Git.Run("update-ref", "-d", ref, sha); err != nil {
				return err
			}
		}
	}
	e.row("%s cleaned; %s preserved", o.Branch, before.Tag)
	return nil
}

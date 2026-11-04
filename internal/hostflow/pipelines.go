package hostflow

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func active(status string) bool {
	switch status {
	case "created", "waiting_for_resource", "preparing", "pending", "running", "scheduled":
		return true
	}
	return false
}
func (e *Engine) pause(message string, delay time.Duration) error {
	e.print("[WAIT] %s; checking again in %s\n", message, delay)
	return e.Sleep(e.Context, delay)
}
func (e *Engine) latest(q PipelineQuery) (Pipeline, error) {
	values, err := e.API.Pipelines(e.Context, q)
	if err != nil {
		return Pipeline{}, err
	}
	if len(values) == 0 || values[0].ID <= 0 {
		return Pipeline{}, fmt.Errorf("no matching pipeline for %s at %s", q.Ref, q.SHA)
	}
	return values[0], nil
}
func (e *Engine) published(p Pipeline, exact bool) error {
	if p.Status != "success" {
		return fmt.Errorf("pipeline %d is %s", p.ID, p.Status)
	}
	jobs, err := e.API.Jobs(e.Context, p.ID)
	if err != nil {
		return err
	}
	count, green := 0, 0
	for _, job := range jobs {
		if job.Name == "forge-publish" {
			count++
			if job.Status == "success" {
				green++
			}
		}
	}
	if green == 0 || exact && count != 1 {
		return fmt.Errorf("pipeline %d must contain a successful, unambiguous forge-publish job", p.ID)
	}
	return nil
}
func (e *Engine) pinMain(sha string) error {
	if err := e.guard(true, true, true); err != nil {
		return err
	}
	branch, err := e.branch()
	if err != nil {
		return err
	}
	if branch != "main" {
		return fmt.Errorf("switch to main before release")
	}
	head, err := e.head()
	if err != nil {
		return err
	}
	if head != sha {
		return fmt.Errorf("HEAD changed while waiting for CI")
	}
	refs, err := e.remoteRefs("refs/heads/main")
	if err != nil {
		return err
	}
	if refs["refs/heads/main"] != sha {
		return fmt.Errorf("remote main changed or differs from HEAD")
	}
	return nil
}
func (e *Engine) mainCI(sha string, wait bool) error {
	for {
		if err := e.pinMain(sha); err != nil {
			return err
		}
		waiting := false
		for _, status := range strings.Fields("created waiting_for_resource preparing pending running scheduled") {
			values, err := e.API.Pipelines(e.Context, PipelineQuery{Status: status})
			if err != nil {
				return err
			}
			for _, p := range values {
				if p.Ref != "main" || p.SHA != sha {
					return fmt.Errorf("tag blocked by pipeline %d (%s) on %s", p.ID, p.Status, p.Ref)
				}
				waiting = true
			}
		}
		p, err := e.latest(PipelineQuery{Ref: "main", SHA: sha, Source: "push"})
		if err != nil {
			return err
		}
		if p.Ref != "main" || p.SHA != sha {
			return fmt.Errorf("main pipeline identity changed")
		}
		waiting = waiting || active(p.Status)
		if !waiting {
			if p.Status != "success" {
				return fmt.Errorf("main pipeline %d is %s", p.ID, p.Status)
			}
			return e.pinMain(sha)
		}
		if !wait {
			return fmt.Errorf("main CI is still active")
		}
		if err := e.pause(fmt.Sprintf("%s: main pipeline %d %s at %s", e.Project.Name, p.ID, p.Status, sha), 5*time.Second); err != nil {
			return err
		}
	}
}
func (e *Engine) localTag(tag, sha string) (string, error) {
	object, exists := e.Git.Optional("rev-parse", "--verify", "refs/tags/"+tag)
	if !exists {
		return "", nil
	}
	kind, err := e.Git.Text("cat-file", "-t", object)
	if err != nil {
		return "", err
	}
	if kind != "tag" {
		return "", fmt.Errorf("local release tag must be annotated")
	}
	head, err := e.Git.Text("rev-parse", "refs/tags/"+tag+"^{}")
	if err != nil {
		return "", err
	}
	if head != sha {
		return "", fmt.Errorf("local tag points to a different commit")
	}
	return object, nil
}
func (e *Engine) tag(o Options) error {
	if err := e.guard(true, true, true); err != nil {
		return err
	}
	m, err := e.metadata("")
	if err != nil {
		return err
	}
	if err := e.changelog(m.Version); err != nil {
		return err
	}
	sha, err := e.head()
	if err != nil {
		return err
	}
	tag := "v" + m.Version
	refs, err := e.remoteRefs("refs/tags/" + tag)
	if err != nil {
		return err
	}
	if len(refs) != 0 {
		return fmt.Errorf("remote %s already exists", tag)
	}
	object, err := e.localTag(tag, sha)
	if err != nil {
		return err
	}
	if err := e.mainCI(sha, true); err != nil {
		return err
	}
	refs, err = e.remoteRefs("refs/tags/" + tag)
	if err != nil {
		return err
	}
	if len(refs) != 0 {
		return fmt.Errorf("remote tag appeared while waiting")
	}
	current, err := e.localTag(tag, sha)
	if err != nil {
		return err
	}
	if current != object {
		return fmt.Errorf("local tag changed while waiting")
	}
	if o.Noop {
		e.row("%s would create and push %s", tag, sha)
		return nil
	}
	if object == "" {
		author, err := e.author()
		if err != nil {
			return err
		}
		if _, err := e.Git.Run(append(author, "tag", "--annotate", tag, sha, "--message", "Release "+m.Version)...); err != nil {
			return err
		}
		object, err = e.localTag(tag, sha)
		if err != nil {
			return err
		}
	}
	if err := e.mainCI(sha, false); err != nil {
		return err
	}
	current, err = e.localTag(tag, sha)
	if err != nil {
		return err
	}
	if current != object {
		return fmt.Errorf("local tag changed before push")
	}
	if _, err := e.Git.Run("push", "--porcelain", "--no-force", "--no-mirror", "--no-follow-tags", "--recurse-submodules=no", e.Project.Remote, "refs/tags/"+tag+":refs/tags/"+tag); err != nil {
		return fmt.Errorf("local tag is preserved; retry release-tag after fixing push: %w", err)
	}
	e.row("%s pushed %s", tag, sha)
	e.Recovery, e.RecoveryRef = "pipeline-jobs", tag
	if err := e.waitPublication(tag, sha, object); err != nil {
		return fmt.Errorf("tag is preserved; retry pipeline-jobs --ref %s --watch 5: %w", tag, err)
	}
	e.row("%s published", tag)
	return nil
}
func (e *Engine) waitPublication(tag, sha, object string) error {
	var id int64
	missing := 0
	for {
		if err := e.origin(); err != nil {
			return err
		}
		refs, err := e.remoteRefs("refs/tags/"+tag, "refs/tags/"+tag+"^{}")
		if err != nil {
			return err
		}
		local, err := e.localTag(tag, sha)
		if err != nil {
			return err
		}
		if local != object {
			return fmt.Errorf("local published tag changed")
		}
		if refs["refs/tags/"+tag] != object || refs["refs/tags/"+tag+"^{}"] != sha {
			return fmt.Errorf("remote tag changed after push")
		}
		var p Pipeline
		if id == 0 {
			values, err := e.API.Pipelines(e.Context, PipelineQuery{Ref: tag, SHA: sha, Source: "push"})
			if err != nil {
				return err
			}
			if len(values) == 0 {
				if missing >= 12 {
					return fmt.Errorf("tag pipeline did not appear")
				}
				missing++
				if err := e.pause("tag pipeline has not appeared", 5*time.Second); err != nil {
					return err
				}
				continue
			}
			p = values[0]
			id = p.ID
			if id <= 0 {
				return fmt.Errorf("tag pipeline has no valid ID")
			}
		} else {
			p, err = e.API.Pipeline(e.Context, id)
			if err != nil {
				return err
			}
		}
		if p.ID != id || p.Ref != tag || p.SHA != sha {
			return fmt.Errorf("tag pipeline identity changed")
		}
		if !active(p.Status) {
			return e.published(p, true)
		}
		if err := e.pause(fmt.Sprintf("tag pipeline %d %s", id, p.Status), 5*time.Second); err != nil {
			return err
		}
	}
}
func (e *Engine) pipelineJobs(o Options) error {
	if err := e.token(); err != nil {
		return err
	}
	ref := o.Ref
	if ref == "" {
		ref = o.Branch
	}
	if ref == "" {
		var err error
		ref, err = e.branch()
		if err != nil {
			return err
		}
	}
	sha := ""
	branch, err := e.API.Branch(e.Context, ref)
	if err == nil {
		sha = branch.SHA
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	p, err := e.latest(PipelineQuery{Ref: ref, SHA: sha, Source: "push"})
	if err != nil {
		return err
	}
	id := p.ID
	for {
		if p.ID != id || p.Ref != ref || sha != "" && p.SHA != sha {
			return fmt.Errorf("pipeline identity changed")
		}
		e.row("pipeline %d %s %s", p.ID, p.Status, p.URL)
		jobs, err := e.API.Jobs(e.Context, p.ID)
		if err != nil {
			return err
		}
		for _, job := range jobs {
			e.row("%s %s %s", job.Name, job.Status, job.URL)
		}
		if !active(p.Status) {
			if p.Status != "success" {
				return fmt.Errorf("pipeline %d is %s", p.ID, p.Status)
			}
			return nil
		}
		if o.Watch == 0 {
			return nil
		}
		if err := e.pause("pipeline still running", o.Watch); err != nil {
			return err
		}
		p, err = e.API.Pipeline(e.Context, id)
		if err != nil {
			return err
		}
	}
}
func (e *Engine) request(o Options, create, merge bool) error {
	if err := e.guard(true, false, true); err != nil {
		return err
	}
	branch := o.Branch
	if branch == "" {
		var err error
		branch, err = e.branch()
		if err != nil {
			return err
		}
	}
	if branch == "main" {
		return fmt.Errorf("source branch cannot be main")
	}
	if err := e.validateBranch(branch); err != nil {
		return err
	}
	requests, err := e.API.Requests(e.Context, "opened", branch, "main")
	if err != nil {
		return err
	}
	if len(requests) > 1 {
		return fmt.Errorf("multiple open requests for %s", branch)
	}
	var request Request
	if len(requests) == 0 {
		if !create {
			return fmt.Errorf("no open merge request for %s", branch)
		}
		title := o.Title
		if title == "" {
			m, err := e.metadata("")
			if err != nil {
				return err
			}
			title = "[REL] Release " + m.Version
			if strings.HasPrefix(branch, "bootstrap/") {
				title = "[UPD] Update module"
			}
		}
		request, err = e.API.CreateRequest(e.Context, title, branch)
		if err != nil {
			return err
		}
	} else {
		request = requests[0]
	}
	e.row("request %s", request.URL)
	if !merge {
		return nil
	}
	request, err = e.API.Request(e.Context, request.ID)
	if err != nil {
		return err
	}
	sha := request.SHA
	if sha == "" {
		return fmt.Errorf("merge request has no head SHA")
	}
	for {
		current, err := e.API.Request(e.Context, request.ID)
		if err != nil {
			return err
		}
		if current.SHA != sha || current.Source != branch || current.Target != "main" || current.State != "opened" && current.State != "open" {
			return fmt.Errorf("merge request head or state changed")
		}
		p, err := e.latest(PipelineQuery{Ref: branch, SHA: sha, Source: "push"})
		if err != nil {
			return err
		}
		if p.Ref != branch || p.SHA != sha {
			return fmt.Errorf("source pipeline identity changed")
		}
		if !active(p.Status) {
			if p.Status != "success" {
				return fmt.Errorf("source pipeline %d is %s", p.ID, p.Status)
			}
			break
		}
		if err := e.pause(fmt.Sprintf("source pipeline %d %s", p.ID, p.Status), 5*time.Second); err != nil {
			return err
		}
	}
	current, err := e.API.Request(e.Context, request.ID)
	if err != nil {
		return err
	}
	if current.SHA != sha || current.Source != branch || current.Target != "main" || current.State != "opened" && current.State != "open" {
		return fmt.Errorf("merge request changed before merge")
	}
	result, err := e.API.MergeRequest(e.Context, request.ID, sha)
	if err != nil {
		return err
	}
	if result.State != "merged" {
		return fmt.Errorf("merge was not confirmed")
	}
	e.row("request merged %s", result.URL)
	return nil
}

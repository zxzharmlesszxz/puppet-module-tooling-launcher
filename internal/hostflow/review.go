package hostflow

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ReviewStatus describes the approval gate for the observed request head.
type ReviewStatus struct {
	SHA, WaitingFor string
}

func (c *Client) Review(ctx context.Context, request Request) (ReviewStatus, error) {
	if c.Project.Provider == "github" {
		return c.githubReview(ctx, request.ID)
	}
	status := ReviewStatus{SHA: request.SHA}
	switch request.DetailedMergeStatus {
	case "not_approved":
		status.WaitingFor = "required GitLab approval"
	case "approvals_syncing", "checking", "unchecked", "preparing":
		status.WaitingFor = "GitLab merge and approval status to settle"
	case "":
		return ReviewStatus{}, fmt.Errorf("missing GitLab detailed merge status")
	}
	return status, nil
}

func (c *Client) githubReview(ctx context.Context, id int64) (ReviewStatus, error) {
	const query = `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) { headRefOid reviewDecision }
  }
}`
	owner, repo, ok := strings.Cut(c.Project.ID, "/")
	if !ok {
		return ReviewStatus{}, fmt.Errorf("invalid GitHub repository for review query")
	}
	graph := *c
	graph.Project.Endpoint = strings.TrimSuffix(c.Project.Endpoint, "/v3")
	body := record{"query": query, "variables": record{"owner": owner, "repo": repo, "number": id}}
	var result record
	if err := graph.call(ctx, http.MethodPost, "/graphql", nil, body, &result); err != nil {
		return ReviewStatus{}, fmt.Errorf("read GitHub review decision: %w", err)
	}
	if len(array(result, "errors")) != 0 {
		return ReviewStatus{}, fmt.Errorf("could not read the GitHub review decision; check pull request read access")
	}
	pr := object(object(object(result, "data"), "repository"), "pullRequest")
	status := ReviewStatus{SHA: text(pr, "headRefOid")}
	decision, found := pr["reviewDecision"]
	if status.SHA == "" || !found {
		return ReviewStatus{}, fmt.Errorf("incomplete review decision from GitHub")
	}
	switch decision {
	case nil, "APPROVED":
	case "REVIEW_REQUIRED":
		status.WaitingFor = "required GitHub approval"
	case "CHANGES_REQUESTED":
		status.WaitingFor = "GitHub review with changes requested to be resolved"
	default:
		return ReviewStatus{}, fmt.Errorf("unsupported review decision from GitHub")
	}
	return status, nil
}

func (e *Engine) openRequest(id int64, branch string) (Request, error) {
	r, err := e.API.Request(e.Context, id)
	if err != nil {
		return r, err
	}
	if r.ID != id || r.SHA == "" || r.Source != branch || r.Target != "main" || r.ForeignSource ||
		(r.State != "opened" && r.State != "open") || (r.Repository != "" && r.Repository != e.Project.ID) {
		return r, fmt.Errorf("merge request source, target, or state changed")
	}
	return r, nil
}

const reviewPoll = 5 * time.Second

func (e *Engine) waitMergeReady(id int64, branch string) (string, error) {
	var sha string
	missingPolls := 0
	for {
		if err := e.Context.Err(); err != nil {
			return "", err
		}
		r, err := e.openRequest(id, branch)
		if err != nil {
			return "", err
		}
		if sha != "" && sha != r.SHA {
			e.row("request head changed to %s; rechecking CI and approval", r.SHA)
			missingPolls = 12
		}
		sha = r.SHA
		p, err := e.latest(PipelineQuery{Ref: branch, SHA: sha, Source: "push"})
		if errors.Is(err, errNoMatchingPipeline) && missingPolls > 0 {
			missingPolls--
			if err := e.pause("new head push pipeline has not appeared", reviewPoll); err != nil {
				return "", err
			}
			continue
		}
		if err != nil {
			return "", err
		}
		missingPolls = 0
		ready, err := e.checkMergePipeline(p, branch, sha)
		if err != nil {
			return "", err
		}
		if !ready {
			continue
		}
		r, err = e.openRequest(id, branch)
		if err != nil {
			return "", err
		}
		if r.SHA != sha {
			continue
		}
		review, err := e.API.Review(e.Context, r)
		if err != nil {
			return "", err
		}
		if review.SHA != sha {
			review.WaitingFor = "review status for the current head"
		}
		if review.WaitingFor != "" {
			if err := e.pause("waiting for "+review.WaitingFor, reviewPoll); err != nil {
				return "", err
			}
			continue
		}
		r, err = e.openRequest(id, branch)
		if err != nil {
			return "", err
		}
		if r.SHA == sha {
			return sha, nil
		}
	}
}

func (e *Engine) checkMergePipeline(p Pipeline, branch, sha string) (bool, error) {
	if p.Ref != branch || p.SHA != sha {
		return false, fmt.Errorf("source pipeline identity changed")
	}
	if active(p.Status) {
		return false, e.pause(fmt.Sprintf("source pipeline %d %s", p.ID, p.Status), reviewPoll)
	}
	if p.Status != "success" {
		e.failedLogs(p)
		return false, fmt.Errorf("source pipeline %d is %s", p.ID, p.Status)
	}
	return true, nil
}

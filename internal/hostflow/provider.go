package hostflow

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func (c *Client) Pipelines(ctx context.Context, q PipelineQuery) ([]Pipeline, error) {
	query := url.Values{}
	path, key := c.prefix()+"/pipelines", ""
	if q.Ref != "" {
		query.Set("ref", q.Ref)
	}
	if q.SHA != "" {
		query.Set("sha", q.SHA)
	}
	if q.Status != "" {
		query.Set("status", q.Status)
	}
	if q.Source != "" {
		query.Set("source", q.Source)
	}
	if c.Project.Provider == "github" {
		path, key = c.prefix()+"/actions/runs", "workflow_runs"
		query = url.Values{}
		if q.Ref != "" {
			query.Set("branch", q.Ref)
			query.Set("event", "push")
		}
		if q.SHA != "" {
			query.Set("head_sha", q.SHA)
		}
	}
	records, err := c.pages(ctx, path, key, query)
	if err != nil {
		return nil, err
	}
	var result []Pipeline
	for _, r := range records {
		p := c.pipeline(r)
		if c.Project.Provider == "github" && q.Ref != "" && (strings.Split(text(r, "path"), "@")[0] != ".github/workflows/puppet-module.yml" || text(r, "event") != "push") {
			continue
		}
		if q.Ref != "" && p.Ref != q.Ref || q.SHA != "" && p.SHA != q.SHA || q.Status != "" && p.Status != q.Status {
			continue
		}
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID > result[j].ID })
	return result, nil
}
func (c *Client) Pipeline(ctx context.Context, id int64) (Pipeline, error) {
	path := c.prefix() + "/pipelines/" + strconv.FormatInt(id, 10)
	if c.Project.Provider == "github" {
		path = c.prefix() + "/actions/runs/" + strconv.FormatInt(id, 10)
	}
	var r record
	err := c.call(ctx, http.MethodGet, path, nil, nil, &r)
	return c.pipeline(r), err
}
func (c *Client) Jobs(ctx context.Context, id int64) ([]Job, error) {
	path, key, query := c.prefix()+"/pipelines/"+strconv.FormatInt(id, 10)+"/jobs", "", url.Values{"include_retried": {"false"}}
	if c.Project.Provider == "github" {
		path, key, query = c.prefix()+"/actions/runs/"+strconv.FormatInt(id, 10)+"/jobs", "jobs", url.Values{"filter": {"latest"}}
	}
	records, err := c.pages(ctx, path, key, query)
	if err != nil {
		return nil, err
	}
	var result []Job
	for _, r := range records {
		name, status, address := text(r, "name"), text(r, "status"), text(r, "web_url")
		if c.Project.Provider == "github" {
			status, address = normalizedStatus(r), text(r, "html_url")
			if strings.HasSuffix(name, " / forge-publish") {
				name = "forge-publish"
			}
		}
		result = append(result, Job{Name: name, Status: status, URL: address, Runner: text(object(r, "runner"), "description")})
	}
	return result, nil
}
func (c *Client) Branch(ctx context.Context, name string) (Branch, error) {
	path := c.prefix() + "/repository/branches/" + url.PathEscape(name)
	if c.Project.Provider == "github" {
		path = c.prefix() + "/branches/" + url.PathEscape(name)
	}
	var r record
	err := c.call(ctx, http.MethodGet, path, nil, nil, &r)
	sha := text(object(r, "commit"), "id")
	if c.Project.Provider == "github" {
		sha = text(object(r, "commit"), "sha")
	}
	return Branch{Name: text(r, "name"), SHA: sha, Protected: boolean(r, "protected")}, err
}
func (c *Client) Requests(ctx context.Context, state, source, target string) ([]Request, error) {
	path := c.prefix() + "/merge_requests"
	query := url.Values{"state": {state}}
	if target != "" {
		query.Set("target_branch", target)
	}
	if source != "" {
		query.Set("source_branch", source)
	}
	if c.Project.Provider == "github" {
		path = c.prefix() + "/pulls"
		query = url.Values{"state": {"closed"}}
		if target != "" {
			query.Set("base", target)
		}
		if state == "opened" {
			query.Set("state", "open")
		}
		if source != "" {
			query.Set("head", strings.Split(c.Project.ID, "/")[0]+":"+source)
		}
	}
	values, err := c.pages(ctx, path, "", query)
	if err != nil {
		return nil, err
	}
	var result []Request
	for _, r := range values {
		p := c.request(r)
		if target != "" && p.Target != target || source != "" && (p.Source != source || p.Repository != c.Project.ID) || state == "merged" && p.State != "merged" {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}
func (c *Client) Request(ctx context.Context, id int64) (Request, error) {
	path := c.prefix() + "/merge_requests/" + strconv.FormatInt(id, 10)
	if c.Project.Provider == "github" {
		path = c.prefix() + "/pulls/" + strconv.FormatInt(id, 10)
	}
	var r record
	err := c.call(ctx, http.MethodGet, path, nil, nil, &r)
	return c.request(r), err
}
func (c *Client) CreateRequest(ctx context.Context, title, branch string) (Request, error) {
	path := c.prefix() + "/merge_requests"
	body := record{"title": title, "source_branch": branch, "target_branch": "main", "remove_source_branch": true}
	if c.Project.Provider == "github" {
		path = c.prefix() + "/pulls"
		body = record{"title": title, "head": branch, "base": "main"}
	}
	var r record
	err := c.call(ctx, http.MethodPost, path, nil, body, &r)
	return c.request(r), err
}
func (c *Client) MergeRequest(ctx context.Context, id int64, sha string) (Request, error) {
	path := c.prefix() + "/merge_requests/" + strconv.FormatInt(id, 10) + "/merge"
	body := record{"sha": sha, "should_remove_source_branch": true}
	if c.Project.Provider == "github" {
		path = c.prefix() + "/pulls/" + strconv.FormatInt(id, 10) + "/merge"
		body = record{"sha": sha, "merge_method": "merge"}
	}
	var r record
	if err := c.call(ctx, http.MethodPut, path, nil, body, &r); err != nil {
		return Request{}, err
	}
	if c.Project.Provider == "github" && !boolean(r, "merged") {
		return Request{}, fmt.Errorf("GitHub refused the pinned-head merge")
	}
	return c.Request(ctx, id)
}

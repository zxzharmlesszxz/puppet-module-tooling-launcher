package hostflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrNotFound = errors.New("hosting object not found")

type Pipeline struct {
	ID                    int64
	Ref, SHA, Status, URL string
}
type Job struct {
	ID                        int64
	Name, Status, URL, Runner string
}
type Branch struct {
	Name, SHA string
	Protected bool
}
type Request struct {
	ID                                                               int64
	State, SHA, Source, Target, Repository, MergeSHA, URL, Reference string
	DetailedMergeStatus                                              string
}
type PipelineQuery struct{ Ref, SHA, Status, Source string }

type Hosting interface {
	Pipelines(context.Context, PipelineQuery) ([]Pipeline, error)
	Pipeline(context.Context, int64) (Pipeline, error)
	Jobs(context.Context, int64) ([]Job, error)
	JobLog(context.Context, int64) ([]byte, error)
	Branch(context.Context, string) (Branch, error)
	Requests(context.Context, string, string, string) ([]Request, error)
	Request(context.Context, int64) (Request, error)
	CreateRequest(context.Context, string, string) (Request, error)
	Review(context.Context, Request) (ReviewStatus, error)
	MergeRequest(context.Context, int64, string) (Request, error)
	Policy(context.Context, bool, bool, io.Writer, PolicyLevels) error
}

type Client struct {
	Project Project
	Token   string
	HTTP    *http.Client
}

func NewClient(project Project, token string) *Client {
	return &Client{Project: project, Token: token, HTTP: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) call(ctx context.Context, method, path string, query url.Values, body any, target any) (caught error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	u, err := url.Parse(strings.TrimRight(c.Project.Endpoint, "/") + path)
	if err != nil {
		return err
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Project.Provider == "github" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	} else {
		req.Header.Set("PRIVATE-TOKEN", c.Token)
	}
	response, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s API request failed: %w", c.Project.Provider, err)
	}
	defer func() {
		if err := response.Body.Close(); caught == nil {
			caught = err
		}
	}()
	if response.StatusCode == 404 {
		return fmt.Errorf("%w: %s %s", ErrNotFound, method, path)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s API %s %s: HTTP %d", c.Project.Provider, method, path, response.StatusCode)
	}
	if target == nil || response.StatusCode == 204 {
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<20))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid hosting API JSON: %w", err)
	}
	return nil
}

func (c *Client) prefix() string {
	if c.Project.Provider == "github" {
		return "/repos/" + c.Project.ID
	}
	return "/projects/" + url.PathEscape(c.Project.ID)
}

type record map[string]any

func text(r record, key string) string { value, _ := r[key].(string); return value }
func number(r record, key string) int64 {
	switch v := r[key].(type) {
	case json.Number:
		n, _ := v.Int64()
		return n
	case float64:
		return int64(v)
	}
	return 0
}
func object(r record, key string) record { v, _ := r[key].(map[string]any); return v }
func array(r record, key string) []any   { v, _ := r[key].([]any); return v }
func boolean(r record, key string) bool  { v, _ := r[key].(bool); return v }

func (c *Client) pages(ctx context.Context, path, key string, query url.Values) ([]record, error) {
	if query == nil {
		query = url.Values{}
	}
	var result []record
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", "100")
		var batch []record
		if key == "" {
			if err := c.call(ctx, http.MethodGet, path, query, nil, &batch); err != nil {
				return nil, err
			}
		} else {
			var data map[string]json.RawMessage
			if err := c.call(ctx, http.MethodGet, path, query, nil, &data); err != nil {
				return nil, err
			}
			if err := json.Unmarshal(data[key], &batch); err != nil {
				return nil, err
			}
		}
		result = append(result, batch...)
		if len(batch) < 100 {
			return result, nil
		}
	}
}

func normalizedStatus(r record) string {
	if text(r, "status") != "completed" {
		switch text(r, "status") {
		case "queued", "in_progress", "waiting", "pending", "requested":
			return "running"
		}
		return "failed"
	}
	switch text(r, "conclusion") {
	case "success":
		return "success"
	case "cancelled":
		return "canceled"
	case "skipped":
		return "skipped"
	case "action_required":
		return "manual"
	}
	return "failed"
}

func (c *Client) pipeline(r record) Pipeline {
	if c.Project.Provider == "github" {
		return Pipeline{ID: number(r, "id"), Ref: text(r, "head_branch"), SHA: text(r, "head_sha"), Status: normalizedStatus(r), URL: text(r, "html_url")}
	}
	return Pipeline{ID: number(r, "id"), Ref: text(r, "ref"), SHA: text(r, "sha"), Status: text(r, "status"), URL: text(r, "web_url")}
}

func (c *Client) request(r record) Request {
	if c.Project.Provider == "github" {
		state := text(r, "state")
		if r["merged_at"] != nil {
			state = "merged"
		}
		head, base := object(r, "head"), object(r, "base")
		return Request{ID: number(r, "number"), State: state, SHA: text(head, "sha"), Source: text(head, "ref"), Target: text(base, "ref"), Repository: text(object(head, "repo"), "full_name"), MergeSHA: text(r, "merge_commit_sha"), URL: text(r, "html_url"), Reference: fmt.Sprintf("%s#%d", c.Project.ID, number(r, "number"))}
	}
	return Request{ID: number(r, "iid"), State: text(r, "state"), SHA: text(r, "sha"), Source: text(r, "source_branch"), Target: text(r, "target_branch"), Repository: c.Project.ID, DetailedMergeStatus: text(r, "detailed_merge_status"), MergeSHA: text(r, "merge_commit_sha"), URL: text(r, "web_url"), Reference: text(object(r, "references"), "full")}
}

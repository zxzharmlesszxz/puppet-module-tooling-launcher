package hostflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(err)
	}
}
func TestProviderPaginationAndAuthentication(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if provider == "gitlab" {
					if r.Header.Get("PRIVATE-TOKEN") != "secret" || r.Header.Get("Authorization") != "" {
						t.Error("wrong token scope")
					}
				} else if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("PRIVATE-TOKEN") != "" {
					t.Error("wrong token scope")
				}
				if r.URL.Query().Get("per_page") != "100" {
					t.Error("pagination size")
				}
				var result []record
				n := 100
				if r.URL.Query().Get("page") == "2" {
					n = 1
				}
				for i := 0; i < n; i++ {
					p := record{"id": calls*100 + i, "ref": "main", "sha": "abc", "status": "success"}
					if provider == "github" {
						p = record{"id": calls*100 + i, "head_branch": "main", "head_sha": "abc", "status": "completed", "conclusion": "success", "path": ".github/workflows/puppet-module.yml@main", "event": "push"}
					}
					result = append(result, p)
				}
				if provider == "github" {
					writeJSON(t, w, record{"workflow_runs": result})
				} else {
					writeJSON(t, w, result)
				}
			}))
			defer server.Close()
			c := NewClient(Project{Endpoint: server.URL, ID: "team/puppet-sample", Provider: provider}, "secret")
			result, err := c.Pipelines(testContext(t), PipelineQuery{Ref: "main", SHA: "abc", Source: "push"})
			if err != nil || len(result) != 101 || calls != 2 || result[0].ID != 200 {
				t.Fatal(result, err, calls)
			}
		})
	}
}
func TestRedirectsCannotLeakTokens(t *testing.T) {
	received := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	c := NewClient(Project{Endpoint: source.URL, ID: "team/puppet-sample", Provider: "gitlab"}, "secret")
	_, err := c.Branch(testContext(t), "main")
	expectError(t, err, "HTTP 302")
	if received {
		t.Fatal("authenticated redirect was followed")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatal("token leaked")
	}
}
func TestProviderRequestAndMergeContracts(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			put := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/merge") {
					put = true
					var body record
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if text(body, "sha") != "pin" {
						t.Error("merge was not pinned")
					}
					if provider == "github" {
						if text(body, "merge_method") != "merge" {
							t.Error(body)
						}
						writeJSON(t, w, record{"merged": true})
						return
					}
				}
				if provider == "github" {
					writeJSON(t, w, record{"number": 3, "state": "closed", "merged_at": "now", "head": record{"ref": "release/1.2.3", "sha": "pin", "repo": record{"full_name": "team/puppet-sample"}}, "base": record{"ref": "main"}, "merge_commit_sha": "merge"})
				} else {
					writeJSON(t, w, record{"iid": 3, "state": "merged", "sha": "pin", "source_branch": "release/1.2.3", "target_branch": "main", "merge_commit_sha": "merge"})
				}
			}))
			defer server.Close()
			c := NewClient(Project{Endpoint: server.URL, ID: "team/puppet-sample", Provider: provider}, "secret")
			r, err := c.MergeRequest(testContext(t), 3, "pin")
			if err != nil || r.SHA != "pin" || r.MergeSHA != "merge" || r.State != "merged" || !put {
				t.Fatal(r, err)
			}
		})
	}
}
func TestAPIErrorAndCancellation(t *testing.T) {
	for _, code := range []int{401, 403, 404, 429, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			count := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				w.WriteHeader(code)
				_, _ = w.Write([]byte("secret server content"))
			}))
			defer s.Close()
			c := NewClient(Project{Endpoint: s.URL, ID: "x/puppet-y", Provider: "github"}, "secret")
			_, err := c.Branch(testContext(t), "main")
			if err == nil || strings.Contains(err.Error(), "secret") || count != 1 {
				t.Fatal(err, count)
			}
			if code == 404 && !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
		})
	}
	c := NewClient(Project{Endpoint: "https://example.invalid", ID: "x/puppet-y", Provider: "gitlab"}, "secret")
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	_, err := c.Branch(ctx, "main")
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestGitHubWorkflowAndSourceFiltering(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, record{"workflow_runs": []record{{"id": 1, "head_branch": "main", "head_sha": "abc", "path": ".github/workflows/puppet-module.yml", "event": "pull_request", "status": "completed", "conclusion": "success"}, {"id": 2, "head_branch": "main", "head_sha": "abc", "path": ".github/workflows/other.yml", "event": "push", "status": "completed", "conclusion": "success"}, {"id": 3, "head_branch": "main", "head_sha": "other", "path": ".github/workflows/puppet-module.yml", "event": "push", "status": "completed", "conclusion": "success"}, {"id": 4, "head_branch": "main", "head_sha": "abc", "path": ".github/workflows/puppet-module.yml", "event": "push", "status": "completed", "conclusion": "success"}}})
	}))
	defer s.Close()
	c := NewClient(Project{Endpoint: s.URL, ID: "x/puppet-y", Provider: "github"}, "secret")
	p, err := c.Pipelines(testContext(t), PipelineQuery{Ref: "main", SHA: "abc", Source: "push"})
	if err != nil || len(p) != 1 || p[0].ID != 4 {
		t.Fatal(p, err)
	}
}
func TestPolicyNoopAndDivergence(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			mutations := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					mutations++
					writeJSON(t, w, record{})
					return
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/main"):
					w.WriteHeader(404)
				case strings.HasSuffix(r.URL.Path, "/master"):
					writeJSON(t, w, record{"name": "master", "commit": record{"sha": "old", "id": "old"}})
				case strings.HasSuffix(r.URL.Path, "/rulesets"), strings.HasSuffix(r.URL.Path, "/protected_tags"):
					writeJSON(t, w, []record{})
				default:
					writeJSON(t, w, record{"default_branch": "master"})
				}
			}))
			defer s.Close()
			c := NewClient(Project{Endpoint: s.URL, ID: "x/puppet-y", Name: "puppet-y", Provider: provider}, "secret")
			if err := c.Policy(testContext(t), true, false, testingWriter{t}, PolicyLevels{40, 40, 40}); err != nil {
				t.Fatal(err)
			}
			if mutations != 0 {
				t.Fatal("policy preview mutated server")
			}
		})
	}
}

type testingWriter struct{ t *testing.T }

func (w testingWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

func TestPipelineAndJobsNormalization(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/branches/"):
					writeJSON(t, w, record{"name": "release/1.2.3", "commit": record{"sha": "abc", "id": "abc"}, "protected": true})
				case strings.HasSuffix(r.URL.Path, "/jobs"):
					job := record{"name": "forge-publish", "status": "success"}
					if provider == "github" {
						job = record{"name": "publish / forge-publish", "status": "completed", "conclusion": "success"}
						writeJSON(t, w, record{"jobs": []record{job}})
					} else {
						writeJSON(t, w, []record{job})
					}
				default:
					if provider == "github" {
						writeJSON(t, w, record{"id": 4, "head_branch": "v1.2.3", "head_sha": "abc", "status": "completed", "conclusion": "success"})
					} else {
						writeJSON(t, w, record{"id": 4, "ref": "v1.2.3", "sha": "abc", "status": "success"})
					}
				}
			}))
			defer s.Close()
			c := NewClient(Project{Endpoint: s.URL, ID: "team/puppet-sample", Provider: provider}, "secret")
			p, err := c.Pipeline(testContext(t), 4)
			if err != nil || p.Status != "success" || p.SHA != "abc" {
				t.Fatal(p, err)
			}
			jobs, err := c.Jobs(testContext(t), 4)
			if err != nil || len(jobs) != 1 || jobs[0].Name != "forge-publish" || jobs[0].Status != "success" {
				t.Fatal(jobs, err)
			}
			b, err := c.Branch(testContext(t), "release/1.2.3")
			if err != nil || !b.Protected || b.SHA != "abc" {
				t.Fatal(b, err)
			}
		})
	}
}
func TestProviderCreationAndForeignPullFiltering(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					var body record
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if text(body, "title") != "title" {
						t.Error(body)
					}
				}
				if provider == "github" {
					pr := record{"number": 3, "state": "open", "head": record{"sha": "abc", "ref": "release/1.2.3", "repo": record{"full_name": "team/puppet-sample"}}, "base": record{"ref": "main"}}
					if r.Method == "POST" {
						writeJSON(t, w, pr)
					} else {
						foreign := record{"number": 4, "state": "open", "head": record{"sha": "foreign", "ref": "release/1.2.3", "repo": record{"full_name": "other/puppet-sample"}}, "base": record{"ref": "main"}}
						writeJSON(t, w, []record{pr, foreign})
					}
				} else {
					pr := record{"iid": 3, "state": "opened", "sha": "abc", "source_branch": "release/1.2.3", "target_branch": "main"}
					if r.Method == "POST" {
						writeJSON(t, w, pr)
					} else {
						writeJSON(t, w, []record{pr})
					}
				}
			}))
			defer s.Close()
			c := NewClient(Project{Endpoint: s.URL, ID: "team/puppet-sample", Provider: provider}, "secret")
			created, err := c.CreateRequest(testContext(t), "title", "release/1.2.3")
			if err != nil || created.SHA != "abc" {
				t.Fatal(created, err)
			}
			requests, err := c.Requests(testContext(t), "opened", "release/1.2.3", "main")
			if err != nil || len(requests) != 1 || requests[0].SHA != "abc" {
				t.Fatal(requests, err)
			}
		})
	}
}

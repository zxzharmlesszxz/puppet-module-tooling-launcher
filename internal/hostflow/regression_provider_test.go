package hostflow

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMigratedProviderJobsAndRequestsAreFullyPaginated(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			var pages []string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := r.URL.Query().Get("page")
				pages = append(pages, page)
				n := 100
				if page == "2" {
					n = 1
				}
				var items []record
				for i := 0; i < n; i++ {
					if strings.HasSuffix(r.URL.Path, "/jobs") {
						name := "other"
						if page == "2" {
							name = "forge-publish"
						}
						job := record{"name": name, "status": "success"}
						if provider == "github" {
							job["status"], job["conclusion"] = "completed", "success"
						}
						items = append(items, job)
					} else {
						pr := record{"iid": i + 1, "state": "merged", "sha": "abc", "source_branch": "release/x", "target_branch": "main"}
						if provider == "github" {
							pr = record{"number": i + 1, "state": "closed", "merged_at": "now", "head": record{"sha": "abc", "ref": "release/x", "repo": record{"full_name": "team/puppet-sample"}}, "base": record{"ref": "main"}}
						}
						items = append(items, pr)
					}
				}
				if provider == "github" && strings.HasSuffix(r.URL.Path, "/jobs") {
					writeJSON(t, w, record{"jobs": items})
				} else {
					writeJSON(t, w, items)
				}
			}))
			defer s.Close()
			c := NewClient(Project{Endpoint: s.URL, ID: "team/puppet-sample", Provider: provider}, "scoped-token")
			jobs, err := c.Jobs(testContext(t), 9)
			if err != nil || len(jobs) != 101 || jobs[100].Name != "forge-publish" {
				t.Fatal(jobs, err)
			}
			requests, err := c.Requests(testContext(t), "merged", "release/x", "main")
			if err != nil || len(requests) != 101 || strings.Join(pages, ",") != "1,2,1,2" {
				t.Fatal(requests, err, pages)
			}
		})
	}
}
func TestMigratedGitHubStatusConclusions(t *testing.T) {
	for _, conclusion := range strings.Fields("cancelled skipped action_required failure timed_out neutral startup_failure stale unknown") {
		if got := normalizedStatus(record{"status": "completed", "conclusion": conclusion}); got == "success" {
			t.Fatal("unsafe conclusion accepted", conclusion)
		}
	}
	for _, status := range strings.Fields("queued in_progress waiting pending requested") {
		if normalizedStatus(record{"status": status}) != "running" {
			t.Fatal(status)
		}
	}
}
func TestMigratedReleaseUsesBothRealRESTAdapters(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			e, _, out, remote := fixture(t)
			head := command(t, e.Project.Root, "rev-parse", "HEAD")
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if provider == "gitlab" && r.Header.Get("PRIVATE-TOKEN") != "test-token" || provider == "github" && r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("wrong provider token")
				}
				if strings.HasSuffix(r.URL.Path, "/jobs") {
					if provider == "github" {
						writeJSON(t, w, record{"jobs": []record{{"name": "module / forge-publish", "status": "completed", "conclusion": "success"}}})
					} else {
						writeJSON(t, w, []record{{"name": "forge-publish", "status": "success"}})
					}
					return
				}
				ref := r.URL.Query().Get("ref")
				if provider == "github" {
					ref = r.URL.Query().Get("branch")
				}
				if ref == "" || r.URL.Query().Get("status") != "" {
					if provider == "github" {
						writeJSON(t, w, record{"workflow_runs": []record{}})
					} else {
						writeJSON(t, w, []record{})
					}
					return
				}
				p := record{"id": 9, "ref": ref, "sha": head, "status": "success"}
				if provider == "github" {
					p = record{"id": 9, "head_branch": ref, "head_sha": head, "status": "completed", "conclusion": "success", "path": ".github/workflows/puppet-module.yml", "event": "push"}
					writeJSON(t, w, record{"workflow_runs": []record{p}})
				} else {
					writeJSON(t, w, []record{p})
				}
			}))
			defer s.Close()
			project := e.Project
			project.Endpoint, project.Provider = s.URL, provider
			e.API = NewClient(project, e.Git.Token)
			if err := e.tag(Options{}); err != nil {
				t.Fatal(err)
			}
			if command(t, remote, "rev-parse", "v1.0.0^{}") != head || !strings.Contains(out.String(), "published") {
				t.Fatal("normalized adapter did not confirm release")
			}
		})
	}
}

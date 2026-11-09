package hostflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/scaffold"
)

func TestTemplateRepositoryEnablesOnlyHostingCommands(t *testing.T) {
	root := configuredOriginClone(t, "https://git.example.test/team/pmtool-template-example.git")
	files, err := scaffold.Files(scaffold.Options{Name: "team", Provider: "gitlab", Version: "v3.1.0", VoxBox: "voxbox:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		put(t, root, name, string(body))
	}
	command(t, root, "add", ".")
	command(t, root, "-c", "commit.gpgsign=false", "commit", "-m", "template fixture")
	getenv := func(string) string { return "" }
	o := Options{Module: root, Command: "mr"}
	e, err := New(t.Context(), o, getenv, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !e.Project.Template || e.Project.ID != "team/pmtool-template-example" || e.Project.Provider != "gitlab" {
		t.Fatalf("unexpected template project %+v", e.Project)
	}
	o.Command = "release-tag"
	_, err = New(t.Context(), o, getenv, &bytes.Buffer{})
	expectError(t, err, "not supported for template repositories")

	put(t, root, "template.yml", "schema: 99\n")
	o.Command = "mr-create"
	_, err = New(t.Context(), o, getenv, &bytes.Buffer{})
	expectError(t, err, "invalid template checkout")
}

func TestTemplateRequestCreatesReusesAndMerges(t *testing.T) {
	e, api, _, _ := fixture(t)
	e.Project.Template = true
	const branch = "release/2.0.4"
	command(t, e.Project.Root, "switch", "-c", branch)
	command(t, e.Project.Root, "commit", "--allow-empty", "-m", "template release")
	head := command(t, e.Project.Root, "rev-parse", "HEAD")
	request := Request{ID: 15, State: "opened", SHA: head, Source: branch, Target: "main", Repository: e.Project.ID}
	created, merged := 0, 0
	api.requests = func(string, string, string) ([]Request, error) {
		if created == 0 {
			return nil, nil
		}
		return []Request{request}, nil
	}
	api.create = func(title, source string) (Request, error) {
		created++
		if title != "[REL] Release template 2.0.4" || source != branch {
			return Request{}, errors.New("unexpected template request title or branch")
		}
		return request, nil
	}
	api.request = func(int64) (Request, error) { return request, nil }
	api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
		if q != (PipelineQuery{Ref: branch, SHA: head, Source: "push"}) {
			return nil, errors.New("unexpected template CI query")
		}
		return []Pipeline{{ID: 1, Ref: branch, SHA: head, Status: "success"}}, nil
	}
	api.merge = func(id int64, sha string) (Request, error) {
		if id != request.ID || sha != head {
			return Request{}, errors.New("template merge was not SHA-pinned")
		}
		merged++
		command(t, e.Project.Root, "push", "origin", "HEAD:refs/heads/main")
		request.State = "merged"
		return request, nil
	}
	if err := e.Run(Options{Command: "mr-create"}); err != nil || created != 1 || merged != 0 {
		t.Fatalf("create err=%v created=%d merged=%d", err, created, merged)
	}
	if err := e.Run(Options{Command: "mr"}); err != nil || created != 1 || merged != 1 {
		t.Fatalf("merge err=%v created=%d merged=%d", err, created, merged)
	}
	if current := command(t, e.Project.Root, "branch", "--show-current"); current != "main" {
		t.Fatalf("current branch %s", current)
	}
	_, next := e.Advice(Options{Command: "mr"}, true)
	if !slices.Equal(next, []string{"pmtool", "template-test", "--module", e.Project.Root}) {
		t.Fatalf("wrong next command %v", next)
	}
}

func TestGitHubTemplatePipelineExcludesModuleAndNonPushRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var runs []record
		for i, tc := range []struct{ workflow, event string }{
			{"template-test", "push"},
			{"puppet-module", "push"},
			{"template-test", "pull_request"},
		} {
			runs = append(runs, record{"id": i + 1, "path": ".github/workflows/" + tc.workflow + ".yml",
				"event": tc.event, "head_branch": "release/2.0.4", "head_sha": "head", "status": "completed", "conclusion": "success"})
		}
		writeJSON(t, w, record{"workflow_runs": runs})
	}))
	t.Cleanup(server.Close)
	client := NewClient(Project{Provider: "github", ID: "team/template", Endpoint: server.URL, Template: true}, "fixture-token")
	pipelines, err := client.Pipelines(t.Context(), PipelineQuery{Ref: "release/2.0.4", SHA: "head", Source: "push"})
	if err != nil || len(pipelines) != 1 || pipelines[0].ID != 1 {
		t.Fatalf("pipelines=%+v err=%v", pipelines, err)
	}
}

func TestGitLabTemplateRequestsSetSourceBranchDeletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body record
		if r.Method != http.MethodGet {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				http.Error(w, "invalid fixture request", http.StatusBadRequest)
				return
			}
		}
		state := "opened"
		switch r.Method {
		case http.MethodPost:
			if !boolean(body, "remove_source_branch") || text(body, "source_branch") != "release/2.0.4" || text(body, "target_branch") != "main" {
				t.Errorf("unexpected create payload: %v", body)
			}
		case http.MethodPut:
			if !boolean(body, "should_remove_source_branch") || text(body, "sha") != "head" {
				t.Errorf("unexpected merge payload: %v", body)
			}
		case http.MethodGet:
			state = "merged"
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
		writeJSON(t, w, record{"iid": 15, "state": state})
	}))
	t.Cleanup(server.Close)
	client := NewClient(Project{Provider: "gitlab", ID: "team/template", Endpoint: server.URL, Template: true}, "fixture-token")
	request, err := client.CreateRequest(t.Context(), "[REL] Release template 2.0.4", "release/2.0.4")
	if err != nil || request.ID != 15 {
		t.Fatalf("create: request=%+v err=%v", request, err)
	}
	request, err = client.MergeRequest(t.Context(), request.ID, "head")
	if err != nil || request.State != "merged" {
		t.Fatalf("merge: request=%+v err=%v", request, err)
	}
}

func TestGitLabSameNamedForkRequestIsNotSelected(t *testing.T) {
	const branch = "release/2.0.4"
	fork := record{"iid": 15, "state": "opened", "sha": "head", "source_branch": branch, "target_branch": "main",
		"source_project_id": 200, "target_project_id": 100}
	sameProject := record{"iid": 16, "state": "opened", "sha": "head", "source_branch": branch, "target_branch": "main",
		"source_project_id": 100, "target_project_id": 100}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/15") {
			writeJSON(t, w, fork)
			return
		}
		writeJSON(t, w, []record{fork, sameProject})
	}))
	t.Cleanup(server.Close)
	project := Project{Provider: "gitlab", ID: "team/template", Endpoint: server.URL, Template: true}
	client := NewClient(project, "fixture-token")
	requests, err := client.Requests(t.Context(), "opened", branch, "main")
	if err != nil || len(requests) != 1 || requests[0].ID != 16 {
		t.Fatalf("requests=%+v err=%v", requests, err)
	}
	e := &Engine{API: client, Context: t.Context(), Project: project}
	_, err = e.openRequest(15, branch)
	expectError(t, err, "merge request source, target, or state changed")
}

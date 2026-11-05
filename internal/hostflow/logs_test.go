package hostflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJobLogProviderEndpointsAndGitHubRedirectIsolation(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		t.Run(provider, func(t *testing.T) {
			requests := 0
			download := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for _, key := range []string{"Authorization", "PRIVATE-TOKEN", "Cookie", "X-GitHub-Api-Version"} {
					if r.Header.Get(key) != "" {
						t.Errorf("download received %s", key)
					}
				}
				if r.URL.Query().Get("signature") != "signed-fixture" {
					t.Error("lost signed query")
				}
				_, _ = io.WriteString(w, "downloaded log\n")
			}))
			defer download.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				want := "/projects/team/project/jobs/42/trace"
				if provider == "github" {
					want = "/repos/team/project/actions/jobs/42/logs"
				}
				if r.URL.Path != want || r.Method != http.MethodGet {
					t.Errorf("unexpected request %s", r.URL)
				}
				if provider == "github" {
					if r.Header.Get("Authorization") != "Bearer fixture-token" {
						t.Error("missing API token")
					}
					http.Redirect(w, r, download.URL+"/logs?signature=signed-fixture", http.StatusFound)
				} else {
					if r.Header.Get("PRIVATE-TOKEN") != "fixture-token" {
						t.Error("missing API token")
					}
					_, _ = io.WriteString(w, "downloaded log\n")
				}
			}))
			defer api.Close()
			c := NewClient(Project{Provider: provider, ID: "team/project", Endpoint: api.URL}, "fixture-token")
			c.HTTP.Transport = download.Client().Transport
			body, err := c.JobLog(testContext(t), 42)
			if err != nil || string(body) != "downloaded log\n" || requests != 1 {
				t.Fatalf("log download: %q %v requests=%d", body, err, requests)
			}
		})
	}
}

func TestJobLogRefusesUnsafeRedirectsStatusesAndOversizedBodies(t *testing.T) {
	for _, tc := range []struct {
		name, provider, location string
		status                   int
		body                     string
	}{
		{"gitlab redirect", "gitlab", "https://example.invalid/log", 302, ""},
		{"http redirect", "github", "http://example.invalid/log", 302, ""},
		{"relative redirect", "github", "/log", 302, ""},
		{"credentials", "github", "https://name:secret@example.invalid/log", 302, ""},
		{"fragment", "github", "https://example.invalid/log#secret", 302, ""},
		{"missing", "gitlab", "", 404, ""},
		{"forbidden", "github", "", 403, "secret response"},
		{"oversized", "gitlab", "", 200, strings.Repeat("x", maxLogBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", tc.location)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer s.Close()
			c := NewClient(Project{Provider: tc.provider, ID: "team/project", Endpoint: s.URL}, "fixture-token")
			if _, err := c.JobLog(testContext(t), 42); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe log accepted or sensitive error: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	c := NewClient(Project{Provider: "gitlab", ID: "x", Endpoint: "https://example.invalid"}, "token")
	if _, err := c.JobLog(ctx, 0); err == nil {
		t.Fatal("invalid job accepted")
	}
	if _, err := c.JobLog(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func logsFixture(t *testing.T) (*Engine, *fakeHosting) {
	t.Helper()
	e, api, _, _ := fixture(t)
	api.pipeline = func(id int64) (Pipeline, error) {
		return Pipeline{ID: id, Ref: "main", SHA: "fixture-sha", Status: "failed"}, nil
	}
	api.jobs = func(int64) ([]Job, error) {
		return []Job{{ID: 41, Name: "audit", Status: "failed"}, {ID: 42, Name: "unit", Status: "success"}}, nil
	}
	api.jobLog = func(id int64) ([]byte, error) { return []byte(fmt.Sprintf("first\njob %d\nlast\n", id)), nil }
	return e, api
}

func TestPipelineLogsSelectsMembershipTailAndExclusiveOutput(t *testing.T) {
	e, api := logsFixture(t)
	path := filepath.Join(t.TempDir(), "logs.txt")
	o := Options{Command: "pipeline-logs", PipelineID: 7, Failed: true, Tail: 1, Output: path}
	before := command(t, e.Project.Root, "status", "--porcelain")
	if err := e.Run(o); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(body), "job 41") || strings.Contains(string(body), "job 42") || strings.Contains(string(body), "first") {
		t.Fatalf("wrong selection: %s %v", body, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("log permissions")
	}
	expectError(t, e.Run(o), "exist")
	o.Output = ""
	o.JobID = 99
	api.jobLog = func(int64) ([]byte, error) { t.Fatal("foreign job downloaded"); return nil, nil }
	expectError(t, e.Run(o), "not selected")
	o.JobID = 0
	o.Ref = "other"
	expectError(t, e.Run(o), "ref does not match")
	if after := command(t, e.Project.Root, "status", "--porcelain"); after != before {
		t.Fatal("log reading mutated checkout")
	}
}

func TestPipelineLogsRedactsBeforeTailingAndPreservesFailure(t *testing.T) {
	e, api := logsFixture(t)
	secret := e.Git.Token
	api.jobLog = func(int64) ([]byte, error) {
		return []byte("\x1b[31mfirst\x1b[0m\n" + secret + "\nhttps://user:password@example.invalid/\n" + base64.StdEncoding.EncodeToString([]byte("oauth2:"+secret)) + "\nlast\n"), nil
	}
	body, err := e.collectLogs(Pipeline{ID: 7}, Options{Failed: true, Tail: 0})
	if err != nil || strings.Contains(string(body), secret) || strings.Contains(string(body), "password") || strings.Contains(string(body), "\x1b") || !strings.Contains(string(body), "[REDACTED]") {
		t.Fatalf("unsafe output: %q %v", body, err)
	}
	api.jobLog = func(int64) ([]byte, error) { return nil, fmt.Errorf("expired") }
	path := filepath.Join(t.TempDir(), "absent")
	expectError(t, e.Run(Options{Command: "pipeline-logs", PipelineID: 7, Output: path}), "expired")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed download created output")
	}
	e.failedLogs(Pipeline{ID: 7})
	if !strings.Contains(e.Out.(*bytes.Buffer).String(), "[WARN]") {
		t.Fatal("automatic log error not reported")
	}
}

func TestPipelineLogArguments(t *testing.T) {
	root := t.TempDir()
	o, err := Parse([]string{"pipeline-logs", "--pipeline", "7", "--job=42", "--failed", "--tail", "0", "--output", "logs.txt"}, root)
	if err != nil || o.PipelineID != 7 || o.JobID != 42 || !o.Failed || o.Tail != 0 || o.Output != "logs.txt" {
		t.Fatalf("parse: %+v %v", o, err)
	}
	for _, flags := range [][]string{{"--pipeline", "0"}, {"--job", "-1"}, {"--tail", "100001"}, {"--failed=maybe"}, {"--watch", "5"}, {"--output", ""}} {
		if _, err := Parse(append([]string{"pipeline-logs"}, flags...), root); err == nil {
			t.Fatalf("accepted %v", flags)
		}
	}
}

func TestLogsResolveRefAndRejectForeignPipelineBeforeDownload(t *testing.T) {
	e, api := logsFixture(t)
	api.branch = func(ref string) (Branch, error) { return Branch{Name: ref, SHA: "head-sha"}, nil }
	api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
		if q.Ref != "main" || q.SHA != "head-sha" || q.Source != "push" {
			t.Fatalf("unscoped pipeline query: %+v", q)
		}
		return []Pipeline{{ID: 7, Ref: "main", SHA: "head-sha"}}, nil
	}
	if err := e.Run(Options{Command: "pipeline-logs", Tail: 1}); err != nil {
		t.Fatal(err)
	}
	api.pipeline = func(int64) (Pipeline, error) { return Pipeline{ID: 99, SHA: "head-sha", Ref: "main"}, nil }
	api.jobs = func(int64) ([]Job, error) { t.Fatal("jobs requested for a different pipeline"); return nil, nil }
	expectError(t, e.Run(Options{Command: "pipeline-logs", PipelineID: 7}), "identity")
}

func TestAutomaticFailedLogsNeverPermitMergeOrTagAndPreservePublication(t *testing.T) {
	for _, stage := range []string{"source", "main", "publication"} {
		for _, expired := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/expired=%t", stage, expired), func(t *testing.T) {
				e, api, out, remote := fixture(t)
				sha := command(t, e.Project.Root, "rev-parse", "HEAD")
				api.jobs = func(int64) ([]Job, error) { return []Job{{ID: 42, Name: "audit", Status: "failed"}}, nil }
				api.jobLog = func(id int64) ([]byte, error) {
					if id != 42 {
						t.Fatal("wrong job")
					}
					if expired {
						return nil, ErrNotFound
					}
					return []byte("diagnostic fixture\n"), nil
				}
				api.pipelines = func(q PipelineQuery) ([]Pipeline, error) {
					if q.Status != "" {
						return nil, nil
					}
					status := "failed"
					if stage == "publication" && q.Ref == "main" {
						status = "success"
					}
					return []Pipeline{{ID: 7, Ref: q.Ref, SHA: sha, Status: status}}, nil
				}
				var err error
				if stage == "source" {
					command(t, e.Project.Root, "switch", "-c", "release/1.0.1")
					r := Request{ID: 3, SHA: sha, Source: "release/1.0.1", Target: "main", State: "opened"}
					api.requests = func(string, string, string) ([]Request, error) { return []Request{r}, nil }
					api.request = func(int64) (Request, error) { return r, nil }
					api.merge = func(int64, string) (Request, error) { t.Fatal("failed CI merged"); return Request{}, nil }
					err = e.Run(Options{Command: "mr"})
				} else {
					err = e.Run(Options{Command: "release-tag"})
				}
				expectError(t, err, "pipeline 7 is failed")
				want := "diagnostic fixture"
				if expired {
					want = "[WARN]"
				}
				if !strings.Contains(out.String(), want) {
					t.Fatalf("missing log evidence: %s", out)
				}
				tags := command(t, remote, "tag", "--list")
				if stage == "publication" {
					if tags != "v1.0.0" || e.Recovery != "pipeline-jobs" {
						t.Fatal("published tag/recovery changed")
					}
				} else if tags != "" {
					t.Fatal("failed source/main CI created a tag")
				}
			})
		}
	}
}

func TestOutputSymlinksAndDuplicateJobsAreRefused(t *testing.T) {
	e, api := logsFixture(t)
	root := t.TempDir()
	target := filepath.Join(root, "original")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(Options{Command: "pipeline-logs", PipelineID: 7, Output: link}); err == nil {
		t.Fatal("symlink overwritten")
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != "untouched" {
		t.Fatal("symlink target changed")
	}
	api.jobs = func(int64) ([]Job, error) { return []Job{{ID: 42}, {ID: 42}}, nil }
	expectError(t, e.Run(Options{Command: "pipeline-logs", PipelineID: 7}), "duplicate")
}

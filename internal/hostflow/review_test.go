package hostflow

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMergeRechecksNewHeadAfterWaitingForReview(t *testing.T) {
	const branch = "release/3.1.0"
	r := Request{ID: 10, Source: branch, Target: "main", State: "opened", SHA: "old", URL: "https://git.example.test/mr/10"}
	var queried, reviewed []string
	api := &fakeHosting{
		request: func(int64) (Request, error) { return r, nil },
		pipelines: func(q PipelineQuery) ([]Pipeline, error) {
			if q.Ref != branch || q.SHA != r.SHA || q.Source != "push" {
				t.Fatalf("unbound pipeline query: %+v", q)
			}
			queried = append(queried, q.SHA)
			if len(queried) == 2 {
				return nil, nil
			}
			return []Pipeline{{ID: 1, Ref: q.Ref, SHA: q.SHA, Status: "success"}}, nil
		},
		review: func(r Request) (ReviewStatus, error) {
			reviewed = append(reviewed, r.SHA)
			if r.SHA == "old" {
				return ReviewStatus{SHA: r.SHA, WaitingFor: "required approval"}, nil
			}
			return ReviewStatus{SHA: r.SHA}, nil
		},
	}
	out := &bytes.Buffer{}
	e := &Engine{API: api, Context: t.Context(), Out: out, Sleep: func(context.Context, time.Duration) error {
		r.SHA = "new"
		return nil
	}}
	sha, err := e.waitMergeReady(r.ID, branch)
	if err != nil || sha != "new" || !slices.Equal(queried, []string{"old", "new", "new"}) ||
		!slices.Equal(reviewed, []string{"old", "new"}) || !strings.Contains(out.String(), "required approval") {
		t.Fatalf("sha=%s CI=%v review=%v output=%q err=%v", sha, queried, reviewed, out, err)
	}
}

func TestMergeReviewCancellationAndManualCI(t *testing.T) {
	for _, status := range []string{"success", "manual"} {
		t.Run(status, func(t *testing.T) {
			r := Request{ID: 10, Source: "release/3.1.0", Target: "main", State: "opened", SHA: "head"}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reviewCalled := false
			api := &fakeHosting{
				request: func(int64) (Request, error) { return r, nil },
				pipelines: func(PipelineQuery) ([]Pipeline, error) {
					return []Pipeline{{ID: 1, Ref: r.Source, SHA: r.SHA, Status: status}}, nil
				},
				review: func(Request) (ReviewStatus, error) {
					reviewCalled = true
					return ReviewStatus{SHA: r.SHA, WaitingFor: "required approval"}, nil
				},
			}
			e := &Engine{API: api, Context: ctx, Out: &bytes.Buffer{}, Sleep: func(context.Context, time.Duration) error {
				cancel()
				return ctx.Err()
			}}
			sha, err := e.waitMergeReady(r.ID, r.Source)
			if sha != "" || reviewCalled != (status == "success") {
				t.Fatalf("unexpected merge readiness %q; reviewed=%t", sha, reviewCalled)
			}
			if status == "manual" {
				expectError(t, err, "source pipeline 1 is manual")
			} else if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation, got %v", err)
			}
		})
	}
}

func TestReviewProviders(t *testing.T) {
	gitlab := &Client{Project: Project{Provider: "gitlab"}}
	for _, state := range []string{"not_approved", "approvals_syncing", "mergeable"} {
		got, err := gitlab.Review(t.Context(), Request{SHA: "head", DetailedMergeStatus: state})
		if err != nil || got.SHA != "head" || (got.WaitingFor != "") != (state != "mergeable") {
			t.Fatalf("GitLab %s: %+v %v", state, got, err)
		}
	}
	for _, tc := range []struct{ field, wait, failure string }{
		{`,"reviewDecision":"REVIEW_REQUIRED"`, "required GitHub approval", ""},
		{`,"reviewDecision":"CHANGES_REQUESTED"`, "changes requested", ""},
		{`,"reviewDecision":"APPROVED"`, "", ""},
		{`,"reviewDecision":null`, "", ""},
		{"", "", "incomplete review decision"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/graphql" || r.Header.Get("Authorization") == "" {
				t.Errorf("unexpected GraphQL request: %s %s", r.Method, r.URL.Path)
			}
			_, err := w.Write([]byte(`{"data":{"repository":{"pullRequest":{"headRefOid":"head"` + tc.field + `}}}}`))
			if err != nil {
				t.Error(err)
			}
		}))
		client := &Client{Project: Project{Provider: "github", ID: "team/module", Endpoint: server.URL + "/api/v3"}, Token: "test", HTTP: server.Client()}
		got, err := client.Review(t.Context(), Request{ID: 10})
		server.Close()
		if tc.failure != "" {
			expectError(t, err, tc.failure)
		} else if err != nil || got.SHA != "head" || !strings.Contains(got.WaitingFor, tc.wait) || (got.WaitingFor == "") != (tc.wait == "") {
			t.Fatalf("GitHub %s: %+v %v", tc.field, got, err)
		}
	}
}

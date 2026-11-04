package hostflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiMutation struct {
	Method, Path string
	Body         record
}
type policyFixture struct {
	Provider, Default, MainSHA, MasterSHA string
	Main, Master, ProtectedMaster         bool
	MainProtection                        record
	Tags, Rules, Open                     []record
	Writes                                []apiMutation
	Fail                                  string
	client                                *Client
	t                                     *testing.T
}

func newPolicyFixture(t *testing.T, provider string) *policyFixture {
	t.Helper()
	f := &policyFixture{Provider: provider, Default: "main", Main: true, Master: true, MainSHA: "abc", MasterSHA: "abc", t: t}
	f.MainProtection = record{"name": "main", "allow_force_push": false, "push_access_levels": []record{{"id": 1, "access_level": 0}}, "merge_access_levels": []record{{"id": 2, "access_level": 40}}, "unprotect_access_levels": []record{{"id": 3, "access_level": 40}}}
	f.Tags = []record{{"name": "v*", "create_access_levels": []record{{"access_level": 40}}}}
	s := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(s.Close)
	f.client = NewClient(Project{Endpoint: s.URL, ID: "team/puppet-sample", Name: "puppet-sample", Provider: provider}, "scoped-token")
	return f
}
func (f *policyFixture) handle(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	prefix := "/projects/team/puppet-sample"
	if f.Provider == "github" {
		prefix = "/repos/team/puppet-sample"
	}
	path = strings.TrimPrefix(path, prefix)
	if f.Fail != "" && strings.Contains(path, f.Fail) {
		w.WriteHeader(503)
		return
	}
	body := record{}
	if r.Method != "GET" {
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				f.t.Error(err)
			}
		}
		f.Writes = append(f.Writes, apiMutation{r.Method, path, body})
	}
	branch := func(name, sha string, exists bool) {
		if !exists {
			w.WriteHeader(404)
			return
		}
		writeJSON(f.t, w, record{"name": name, "commit": record{"id": sha, "sha": sha}, "protected": name == "master" && f.ProtectedMaster})
	}
	switch {
	case path == "":
		if r.Method != "GET" {
			f.Default = text(body, "default_branch")
		}
		writeJSON(f.t, w, record{"default_branch": f.Default})
	case strings.HasSuffix(path, "/branches/main"):
		branch("main", f.MainSHA, f.Main)
	case strings.HasSuffix(path, "/branches/master") && r.Method == "GET":
		branch("master", f.MasterSHA, f.Master)
	case path == "/repository/branches" || path == "/git/refs":
		f.Main = true
		writeJSON(f.t, w, record{})
	case path == "/protected_branches/main":
		if f.MainProtection == nil {
			w.WriteHeader(404)
			return
		}
		writeJSON(f.t, w, f.MainProtection)
	case path == "/protected_branches":
		f.MainProtection = body
		writeJSON(f.t, w, body)
	case path == "/protected_tags":
		if r.Method == "GET" {
			writeJSON(f.t, w, f.Tags)
		} else {
			writeJSON(f.t, w, body)
		}
	case strings.HasPrefix(path, "/protected_tags/"):
		w.WriteHeader(204)
	case path == "/rulesets":
		if r.Method == "GET" {
			writeJSON(f.t, w, f.Rules)
		} else {
			writeJSON(f.t, w, body)
		}
	case strings.HasPrefix(path, "/rulesets/"):
		writeJSON(f.t, w, body)
	case path == "/merge_requests" || path == "/pulls":
		writeJSON(f.t, w, f.Open)
	case path == "/protected_branches/master":
		w.WriteHeader(204)
	case path == "/repository/branches/master" || path == "/git/refs/heads/master":
		f.Master = false
		w.WriteHeader(204)
	default:
		f.t.Errorf("unexpected policy endpoint: %s %s", r.Method, path)
		w.WriteHeader(500)
	}
}
func TestMigratedProviderPolicyApplyPreviewAndDivergence(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		for _, scenario := range []string{"preview", "create main", "missing both", "divergent default", "divergent master", "existing protections", "api failure"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				f := newPolicyFixture(t, provider)
				switch scenario {
				case "preview", "create main":
					f.Main = false
					f.Default = "master"
					f.MainProtection = nil
					f.Tags = nil
				case "missing both":
					f.Main, f.Master = false, false
				case "divergent default":
					f.Default = "other"
				case "divergent master":
					f.Default = "master"
					f.MasterSHA = "other"
				case "api failure":
					f.Fail = "main"
				}
				err := f.client.Policy(testContext(t), scenario == "preview", false, testingWriter{t}, PolicyLevels{40, 40, 40})
				if strings.HasPrefix(scenario, "divergent") || scenario == "missing both" || scenario == "api failure" {
					if err == nil || len(f.Writes) > 0 {
						t.Fatal("policy refusal mutated remote", err, f.Writes)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "preview" {
					if len(f.Writes) > 0 {
						t.Fatal("preview wrote remote policy")
					}
					return
				}
				if scenario == "create main" && (!f.Main || f.Default != "main") {
					t.Fatal("main/default migration missing")
				}
				for _, change := range f.Writes {
					if strings.Contains(change.Path, "rulesets") {
						actors, ok := change.Body["bypass_actors"].([]any)
						if !ok || len(actors) != 0 {
							t.Fatal("ruleset permits bypass", change.Body)
						}
						if text(change.Body, "target") == "tag" {
							rules := array(change.Body, "rules")
							if len(rules) != 2 || text(object(record{"rule": rules[0]}, "rule"), "type") != "deletion" || text(object(record{"rule": rules[1]}, "rule"), "type") != "update" {
								t.Fatal("tag deletion/update protection lost")
							}
						}
					}
				}
			})
		}
	}
}
func TestMigratedGitLabAccessUpdatesAndTagReplacement(t *testing.T) {
	f := newPolicyFixture(t, "gitlab")
	f.MainProtection = record{"name": "main", "allow_force_push": true, "push_access_levels": []record{{"id": 11, "access_level": 40}, {"id": 12, "access_level": 30}}, "merge_access_levels": []record{{"id": 21, "access_level": 30}}, "unprotect_access_levels": []record{}}
	f.Tags = []record{{"name": "v*", "create_access_levels": []record{{"access_level": 30}}}}
	if err := f.client.Policy(testContext(t), false, false, testingWriter{t}, PolicyLevels{40, 50, 40}); err != nil {
		t.Fatal(err)
	}
	if len(f.Writes) != 3 {
		t.Fatal(f.Writes)
	}
	patch := f.Writes[0]
	if patch.Method != "PATCH" || boolean(patch.Body, "allow_force_push") {
		t.Fatal(patch)
	}
	push := array(patch.Body, "allowed_to_push")
	if len(push) != 2 {
		t.Fatal(push)
	}
	first, _ := push[0].(map[string]any)
	extra, _ := push[1].(map[string]any)
	if number(first, "id") != 11 || number(first, "access_level") != 0 || !boolean(extra, "_destroy") {
		t.Fatal(push)
	}
	if f.Writes[1].Method != "DELETE" || f.Writes[2].Method != "POST" || number(f.Writes[2].Body, "create_access_level") != 40 {
		t.Fatal(f.Writes)
	}
}
func TestMigratedGitHubRulesetReuseAndAmbiguity(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "reuse", true: "ambiguous"}[duplicate], func(t *testing.T) {
			f := newPolicyFixture(t, "github")
			f.Rules = []record{{"id": 1, "name": "puppet-tooling-main"}, {"id": 2, "name": "puppet-tooling-release-tags"}}
			if duplicate {
				f.Rules = append(f.Rules, record{"id": 3, "name": "puppet-tooling-main"})
			}
			err := f.client.Policy(testContext(t), false, false, testingWriter{t}, PolicyLevels{40, 40, 40})
			if duplicate {
				if err == nil || len(f.Writes) != 0 {
					t.Fatal("duplicate rulesets mutated remote", err)
				}
				return
			}
			if err != nil || len(f.Writes) != 2 {
				t.Fatal(err, f.Writes)
			}
			for _, w := range f.Writes {
				if w.Method != "PUT" {
					t.Fatal("rulesets recreated instead of updated", w)
				}
			}
		})
	}
}
func TestMigratedMasterDeletionGuards(t *testing.T) {
	for _, provider := range []string{"gitlab", "github"} {
		for _, scenario := range []string{"preview", "delete", "divergent", "open request", "protected"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				f := newPolicyFixture(t, provider)
				switch scenario {
				case "divergent":
					f.MasterSHA = "different"
				case "open request":
					if provider == "github" {
						f.Open = []record{{"number": 7, "state": "open", "head": record{"ref": "feature", "repo": record{"full_name": "fork/puppet-sample"}}, "base": record{"ref": "master"}}}
					} else {
						f.Open = []record{{"iid": 7, "state": "opened", "source_branch": "feature", "target_branch": "master"}}
					}
				case "protected":
					f.ProtectedMaster = true
				}
				err := f.client.Policy(testContext(t), scenario == "preview", true, testingWriter{t}, PolicyLevels{40, 40, 40})
				refused := scenario == "divergent" || scenario == "open request" || provider == "github" && scenario == "protected"
				if refused {
					if err == nil || !f.Master {
						t.Fatal("master guard bypassed", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "preview" {
					if len(f.Writes) > 0 || !f.Master {
						t.Fatal("deletion preview mutated remote")
					}
					return
				}
				if f.Master {
					t.Fatal("master was not removed")
				}
				if provider == "gitlab" && scenario == "protected" {
					unprotect, deleted := -1, -1
					for i, v := range f.Writes {
						if v.Path == "/protected_branches/master" {
							unprotect = i
						}
						if v.Path == "/repository/branches/master" {
							deleted = i
						}
					}
					if unprotect < 0 || deleted <= unprotect {
						t.Fatal("protected master removed before unprotect")
					}
				}
			})
		}
	}
}

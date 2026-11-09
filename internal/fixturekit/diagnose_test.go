package fixturekit

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnoseForgeCatalogAndArchiveFailures(t *testing.T) {
	for _, tc := range []struct {
		name, release, metadata, failure string
		catalogStatus, archiveStatus     int
	}{
		{"ready", "1.1.0", "neon-common", "", 200, 206},
		{"unauthorized", "", "", "authorization failed", 401, 0},
		{"missing", "1.0.0", "neon-common", "requested release not found", 200, 0},
		{"metadata", "1.1.0", "", "incomplete or inconsistent release metadata", 200, 0},
		{"archive", "1.1.0", "neon-common", "archive unavailable", 200, 404},
		{"archive-html", "1.1.0", "neon-common", "does not start with a gzip header", 200, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, ".fixtures.yml"), "fixtures:\n  resolve_dependencies: true\n  forge_modules:\n    common:\n      repo: neon/common\n      ref: '1.1.0'\n")
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer read-token" {
					t.Error("expected authenticated read")
				}
				var data string
				switch r.URL.Path {
				case "/v3/releases":
					if r.URL.Query().Get("module") == "puppetlabs-apt" {
						data = `{"pagination":{"next":null},"results":[{"file_uri":"/v3/files/apt.tar.gz","file_md5":"sum","metadata":{"name":"puppetlabs-apt","version":"11.2.0"}}]}`
					} else {
						if r.URL.Query().Get("module") != "neon-common" {
							t.Errorf("unexpected catalog query %v", r.URL.Query())
						}
						w.WriteHeader(tc.catalogStatus)
						data = `{"pagination":{"next":null},"results":[{"file_uri":"/v3/files/common.tar.gz","file_sha256":"sum","metadata":{"name":"` + tc.metadata + `","version":"` + tc.release + `","dependencies":[{"name":"puppetlabs/apt","version_requirement":">= 11.2.0 < 12.0.0"}]}}]}`
					}
				case "/v3/files/common.tar.gz":
					if tc.archiveStatus == 0 || r.Header.Get("Range") != "bytes=0-1" {
						t.Errorf("unexpected archive probe")
						http.NotFound(w, r)
						return
					}
					w.WriteHeader(tc.archiveStatus)
					data = "\x1f\x8b"
					if tc.archiveStatus == http.StatusOK {
						data = "<html>not an archive</html>"
					}
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if _, err := w.Write([]byte(data)); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			env := environment(map[string]string{"FORGE_URL": server.URL, "FORGE_TOKEN": "read-token"})
			report, err := diagnoseForge(t.Context(), root, env, server.Client().Transport)
			if tc.failure != "" {
				if err == nil || !strings.Contains(err.Error(), tc.failure) {
					t.Fatalf("expected %q, got %v", tc.failure, err)
				}
			} else if err != nil || report != (ForgeReport{Catalogs: 2, Archives: 1, Unpinned: 1}) {
				t.Fatalf("report=%+v err=%v", report, err)
			}
		})
	}
}

func TestDiagnoseForgeArchiveRedirectDoesNotForwardToken(t *testing.T) {
	cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Range") != "bytes=0-1" {
			t.Error("CDN must receive a range request without Forge credentials")
		}
		if _, err := w.Write([]byte{0x1f, 0x8b}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(cdn.Close)
	forge := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer read-token" {
			t.Error("missing Forge credentials")
		}
		http.Redirect(w, r, cdn.URL+"/archive?signed=example", http.StatusFound)
	}))
	t.Cleanup(forge.Close)
	base, err := url.Parse(forge.URL)
	if err != nil {
		t.Fatal(err)
	}
	probe := newForgeProbe(base, "read-token", forge.Client().Transport)
	if err := probe.archive(t.Context(), "/v3/files/module.tar.gz"); err != nil {
		t.Fatal(err)
	}
}

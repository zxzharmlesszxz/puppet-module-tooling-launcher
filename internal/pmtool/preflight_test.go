package pmtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplatePreflightStopsBeforeDockerOrConfig(t *testing.T) {
	for _, row := range []struct{ config, token, missing string }{
		{"", "", "template.use"},
		{"template:\n  use: git.example.test/team/template@v1.0.0\n", "not-printed-secret", "TOOLING_TEMPLATE_TOKEN_HOST"},
	} {
		t.Run(row.missing, func(t *testing.T) {
			a, d, out := testApp(t)
			a.templateOperation = nil
			a.getenv = func(key string) string {
				if key == "TOOLING_TEMPLATE_TOKEN" {
					return row.token
				}
				return ""
			}
			if row.config != "" {
				if err := os.WriteFile(filepath.Join(a.cwd, ".sync.yml"), []byte(row.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			expectFailure(t, row.missing, func() { a.run([]string{"sync"}) })
			if len(d.calls)+len(d.plans)+len(d.queries) != 0 {
				t.Fatal("preflight started an external tool")
			}
			if strings.Contains(out.String(), row.token) && row.token != "" {
				t.Fatal("preflight printed a credential")
			}
			if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
				t.Fatal("preflight created image config", err)
			}
		})
	}
}
func TestForgePreflightReportsAllMissingNames(t *testing.T) {
	a, d, _ := testApp(t)
	err := a.preflightForge(false)
	if err == nil || !strings.Contains(err.Error(), "FORGE_URL") || !strings.Contains(err.Error(), "FORGE_PUBLISH_TOKEN") {
		t.Fatal(err)
	}
	a.getenv = func(key string) string {
		if key == "FORGE_URL" {
			return "https://forge.example.test"
		}
		return ""
	}
	if err := a.preflightForge(true); err != nil {
		t.Fatal("preview required a publish credential", err)
	}
	if len(d.calls)+len(d.plans)+len(d.queries) != 0 {
		t.Fatal("preflight contacted a worker")
	}
}
func TestAuthenticatedForgePreflightRunsBeforeSDK(t *testing.T) {
	a, d, _ := testApp(t)
	body := "fixtures:\n  forge_modules:\n    common: {repo: team/common, ref: '1.0.0'}\n"
	if err := os.WriteFile(filepath.Join(a.cwd, ".fixtures.yml"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	a.getenv = func(key string) string {
		if key == "FORGE_URL" {
			return "https://forge.example.test"
		}
		return ""
	}
	expectFailure(t, "FORGE_TOKEN", func() { a.run([]string{"voxbox", "--stage", "unit"}) })
	if len(d.calls)+len(d.plans)+len(d.queries) != 0 {
		t.Fatal("missing Forge read token started Docker")
	}
	if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
		t.Fatal("preflight created image configuration", err)
	}
}

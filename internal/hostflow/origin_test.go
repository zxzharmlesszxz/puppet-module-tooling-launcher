package hostflow

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configuredOriginClone(t *testing.T, origin string) string {
	t.Helper()
	root := t.TempDir()
	command(t, root, "init", "--initial-branch=main")
	command(t, root, "remote", "add", "origin", origin)
	return root
}

func TestConfiguredOriginIgnoresLocalAndIncludedTransportRewrites(t *testing.T) {
	for _, host := range []string{"git.example.test", "github.com"} {
		for _, scope := range []string{"local", "included"} {
			t.Run(host+"/"+scope, func(t *testing.T) {
				origin := "https://" + host + "/team/puppet-sample.git"
				root := configuredOriginClone(t, origin)
				rewrite := "url.https://gitlab-ci-token:fixture-rewrite-token@" + host + "/.insteadOf"
				if scope == "included" {
					include := filepath.Join(root, ".git", "runner-config")
					command(t, root, "config", "--file", include, rewrite, "https://"+host+"/")
					command(t, root, "config", "--local", "include.path", include)
				} else {
					command(t, root, "config", "--local", rewrite, "https://"+host+"/")
				}
				if !strings.Contains(command(t, root, "remote", "get-url", "origin"), "fixture-rewrite-token@") {
					t.Fatal("fixture did not reproduce GitLab's credential-bearing transport rewrite")
				}
				before, err := os.ReadFile(filepath.Join(root, ".git", "config"))
				if err != nil {
					t.Fatal(err)
				}
				out := &bytes.Buffer{}
				e, err := New(testContext(t), Options{Module: root}, func(string) string { return "" }, out)
				if err != nil {
					t.Fatal("configured origin was rejected because of transport credentials", err)
				}
				provider := "gitlab"
				if host == "github.com" {
					provider = "github"
				}
				if e.Project.Origin != origin || e.Project.Remote != origin || e.Project.Provider != provider || e.Project.ID != "team/puppet-sample" {
					t.Fatal("transport rewrite changed repository identity", e.Project.ID, e.Project.Provider)
				}
				if err := e.origin(); err != nil {
					t.Fatal("unchanged configured origin failed its lifecycle guard", err)
				}
				if err := e.Run(Options{Command: "status"}); err != nil {
					t.Fatal(err)
				}
				after, err := os.ReadFile(filepath.Join(root, ".git", "config"))
				if err != nil || !bytes.Equal(before, after) || strings.Contains(out.String(), "fixture-rewrite-token") {
					t.Fatal("reading origin modified Git configuration or exposed credentials", err)
				}
				command(t, root, "remote", "set-url", "origin", "https://"+host+"/other/puppet-sample.git")
				expectError(t, e.origin(), "origin changed")
			})
		}
	}
}

func TestConfiguredOriginStillRejectsStoredCredentialsHiddenByRewrite(t *testing.T) {
	origin := "https://user:fixture-stored-secret@git.example.test/team/puppet-sample.git"
	root := configuredOriginClone(t, origin)
	command(t, root, "config", "--local", "url.https://git.example.test/.insteadOf", "https://user:fixture-stored-secret@git.example.test/")
	if strings.Contains(command(t, root, "remote", "get-url", "origin"), "fixture-stored-secret") {
		t.Fatal("fixture did not hide stored credentials behind a clean transport URL")
	}
	_, err := New(testContext(t), Options{Module: root}, func(string) string { return "" }, &bytes.Buffer{})
	expectError(t, err, "origin must not contain a password, query or fragment")
	if strings.Contains(err.Error(), "fixture-stored-secret") {
		t.Fatal("origin rejection exposed stored credentials")
	}
}

func TestConfiguredOriginRejectsMultipleURLsAndEmbeddedNewlines(t *testing.T) {
	for _, extra := range []string{"https://git.example.test/other/puppet-sample.git", "https://git.example.test/team/puppet-sample.git\nextra"} {
		root := configuredOriginClone(t, "https://git.example.test/team/puppet-sample.git")
		command(t, root, "config", "--local", "--add", "remote.origin.url", extra)
		_, err := New(testContext(t), Options{Module: root}, func(string) string { return "" }, &bytes.Buffer{})
		expectError(t, err, "exactly one URL")
	}
	for _, output := range []string{"", "\n", "https://git.example.test/team/puppet-sample.git\nextra\n", "https://git.example.test/team/puppet-sample.git\r\n"} {
		if _, err := ConfiguredOrigin(func(...string) (string, error) { return output, nil }); err == nil {
			t.Fatal("empty or multiline origin accepted")
		}
	}
	cause := errors.New("fixture config read refused")
	_, err := ConfiguredOrigin(func(...string) (string, error) { return "", cause })
	if !errors.Is(err, cause) {
		t.Fatal("origin lookup failure was lost", err)
	}
}

func TestConfiguredOriginRejectsStoredQueryAndFragment(t *testing.T) {
	for _, suffix := range []string{"?token=fixture-query-secret", "#fixture-fragment"} {
		root := configuredOriginClone(t, "https://git.example.test/team/puppet-sample.git"+suffix)
		_, err := New(testContext(t), Options{Module: root}, func(string) string { return "" }, &bytes.Buffer{})
		expectError(t, err, "origin must not contain a password, query or fragment")
		if strings.Contains(err.Error(), "fixture-") {
			t.Fatal("origin rejection exposed its query or fragment")
		}
	}
}

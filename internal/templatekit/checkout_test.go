package templatekit

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func credentialEnvironment() map[string]string {
	return map[string]string{"TOOLING_TEMPLATE_TOKEN": "fixture-token+/:%", "TOOLING_TEMPLATE_TOKEN_HOST": "git.example.test", "TOOLING_TEMPLATE_USERNAME": "oauth2"}
}
func archiveFixture(t *testing.T, root string) []byte {
	t.Helper()
	var data bytes.Buffer
	w := tar.NewWriter(&data)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := w.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Typeflag: tar.TypeReg, Size: int64(len(body)), Mode: 0644}); err != nil {
			return err
		}
		_, err = w.Write(body)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func TestCheckoutPinsFetchScopesCredentialsAndCleans(t *testing.T) {
	template, _ := fixture(t)
	archive := archiveFixture(t, template.Root)
	for _, reference := range []string{fixtureReference, "git.example.test/team/template@" + fixtureRevision} {
		var checkoutRoot string
		var operations []string
		env := credentialEnvironment()
		getenv := func(key string) string { return env[key] }
		runner := func(_ context.Context, root string, c Credentials, args ...string) ([]byte, error) {
			checkoutRoot = filepath.Dir(root)
			operations = append(operations, args[0])
			if args[0] == "fetch" {
				if c.Token != env["TOOLING_TEMPLATE_TOKEN"] || c.Repository != "git.example.test/team/template" {
					t.Fatal("fetch lost scoped auth", c.Repository)
				}
				if strings.Contains(strings.Join(args, " "), c.Token) {
					t.Fatal("token exposed in argv")
				}
				want := "refs/tags/v1.2.3"
				if strings.HasSuffix(reference, fixtureRevision) {
					want = fixtureRevision
				}
				if args[len(args)-1] != want || args[len(args)-2] != "https://git.example.test/team/template.git" {
					t.Fatal(args)
				}
			} else if c.Token != "" {
				t.Fatal("credentials reached non-fetch Git", args)
			}
			switch args[0] {
			case "rev-parse":
				return []byte(fixtureRevision + "\n"), nil
			case "archive":
				return archive, nil
			}
			return nil, nil
		}
		called := false
		err := Checkout(context.Background(), reference, getenv, runner, func(selected *Template) error {
			called = true
			if selected.Reference != reference || selected.Revision != fixtureRevision {
				t.Fatal("template identity changed")
			}
			return nil
		})
		if err != nil || !called || !reflect.DeepEqual(operations, []string{"init", "fetch", "rev-parse", "archive"}) {
			t.Fatal(operations, err)
		}
		if _, err := os.Stat(checkoutRoot); !os.IsNotExist(err) {
			t.Fatal("checkout leaked", checkoutRoot, err)
		}
		if !reflect.DeepEqual(env, credentialEnvironment()) {
			t.Fatal("caller environment mutated")
		}
	}
}
func TestCheckoutFailureDoesNotConsumeTemplateAndCleans(t *testing.T) {
	for _, stage := range []string{"init", "fetch", "rev-parse", "archive", "consume"} {
		t.Run(stage, func(t *testing.T) {
			template, _ := fixture(t)
			data := archiveFixture(t, template.Root)
			parent := ""
			runner := func(_ context.Context, root string, _ Credentials, args ...string) ([]byte, error) {
				parent = filepath.Dir(root)
				if args[0] == stage {
					return nil, fmt.Errorf("refused %s", stage)
				}
				switch args[0] {
				case "rev-parse":
					return []byte(fixtureRevision), nil
				case "archive":
					return data, nil
				}
				return nil, nil
			}
			err := Checkout(context.Background(), fixtureReference, func(string) string { return "" }, runner, func(*Template) error {
				if stage != "consume" {
					t.Fatal("failed checkout consumed template")
				}
				return fmt.Errorf("consume refused")
			})
			if err == nil {
				t.Fatal("checkout failure lost")
			}
			if _, err := os.Stat(parent); !os.IsNotExist(err) {
				t.Fatal("failed checkout leaked", err)
			}
		})
	}
}
func TestCredentialHelperProtocolHostPathAndOperationScope(t *testing.T) {
	env := credentialEnvironment()
	c, err := NewCredentials("git.example.test/team/template", func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "credential")
	if err := os.WriteFile(helper, []byte(credentialScript), 0700); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, dir, "credential"), c.Token) {
		t.Fatal("secret persisted in helper")
	}
	for _, row := range []struct {
		operation, protocol, host, path string
		allowed                         bool
	}{
		{"get", "https", "git.example.test", "team/template.git", true},
		{"store", "https", "git.example.test", "team/template.git", false},
		{"erase", "https", "git.example.test", "team/template.git", false},
		{"get", "http", "git.example.test", "team/template.git", false},
		{"get", "https", "other.example.test", "team/template.git", false},
		{"get", "https", "git.example.test:443", "team/template.git", false},
		{"get", "https", "git.example.test.evil.test", "team/template.git", false},
		{"get", "https", "git.example.test", "team/other.git", false},
		{"get", "https", "git.example.test", "team/template.git/extra", false},
	} {
		cmd := exec.Command(helper, row.operation)
		cmd.Env = append(cleanEnvironment(), "PMTOOL_TEMPLATE_TOKEN="+c.Token, "PMTOOL_TEMPLATE_USERNAME="+c.Username, "PMTOOL_TEMPLATE_REPOSITORY="+c.Repository)
		cmd.Stdin = strings.NewReader(fmt.Sprintf("protocol=%s\nhost=%s\npath=%s\n\n", row.protocol, row.host, row.path))
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if row.allowed {
			if string(out) != "username=oauth2\npassword="+c.Token+"\n" {
				t.Fatal("credential response changed")
			}
		} else if len(out) > 0 {
			t.Fatal("helper leaked or persisted credentials", row)
		}
	}
}
func TestCredentialsValidationAnonymousIsolationAndRedaction(t *testing.T) {
	moduleOnly := map[string]string{"GITLAB_TOKEN": "module-token", "GITHUB_TOKEN": "another-token"}
	c, err := NewCredentials("git.example.test/team/template", func(k string) string { return moduleOnly[k] })
	if err != nil || c.Token != "" {
		t.Fatal("module token reused", err)
	}
	for _, override := range []map[string]string{{"TOOLING_TEMPLATE_TOKEN_HOST": ""}, {"TOOLING_TEMPLATE_TOKEN_HOST": "other.example.test"}, {"TOOLING_TEMPLATE_TOKEN_HOST": "https://git.example.test"}, {"TOOLING_TEMPLATE_TOKEN": "bad\nvalue"}, {"TOOLING_TEMPLATE_USERNAME": "bad\rvalue"}, {"TOOLING_TEMPLATE_TOKEN": "bad\x00value"}} {
		env := credentialEnvironment()
		maps.Copy(env, override)
		if _, err := NewCredentials("git.example.test/team/template", func(k string) string { return env[k] }); err == nil {
			t.Fatal("unsafe auth accepted", override)
		}
	}
	env := credentialEnvironment()
	c, err = NewCredentials("GIT.EXAMPLE.TEST/team/template", func(k string) string { return env[k] })
	if err != nil || c.Repository != "git.example.test/team/template" {
		t.Fatal("host not normalized", err)
	}
	forms := []string{c.Token, base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Token)), url.QueryEscape(c.Token)}
	if c.Redact(strings.Join(forms, " ")) != "[REDACTED] [REDACTED] [REDACTED]" {
		t.Fatal("secret redaction failed")
	}
	for _, key := range []string{"GIT_TRACE_CURL", "GIT_CURL_VERBOSE", "GIT_CONFIG_PARAMETERS", "GITLAB_TOKEN", "GITHUB_TOKEN", "TOOLING_TEMPLATE_TOKEN", "PMTOOL_TEMPLATE_TOKEN"} {
		t.Setenv(key, "DO_NOT_INHERIT")
	}
	for _, entry := range cleanEnvironment() {
		if strings.Contains(entry, "DO_NOT_INHERIT") {
			t.Fatal("Git inherited caller traces or credentials")
		}
	}
}
func TestArchiveRejectsTraversalLinksDuplicatesAndSize(t *testing.T) {
	for _, header := range []*tar.Header{
		{Name: "../escape", Typeflag: tar.TypeReg}, {Name: "/absolute", Typeflag: tar.TypeReg},
		{Name: ".git/config", Typeflag: tar.TypeReg}, {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "outside"},
		{Name: "link", Typeflag: tar.TypeLink, Linkname: "outside"}, {Name: "device", Typeflag: tar.TypeChar},
		{Name: "large", Typeflag: tar.TypeReg, Size: 33 << 20},
	} {
		var data bytes.Buffer
		w := tar.NewWriter(&data)
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		err := w.Close()
		if err != nil && header.Size == 0 {
			t.Fatal(err)
		}
		if err := Extract(data.Bytes(), t.TempDir()); err == nil {
			t.Fatal("unsafe archive accepted", header.Name)
		}
	}
	var data bytes.Buffer
	w := tar.NewWriter(&data)
	for range 2 {
		if err := w.WriteHeader(&tar.Header{Name: "duplicate", Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	requireError(t, Extract(data.Bytes(), t.TempDir()), "duplicate")
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "ci")); err != nil {
		t.Fatal(err)
	}
	data.Reset()
	w = tar.NewWriter(&data)
	if err := w.WriteHeader(&tar.Header{Name: "ci/module.yml", Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	requireError(t, Extract(data.Bytes(), root), "symlink")
	if files, err := os.ReadDir(outside); err != nil || len(files) > 0 {
		t.Fatal("archive escaped root", err)
	}
}

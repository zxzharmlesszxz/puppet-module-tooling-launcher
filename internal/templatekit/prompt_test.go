package templatekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTemplateGitDisablesPromptsWithoutPlatformSpecificExecutables(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "askpass-called")
	helper := filepath.Join(root, "caller-askpass")
	body := "#!/bin/sh\nprintf called > " + shellQuote(marker) + "\n"
	if err := os.WriteFile(helper, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_ASKPASS", helper)
	t.Setenv("SSH_ASKPASS", helper)
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	environment := map[string]string{}
	for _, entry := range cleanEnvironment() {
		key, value, _ := strings.Cut(entry, "=")
		environment[key] = value
	}
	for key, expected := range map[string]string{"GIT_ASKPASS": "", "SSH_ASKPASS": "", "GIT_TERMINAL_PROMPT": "0"} {
		value, exists := environment[key]
		if !exists || value != expected {
			t.Fatal("noninteractive Git depends on an external askpass executable", key)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := RunGit(ctx, root, Credentials{}, "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	if _, err := RunGit(ctx, root, Credentials{}, "config", "core.askPass", helper); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("anonymous template request acquired caller credentials")
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="template-fixture"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	_, err := RunGit(ctx, root, Credentials{}, "ls-remote", server.URL+"/template.git")
	if err == nil || !strings.Contains(err.Error(), "terminal prompts disabled") {
		t.Fatal("missing template credentials did not fail noninteractively", err)
	}
	if ctx.Err() != nil || requests.Load() == 0 {
		t.Fatal("authentication refusal hung or bypassed the HTTP challenge", ctx.Err())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("Git executed caller or repository askpass", err)
	}
}

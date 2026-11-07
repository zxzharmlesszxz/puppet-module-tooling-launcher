package templatekit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type credentialLookup func(context.Context, string, Credentials) (Credentials, error)

func resolveTemplateCredentials(ctx context.Context, root string, c Credentials, lookup credentialLookup) (Credentials, error) {
	if c.Token != "" || c.Repository == "" {
		return c, nil
	}
	if lookup == nil {
		lookup = lookupGitCredentials
	}
	return lookup(ctx, root, c)
}

type credentialOutput struct{ buffer bytes.Buffer }

func (b *credentialOutput) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > 64<<10 {
		return 0, fmt.Errorf("credential helper response exceeds 64 KiB")
	}
	return b.buffer.Write(data)
}
func (b *credentialOutput) String() string { return b.buffer.String() }
func lookupGitCredentials(ctx context.Context, root string, c Credentials) (Credentials, error) {
	reference, err := ParseReference(c.Repository + "@v0.0.0")
	if err != nil || reference.Repository != c.Repository {
		return c, fmt.Errorf("invalid template credential repository")
	}
	host, path, _ := strings.Cut(c.Repository, "/")
	if !strings.EqualFold(host, c.Host) {
		return c, fmt.Errorf("template credential host mismatch")
	}
	lookupContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := gitProcess(lookupContext, []string{"-c", "credential.interactive=false", "-c", "core.askPass=", "-c", "core.hooksPath=/dev/null", "-C", root, "credential", "fill"}, credentialLookupEnvironment())
	cmd.Stdin = strings.NewReader("url=https://" + c.Repository + ".git\n\n")
	var output credentialOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		if killErr := stopCanceledGit(lookupContext, cmd); killErr != nil {
			return c, killErr
		}
		if lookupContext.Err() != nil {
			return c, fmt.Errorf("template credential helper canceled or timed out")
		}
		if _, ok := errors.AsType[*exec.ExitError](err); ok {
			return c, nil
		}
		return c, fmt.Errorf("cannot read Git credential helper")
	}
	values, err := parseCredentialOutput(output.String())
	if err != nil {
		return c, err
	}
	if values["username"] == "" || values["password"] == "" {
		return c, nil
	}
	if values["protocol"] != "https" || !strings.EqualFold(values["host"], host) || values["path"] != "" && values["path"] != path+".git" {
		return c, fmt.Errorf("git credential helper returned credentials for another repository")
	}
	c.Username, c.Token = values["username"], values["password"]
	if strings.ContainsAny(c.Username+c.Token, "\x00\r\n") {
		return Credentials{}, fmt.Errorf("invalid Git credential helper response")
	}
	return c, nil
}
func parseCredentialOutput(output string) (map[string]string, error) {
	values := map[string]string{}
	for line := range strings.SplitSeq(output, "\n") {
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.ContainsAny(line, "\r\x00") {
			return nil, fmt.Errorf("invalid Git credential helper response")
		}
		switch key {
		case "protocol", "host", "path", "username", "password":
			if _, duplicate := values[key]; duplicate {
				return nil, fmt.Errorf("duplicate Git credential helper field")
			}
			values[key] = value
		}
	}
	return values, nil
}
func credentialLookupEnvironment() []string {
	var result []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") && key != "GIT_CONFIG_GLOBAL" || strings.HasPrefix(key, "TOOLING_TEMPLATE_") || strings.HasPrefix(key, "PMTOOL_") || strings.HasPrefix(key, "FORGE_") {
			continue
		}
		if contains([]string{"GITLAB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN", "CI_JOB_TOKEN", "SSH_ASKPASS", "GCM_INTERACTIVE", "GCM_GUI_PROMPT"}, key) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "GIT_CONFIG_COUNT=0", "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GCM_INTERACTIVE=Never", "GCM_GUI_PROMPT=false")
}

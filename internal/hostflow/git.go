package hostflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Git struct {
	Root            string
	Context         context.Context
	Getenv          func(string) string
	Token, Username string
	OnCommand       func([]string)
}

func (g *Git) Run(args ...string) (result string, caught error) {
	ctx := g.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if g.OnCommand != nil {
		g.OnCommand(append([]string(nil), args...))
	}
	argv := append([]string{"-c", "safe.directory=" + g.Root, "-c", "credential.helper=", "-c", "credential.useHttpPath=true", "-C", g.Root}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...) // #nosec G204 -- Fixed Git executable and separate launcher-owned arguments.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 3 * time.Second
	cmd.Env = gitEnvironment()
	cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	if g.Getenv != nil {
		for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
			if value := g.Getenv(name); value != "" {
				cmd.Env = append(cmd.Env, name+"="+value)
			}
		}
	}
	if len(args) > 0 && (args[0] == "fetch" || args[0] == "push" || args[0] == "ls-remote") && g.Token != "" {
		directory, err := os.MkdirTemp("", "pmtool-askpass-")
		if err != nil {
			return "", err
		}
		defer func() {
			if err := os.RemoveAll(directory); caught == nil {
				caught = err
			}
		}()
		helper := filepath.Join(directory, "askpass")
		body := "#!/bin/sh\ncase \"$1\" in\n *Username*) printf '%s\\n' \"$PMTOOL_GIT_USERNAME\" ;;\n *Password*) printf '%s\\n' \"$PMTOOL_GIT_TOKEN\" ;;\nesac\n"
		if err := os.WriteFile(helper, []byte(body), 0700); err != nil { // #nosec G306 -- Git must execute this owner-only helper; its source contains no secret.
			return "", err
		}
		cmd.Env = append(cmd.Env, "GIT_ASKPASS="+helper, "PMTOOL_GIT_TOKEN="+g.Token, "PMTOOL_GIT_USERNAME="+g.Username)
	}
	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil && cmd.Process != nil {
			if killErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
				return output.String(), fmt.Errorf("stop canceled Git process group: %w", killErr)
			}
		}
		return output.String(), fmt.Errorf("git %s: %s: %w", firstArgument(args), g.redact(strings.TrimSpace(stderr.String())), err)
	}
	return output.String(), nil
}

func firstArgument(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
func (g *Git) redact(value string) string {
	if g.Token != "" {
		return strings.ReplaceAll(value, g.Token, "[REDACTED]")
	}
	return value
}
func (g *Git) Text(args ...string) (string, error) {
	out, err := g.Run(args...)
	return strings.TrimSpace(out), err
}
func (g *Git) Optional(args ...string) (string, bool) {
	out, err := g.Text(args...)
	return out, err == nil
}

func gitEnvironment() []string {
	var result []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_CONFIG_") || strings.HasPrefix(key, "PMTOOL_GIT_") {
			continue
		}
		switch key {
		case "GITLAB_TOKEN", "GITHUB_TOKEN", "TOOLING_TEMPLATE_TOKEN", "GIT_ASKPASS", "SSH_ASKPASS", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE":
			continue
		}
		result = append(result, entry)
	}
	return result
}

func OrdinaryClone(root string) error {
	info, err := os.Stat(filepath.Join(root, ".git"))
	if err != nil || !info.IsDir() {
		return fmt.Errorf("ordinary clone with a .git directory is required")
	}
	return nil
}

package templatekit

import (
	"archive/tar"
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Credentials struct{ Host, Repository, Username, Token string }

func NewCredentials(repository string, getenv func(string) string) (Credentials, error) {
	host, _, _ := strings.Cut(repository, "/")
	c := Credentials{Host: strings.ToLower(host), Repository: strings.ToLower(host) + strings.TrimPrefix(repository, host), Username: getenv("TOOLING_TEMPLATE_USERNAME"), Token: getenv("TOOLING_TEMPLATE_TOKEN")}
	if c.Username == "" {
		c.Username = "oauth2"
	}
	if c.Token == "" {
		return c, nil
	}
	if !strings.EqualFold(getenv("TOOLING_TEMPLATE_TOKEN_HOST"), host) {
		return c, fmt.Errorf("TOOLING_TEMPLATE_TOKEN_HOST must match the template host")
	}
	if strings.ContainsAny(c.Username+c.Token, "\x00\r\n") {
		return c, fmt.Errorf("invalid template credential characters")
	}
	return c, nil
}
func (c Credentials) Redact(value string) string {
	if c.Token == "" {
		return value
	}
	for _, raw := range []string{c.Token, base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Token)), url.QueryEscape(c.Token)} {
		value = strings.ReplaceAll(value, raw, "[REDACTED]")
	}
	return value
}

type GitRunner func(context.Context, string, Credentials, ...string) ([]byte, error)

func Checkout(ctx context.Context, reference string, getenv func(string) string, runner GitRunner, consume func(*Template) error) (caught error) {
	r, err := ParseReference(reference)
	if err != nil {
		return err
	}
	c, err := NewCredentials(r.Repository, getenv)
	if err != nil {
		return err
	}
	if runner == nil {
		runner = RunGit
	}
	tmp, err := os.MkdirTemp("", "pmtool-template-*")
	if err != nil {
		return err
	}
	defer func() {
		err := os.RemoveAll(tmp)
		if caught == nil {
			caught = err
		}
	}()
	git := filepath.Join(tmp, "git")
	if err := os.Mkdir(git, 0700); err != nil {
		return err
	}
	if _, err := runner(ctx, git, Credentials{}, "init", "--quiet"); err != nil {
		return err
	}
	ref := r.Version
	if len(ref) != 40 || !isHex(ref) {
		ref = "refs/tags/" + ref
	}
	if _, err := runner(ctx, git, c, "fetch", "--quiet", "--depth=1", "https://"+r.Repository+".git", ref); err != nil {
		return err
	}
	sha, err := runner(ctx, git, Credentials{}, "rev-parse", "FETCH_HEAD^{commit}")
	if err != nil {
		return err
	}
	revision := strings.TrimSpace(string(sha))
	if len(revision) != 40 || !isHex(revision) {
		return fmt.Errorf("template fetch returned invalid commit SHA")
	}
	archive, err := runner(ctx, git, Credentials{}, "archive", "--format=tar", revision)
	if err != nil {
		return err
	}
	root := filepath.Join(tmp, "package")
	if err := os.Mkdir(root, 0700); err != nil {
		return err
	}
	if err := Extract(archive, root); err != nil {
		return err
	}
	if r.Subdir != "" {
		root, err = SafePath(root, r.Subdir)
		if err != nil {
			return err
		}
	}
	t, err := Load(root, reference, revision)
	if err != nil {
		return err
	}
	return consume(t)
}
func isHex(value string) bool {
	return strings.Trim(value, "0123456789abcdefABCDEF") == ""
}
func Extract(data []byte, root string) error {
	reader := tar.NewReader(bytes.NewReader(data))
	var total int64
	seen := map[string]bool{}
	for {
		entry, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(entry.Name, "/")
		if entry.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		path, err := SafePath(root, name)
		if err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("duplicate template archive path: %s", name)
		}
		seen[name] = true
		switch entry.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			if entry.Size < 0 || entry.Size > 32<<20 || total > (128<<20)-entry.Size {
				return fmt.Errorf("template archive exceeds size limit")
			}
			total += entry.Size
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- SafePath rejects traversal and links; each archive file is created exclusively.
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, reader, entry.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("template contains unsupported link/device: %s", name)
		}
	}
}
func RunGit(ctx context.Context, root string, c Credentials, args ...string) (output []byte, caught error) {
	base := []string{"-c", "credential.helper=", "-c", "core.askPass=", "-c", "core.hooksPath=/dev/null", "-c", "protocol.file.allow=never", "-c", "http.followRedirects=false", "-c", "credential.useHttpPath=true", "-C", root}
	environment := cleanEnvironment()
	var helper string
	if c.Token != "" {
		tmp, err := os.MkdirTemp("", "pmtool-template-auth-*")
		if err != nil {
			return nil, err
		}
		defer func() {
			err := os.RemoveAll(tmp)
			if caught == nil {
				caught = err
			}
		}()
		helper = filepath.Join(tmp, "credential")

		if err := os.WriteFile(helper, []byte(credentialScript), 0700); err != nil { // #nosec G306 -- Owner-only executable helper contains no credential value.
			return nil, err
		}
		base = append([]string{"-c", "credential.helper=", "-c", "credential.helper=!" + shellQuote(helper)}, base[2:]...)
		environment = append(environment, "PMTOOL_TEMPLATE_TOKEN="+c.Token, "PMTOOL_TEMPLATE_USERNAME="+c.Username, "PMTOOL_TEMPLATE_REPOSITORY="+c.Repository)
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...) // #nosec G204 -- Fixed Git executable, validated ref/repository and separate argv.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 3 * time.Second
	cmd.Env = environment
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	result, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil && cmd.Process != nil {
			if killErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
				return nil, fmt.Errorf("stop canceled template Git process group: %w", killErr)
			}
		}
		return nil, fmt.Errorf("cannot fetch template: %s: %w", c.Redact(strings.TrimSpace(stderr.String())), err)
	}
	return result, nil
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func cleanEnvironment() []string {
	var result []string
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(key, "GIT_") || strings.HasPrefix(key, "TOOLING_TEMPLATE_") || strings.HasPrefix(key, "PMTOOL_TEMPLATE_") {
			continue
		}
		if contains([]string{"GITLAB_TOKEN", "GITHUB_TOKEN", "CI_JOB_TOKEN", "SSH_ASKPASS"}, key) {
			continue
		}
		result = append(result, v)
	}
	return append(result, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GIT_TERMINAL_PROMPT=0")
}

//go:embed assets/credential_helper.sh
var credentialScript string

package hostflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Command, Module, Branch, Ref, Title, Version, Files, File, CIProject string
	Watch                                                                time.Duration
	Settings                                                             Settings
	Noop                                                                 bool
}
type Engine struct {
	Project               Project
	Git                   *Git
	API                   Hosting
	Context               context.Context
	Out                   io.Writer
	Sleep                 func(context.Context, time.Duration) error
	Getenv                func(string) string
	Color                 string
	outputErr             error
	Recovery, RecoveryRef string
}

var commands = strings.Fields("status diff settings bootstrap-prepare bootstrap-commit bootstrap-commit-noop bootstrap-push bootstrap-push-noop main-sync release-prepare release-commit release-commit-noop release-submit release-push release-push-noop release-tag release-tag-noop cleanup cleanup-noop mr mr-create mr-merge pipeline-jobs policy policy-noop policy-delete-master")

func IsCommand(name string) bool {
	return slices.Contains(commands, name)
}
func Parse(args []string, cwd string) (Options, error) {
	if len(args) == 0 {
		return Options{}, fmt.Errorf("missing workflow command")
	}
	o := Options{Command: args[0], Module: cwd, Noop: strings.HasSuffix(args[0], "-noop")}
	for i := 1; i < len(args); i++ {
		key, value, equals := strings.Cut(args[i], "=")
		if !equals {
			i++
			if i >= len(args) {
				return o, fmt.Errorf("missing value for %s", key)
			}
			value = args[i]
		}
		if !optionAllowed(o.Command, key) {
			return o, fmt.Errorf("unsupported option for %s: %s", o.Command, key)
		}
		switch key {
		case "--module":
			o.Module = value
		case "--branch":
			o.Branch = value
		case "--ref":
			o.Ref = value
		case "--title":
			o.Title = value
		case "--version":
			o.Version = value
		case "--file":
			o.File = value
		case "--files":
			o.Files = value
		case "--ci-project":
			o.CIProject = value
		case "--ci":
			if value == "" {
				return o, fmt.Errorf("ci must not be empty")
			}
			o.Settings.CI = value
		case "--template":
			if value == "" {
				return o, fmt.Errorf("template must not be empty")
			}
			o.Settings.Template = value
		case "--provider":
			if value == "" {
				return o, fmt.Errorf("provider must not be empty")
			}
			o.Settings.Provider = value
		case "--watch":
			seconds, err := strconv.Atoi(value)
			if err != nil || seconds < 0 || seconds > 86400 {
				return o, fmt.Errorf("watch must be 0..86400 seconds")
			}
			o.Watch = time.Duration(seconds) * time.Second
		default:
			return o, fmt.Errorf("unsupported option for %s: %s", o.Command, key)
		}
	}
	root, err := filepath.Abs(o.Module)
	if err != nil {
		return o, err
	}
	o.Module, err = filepath.EvalSymlinks(root)
	return o, err
}
func New(ctx context.Context, o Options, getenv func(string) string, out io.Writer) (*Engine, error) {
	if err := OrdinaryClone(o.Module); err != nil {
		return nil, err
	}
	g := &Git{Root: o.Module, Context: ctx, Getenv: getenv}
	origin, err := g.Text("remote", "get-url", "origin")
	if err != nil {
		return nil, err
	}
	settings, err := ReadSettings(o.Module, o.Settings)
	if err != nil {
		return nil, err
	}
	project, err := ResolveProject(o.Module, origin, settings, getenv)
	if err != nil {
		return nil, err
	}
	key, username := "GITLAB_TOKEN", getenv("GITLAB_USERNAME")
	if username == "" {
		username = "oauth2"
	}
	if project.Provider == "github" {
		key, username = "GITHUB_TOKEN", "x-access-token"
	}
	g.Token, g.Username = getenv(key), username
	return &Engine{Project: project, Git: g, API: NewClient(project, g.Token), Context: ctx, Out: out, Getenv: getenv, Sleep: func(ctx context.Context, d time.Duration) error {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}}, nil
}
func (e *Engine) row(format string, args ...any) {
	e.print("%s\t%s\n", e.Project.Name, fmt.Sprintf(format, args...))
}
func (e *Engine) origin() error {
	origin, err := e.Git.Text("remote", "get-url", "origin")
	if err != nil {
		return err
	}
	if Identity(origin) != Identity(e.Project.Origin) {
		return fmt.Errorf("origin changed")
	}
	return nil
}
func (e *Engine) branch() (string, error) {
	return e.Git.Text("symbolic-ref", "--quiet", "--short", "HEAD")
}
func (e *Engine) head() (string, error) { return e.Git.Text("rev-parse", "HEAD") }
func (e *Engine) token() error {
	if e.Git.Token == "" {
		key := "GITLAB_TOKEN"
		if e.Project.Provider == "github" {
			key = "GITHUB_TOKEN"
		}
		return fmt.Errorf("%s is required", key)
	}
	return nil
}
func (e *Engine) guard(clean, main, token bool) error {
	if err := OrdinaryClone(e.Project.Root); err != nil {
		return err
	}
	if err := e.origin(); err != nil {
		return err
	}
	if token {
		if err := e.token(); err != nil {
			return err
		}
	}
	for _, marker := range strings.Fields("MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD rebase-merge rebase-apply BISECT_START") {
		path, err := e.Git.Text("rev-parse", "--git-path", marker)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(e.Project.Root, path)
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("unfinished Git operation: %s", marker)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if main {
		if _, ok := e.Git.Optional("rev-parse", "--verify", "refs/heads/main"); !ok {
			return fmt.Errorf("local main is missing")
		}
	}
	if clean {
		status, err := e.Git.Text("status", "--porcelain", "--untracked-files=all")
		if err != nil {
			return err
		}
		if status != "" {
			return fmt.Errorf("working tree or index is not clean")
		}
	}
	return nil
}
func (e *Engine) remoteRefs(patterns ...string) (map[string]string, error) {
	args := append([]string{"ls-remote", e.Project.Remote}, patterns...)
	out, err := e.Git.Run(args...)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			result[fields[1]] = fields[0]
		}
	}
	return result, nil
}
func (e *Engine) ancestor(a, b string) error {
	_, err := e.Git.Run("merge-base", "--is-ancestor", a, b)
	if err != nil {
		return fmt.Errorf("%s has commits outside %s", a, b)
	}
	return nil
}
func (e *Engine) fetchMain() (string, error) {
	if _, err := e.Git.Run("fetch", "--no-tags", e.Project.Remote, "refs/heads/main:refs/remotes/origin/main"); err != nil {
		return "", err
	}
	return e.Git.Text("rev-parse", "refs/remotes/origin/main")
}
func (e *Engine) syncMain() error {
	if err := e.guard(true, false, true); err != nil {
		return err
	}
	sha, err := e.fetchMain()
	if err != nil {
		return err
	}
	if _, exists := e.Git.Optional("rev-parse", "--verify", "refs/heads/main"); exists {
		if err := e.ancestor("refs/heads/main", sha); err != nil {
			return err
		}
	}
	if err := e.ancestor("HEAD", sha); err != nil {
		return err
	}
	if _, exists := e.Git.Optional("rev-parse", "--verify", "refs/heads/main"); exists {
		if _, err := e.Git.Run("switch", "main"); err != nil {
			return err
		}
		if _, err := e.Git.Run("merge", "--ff-only", sha); err != nil {
			return err
		}
	} else {
		if _, err := e.Git.Run("switch", "--no-track", "-c", "main", sha); err != nil {
			return err
		}
	}
	if _, err := e.Git.Run("branch", "--set-upstream-to=origin/main", "main"); err != nil {
		return err
	}
	e.row("main synchronized %s", sha)
	return nil
}

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func validVersion(version string) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("version must be canonical X.Y.Z")
	}
	return nil
}
func greater(a, b string) bool {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range aa {
		if len(aa[i]) != len(bb[i]) {
			return len(aa[i]) > len(bb[i])
		}
		if aa[i] != bb[i] {
			return aa[i] > bb[i]
		}
	}
	return false
}

type Metadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (e *Engine) metadata(ref string) (Metadata, error) {
	var body []byte
	var err error
	if ref == "" {
		body, err = os.ReadFile(filepath.Join(e.Project.Root, "metadata.json"))
	} else {
		var raw string
		raw, err = e.Git.Run("show", ref+":metadata.json")
		body = []byte(raw)
	}
	if err != nil {
		return Metadata{}, err
	}
	var m Metadata
	if err := json.Unmarshal(body, &m); err != nil {
		return m, err
	}
	if err := validVersion(m.Version); err != nil {
		return m, err
	}
	if len(strings.SplitN(m.Name, "-", 2)) != 2 || strings.SplitN(m.Name, "-", 2)[1] != strings.TrimPrefix(e.Project.Name, "puppet-") {
		return m, fmt.Errorf("metadata module name does not match origin")
	}
	return m, nil
}
func (e *Engine) changelog(version string) error {
	body, err := os.ReadFile(filepath.Join(e.Project.Root, "CHANGELOG.md"))
	if err != nil {
		return err
	}
	if !regexp.MustCompile(`(?m)^## Release ` + regexp.QuoteMeta(version) + `(?:\s|$)`).Match(body) {
		return fmt.Errorf("CHANGELOG.md: missing Release %s", version)
	}
	return nil
}
func (e *Engine) validateBranch(branch string) error {
	_, err := e.Git.Run("check-ref-format", "--branch", branch)
	if err != nil {
		return fmt.Errorf("invalid branch name")
	}
	return nil
}
func (e *Engine) prepare(o Options, bootstrap bool) error {
	if err := e.guard(true, !bootstrap, true); err != nil {
		return err
	}
	branch := o.Branch
	if bootstrap {
		if !strings.HasPrefix(branch, "bootstrap/") {
			return fmt.Errorf("branch must be bootstrap/<name>")
		}
	} else {
		if err := validVersion(o.Version); err != nil {
			return err
		}
		branch = "release/" + o.Version
	}
	if err := e.validateBranch(branch); err != nil {
		return err
	}
	if _, ok := e.Git.Optional("rev-parse", "--verify", "refs/heads/"+branch); ok {
		return fmt.Errorf("local branch already exists")
	}
	patterns := []string{"refs/heads/" + branch}
	if !bootstrap {
		tag := "refs/tags/v" + o.Version
		if _, ok := e.Git.Optional("rev-parse", "--verify", tag); ok {
			return fmt.Errorf("local release tag already exists")
		}
		patterns = append(patterns, tag)
	}
	refs, err := e.remoteRefs(patterns...)
	if err != nil {
		return err
	}
	if len(refs) != 0 {
		return fmt.Errorf("remote branch or tag already exists")
	}
	if !bootstrap {
		sha, err := e.fetchMain()
		if err != nil {
			return err
		}
		m, err := e.metadata(sha)
		if err != nil {
			return err
		}
		if !greater(o.Version, m.Version) {
			return fmt.Errorf("release version must exceed main metadata")
		}
	}
	if err := e.syncMain(); err != nil {
		return err
	}
	if !bootstrap {
		current, err := e.metadata("main")
		if err != nil {
			return err
		}
		if !greater(o.Version, current.Version) {
			return fmt.Errorf("release version must exceed synchronized main metadata")
		}
	}
	if _, err := e.Git.Run("switch", "--no-track", "-c", branch, "main"); err != nil {
		return err
	}
	e.row("%s prepared", branch)
	return nil
}

func (e *Engine) print(format string, args ...any) {
	if e.outputErr == nil {
		_, e.outputErr = fmt.Fprintf(e.Out, format, args...)
	}
}

func optionAllowed(command, key string) bool {
	switch key {
	case "--module", "--ci", "--provider", "--template", "--ci-project":
		return true
	case "--file":
		return command == "diff"
	case "--branch":
		return command == "bootstrap-prepare" || command == "cleanup" || command == "cleanup-noop" || strings.HasPrefix(command, "mr") || command == "pipeline-jobs"
	case "--ref", "--watch":
		return command == "pipeline-jobs"
	case "--title":
		return command == "mr" || command == "mr-create"
	case "--version":
		return command == "release-prepare"
	case "--files":
		return strings.Contains(command, "-commit") || command == "release-submit"
	}
	return false
}

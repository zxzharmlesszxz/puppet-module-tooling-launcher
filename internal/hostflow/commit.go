package hostflow

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

func splitPaths(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool { return r == 0 })
}
func (e *Engine) selectedFiles(explicit string) ([]string, []string, error) {
	unresolved, err := e.Git.Text("ls-files", "--unmerged")
	if err != nil {
		return nil, nil, err
	}
	if unresolved != "" {
		return nil, nil, fmt.Errorf("index contains unresolved conflicts")
	}
	changed := map[string]bool{}
	pending := map[string]bool{}
	for i, args := range [][]string{{"diff", "HEAD", "--name-only", "--no-renames", "-z"}, {"diff", "--cached", "--name-only", "--no-renames", "-z"}, {"ls-files", "--others", "--exclude-standard", "-z"}, {"diff", "--name-only", "--no-renames", "-z"}} {
		raw, err := e.Git.Run(args...)
		if err != nil {
			return nil, nil, err
		}
		for _, name := range splitPaths(raw) {
			changed[name] = true
			if i >= 2 {
				pending[name] = true
			}
		}
	}
	var files []string
	if explicit == "" {
		for name := range changed {
			files = append(files, name)
		}
		sort.Strings(files)
	} else {
		seen := map[string]bool{}
		for _, name := range strings.Fields(explicit) {
			if !seen[name] {
				files = append(files, name)
				seen[name] = true
			}
		}
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("no changes to commit")
	}
	selected := map[string]bool{}
	var stage []string
	for _, name := range files {
		if path.IsAbs(name) || strings.Contains(name, "\\") || strings.ContainsAny(name, "\r\n\x00") {
			return nil, nil, fmt.Errorf("unsafe file path %q", name)
		}
		for _, part := range strings.Split(name, "/") {
			if part == "" || part == "." || part == ".." || part == ".git" {
				return nil, nil, fmt.Errorf("unsafe file path %q", name)
			}
		}
		if explicit != "" && (strings.HasPrefix(name, "-") || !regexp.MustCompile(`^[\w./-]+$`).MatchString(name)) {
			return nil, nil, fmt.Errorf("unsafe explicit file path %q", name)
		}
		if !changed[name] {
			return nil, nil, fmt.Errorf("file has no changes: %q", name)
		}
		if info, err := os.Stat(e.Project.Root + "/" + name); err == nil && info.IsDir() {
			return nil, nil, fmt.Errorf("directories cannot be committed as selections")
		}
		selected[name] = true
		if pending[name] {
			stage = append(stage, name)
		}
	}
	for name := range changed {
		if !selected[name] {
			return nil, nil, fmt.Errorf("changes outside selected files: %q", name)
		}
	}
	return files, stage, nil
}
func literalPaths(files []string) []string {
	paths := make([]string, len(files))
	for i, name := range files {
		paths[i] = ":(literal)" + name
	}
	return paths
}
func (e *Engine) author() ([]string, error) {
	var result []string
	for _, field := range []struct{ env, key string }{{"GIT_AUTHOR_NAME", "user.name"}, {"GIT_AUTHOR_EMAIL", "user.email"}} {
		value := e.Getenv(field.env)
		if value == "" {
			value, _ = e.Git.Optional("config", "--get", field.key)
		}
		if value == "" {
			return nil, fmt.Errorf("git author name and email are required")
		}
		result = append(result, "-c", field.key+"="+value)
	}
	return result, nil
}
func (e *Engine) commit(o Options, bootstrap bool) error {
	if err := e.guard(false, !bootstrap, false); err != nil {
		return err
	}
	branch, err := e.branch()
	if err != nil {
		return err
	}
	m, err := e.metadata("")
	if err != nil {
		return err
	}
	if err := e.changelog(m.Version); err != nil {
		return err
	}
	if bootstrap {
		if !strings.HasPrefix(branch, "bootstrap/") {
			return fmt.Errorf("switch to bootstrap/<name> before bootstrap commit")
		}
	} else {
		if branch != "release/"+m.Version {
			return fmt.Errorf("release branch must match metadata version")
		}
		previous, err := e.metadata("HEAD")
		if err != nil {
			return err
		}
		if !greater(m.Version, previous.Version) {
			return fmt.Errorf("release version must exceed HEAD metadata")
		}
	}
	files, stage, err := e.selectedFiles(o.Files)
	if err != nil {
		return err
	}
	if !bootstrap {
		changed, err := e.Git.Run("diff", "HEAD", "--name-only", "--no-renames", "-z")
		if err != nil {
			return err
		}
		changedPaths := map[string]bool{}
		for _, name := range splitPaths(changed) {
			changedPaths[name] = true
		}
		for _, required := range []string{"metadata.json", "CHANGELOG.md"} {
			if !changedPaths[required] {
				return fmt.Errorf("metadata.json and CHANGELOG.md must both change")
			}
			found := false
			for _, name := range files {
				found = found || name == required
			}
			if !found {
				return fmt.Errorf("release commit must include changed %s", required)
			}
		}
	}
	e.row("files:")
	for _, name := range files {
		e.print("  %q\n", name)
	}
	message := "[REL] Release " + m.Version
	if bootstrap {
		message = "[UPD] Bootstrap module " + m.Version
	}
	if o.Noop {
		diff, err := e.Git.Run(append([]string{"diff", "--color=" + e.color(), "--no-ext-diff", "--no-textconv", "HEAD", "--"}, literalPaths(files)...)...)
		if err != nil {
			return err
		}
		e.print("%s", diff)
		untracked, err := e.Git.Run("ls-files", "--others", "--exclude-standard", "-z")
		if err != nil {
			return err
		}
		selected := map[string]bool{}
		for _, file := range files {
			selected[file] = true
		}
		for _, file := range splitPaths(untracked) {
			if selected[file] {
				diff, _ := e.Git.Run("diff", "--color="+e.color(), "--no-ext-diff", "--no-textconv", "--no-index", "--", "/dev/null", file)
				e.print("%s", diff)
			}
		}
		e.row("%s would commit %s", branch, message)
		return nil
	}
	author, err := e.author()
	if err != nil {
		return err
	}
	if len(stage) > 0 {
		if _, err := e.Git.Run(append([]string{"add", "--all", "--"}, literalPaths(stage)...)...); err != nil {
			return err
		}
	}
	args := append(author, "commit", "--message", message)
	output, err := e.Git.Run(args...)
	if err != nil {
		return err
	}
	e.print("%s", output)
	return nil
}
func (e *Engine) push(o Options, bootstrap bool) error {
	if err := e.guard(true, !bootstrap, true); err != nil {
		return err
	}
	m, err := e.metadata("")
	if err != nil {
		return err
	}
	if err := e.changelog(m.Version); err != nil {
		return err
	}
	branch, err := e.branch()
	if err != nil {
		return err
	}
	if bootstrap {
		if !strings.HasPrefix(branch, "bootstrap/") {
			return fmt.Errorf("switch to bootstrap/<name> before push")
		}
	} else if branch != "release/"+m.Version {
		return fmt.Errorf("release branch must match metadata version")
	}
	sha, err := e.head()
	if err != nil {
		return err
	}
	args := []string{"push", "--porcelain", "--no-force", "--no-mirror", "--no-follow-tags", "--recurse-submodules=no"}
	if o.Noop {
		args = append(args, "--dry-run")
	}
	args = append(args, e.Project.Remote, sha+":refs/heads/"+branch)
	output, err := e.Git.Run(args...)
	if err != nil {
		return err
	}
	e.print("%s", output)
	if o.Noop {
		e.row("%s push preview passed %s", branch, sha)
		return nil
	}
	for _, args := range [][]string{{"update-ref", "refs/remotes/origin/" + branch, sha}, {"config", "branch." + branch + ".remote", "origin"}, {"config", "branch." + branch + ".merge", "refs/heads/" + branch}} {
		if _, err := e.Git.Run(args...); err != nil {
			return err
		}
	}
	e.row("%s pushed %s", branch, sha)
	return nil
}

package hostflow

import (
	"fmt"
	"strings"
)

// ValidateInputs reads local state only, before a command can mutate Git or call an API.
func (e *Engine) ValidateInputs(o Options, token, author bool) error {
	var missing []string
	switch o.Command {
	case "release-prepare":
		if o.Version == "" {
			missing = append(missing, "--version X.Y.Z")
		}
	case "bootstrap-prepare":
		if o.Branch == "" {
			missing = append(missing, "--branch bootstrap/NAME")
		}
	case "cleanup", "cleanup-noop":
		if o.Branch == "" {
			missing = append(missing, "--branch")
		}
	}
	if token {
		key := "GITLAB_TOKEN"
		if e.Project.Provider == "github" {
			key = "GITHUB_TOKEN"
		}
		if strings.TrimSpace(e.Git.Token) == "" {
			missing = append(missing, key)
		} else if strings.ContainsAny(e.Git.Token, "\r\n\x00") {
			return fmt.Errorf("%s: invalid %s characters", o.Command, key)
		}
	}
	if author {
		if _, err := e.author(); err != nil {
			missing = append(missing, "Git user.name/user.email or GIT_AUTHOR_NAME/GIT_AUTHOR_EMAIL")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s: missing required inputs: %s", o.Command, strings.Join(missing, ", "))
	}
	return nil
}

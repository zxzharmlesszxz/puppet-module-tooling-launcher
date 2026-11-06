package pmtool

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode"
)

func validateTemplateOrigin(origin string) error {
	invalid := fmt.Errorf("origin must be a credential-free HTTP(S) or SSH repository URL with a namespace and repository, without query, fragment or whitespace")
	if origin == "" || strings.ContainsAny(origin, "\\?#") || strings.IndexFunc(origin, unicode.IsSpace) >= 0 || strings.IndexFunc(origin, unicode.IsControl) >= 0 {
		return invalid
	}
	address := origin
	if !strings.Contains(address, "://") {
		if !regexp.MustCompile(`^(?:[A-Za-z0-9_][A-Za-z0-9_.-]*@)?[A-Za-z0-9][A-Za-z0-9.-]*:.+$`).MatchString(address) {
			return invalid
		}
		host, path, _ := strings.Cut(address, ":")
		address = "ssh://" + host + "/" + path
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || u.Opaque != "" || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return invalid
	}
	switch u.Scheme {
	case "http", "https":
		if u.User != nil {
			return invalid
		}
	case "ssh":
		if u.User != nil {
			if _, password := u.User.Password(); password || !regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`).MatchString(u.User.Username()) {
				return invalid
			}
		}
	default:
		return invalid
	}
	path := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
	if !strings.Contains(path, "/") {
		return invalid
	}
	for part := range strings.SplitSeq(path, "/") {
		if part == "." || part == ".." || part == ".git" || !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(part) {
			return invalid
		}
	}
	return nil
}

func templateGitPlan(directory string, args ...string) plan {
	environment := privateValidationEnvironment()
	environment["GIT_CONFIG_GLOBAL"] = os.DevNull
	environment["GIT_CONFIG_NOSYSTEM"] = "1"
	environment["GIT_TERMINAL_PROMPT"] = "0"
	return plan{Docker: "git", Args: args, Directory: directory, Environment: environment}
}

func (a *app) initializeTemplateRepository(directory, origin string) error {
	for _, args := range [][]string{
		{"init", "--initial-branch=main", "--object-format=sha1", "--template=", "."},
		{"remote", "add", "--", "origin", origin},
	} {
		if _, err := a.driver.capturePlan(templateGitPlan(directory, args...)); err != nil {
			return fmt.Errorf("template files are preserved at %s; local Git setup failed, inspect that directory before recovery: %w", directory, err)
		}
	}
	return nil
}

package templatekit

import (
	"fmt"
	"regexp"
	"strings"
)

type Reference struct{ Repository, Version, Subdir string }

func ParseReference(value string) (Reference, error) {
	r := Reference{}
	if strings.Count(value, "@") != 1 {
		return r, fmt.Errorf("template.use must be repository@version")
	}
	location, version, _ := strings.Cut(value, "@")
	if !versionPattern.MatchString(version) {
		return r, fmt.Errorf("template version must be a version tag or full commit SHA")
	}
	repository, subdir, _ := strings.Cut(location, "//")
	if strings.Contains(location, "//") && subdir == "" {
		return r, fmt.Errorf("template subdirectory must not be empty")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]+/[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+$`).MatchString(repository) || !strings.Contains(strings.Split(repository, "/")[0], ".") {
		return r, fmt.Errorf("template repository must be host/namespace/repository without credentials or a URL scheme")
	}
	for _, part := range strings.Split(repository, "/") {
		if err := Path(part); err != nil {
			return r, err
		}
	}
	if subdir != "" {
		if err := Path(subdir); err != nil {
			return r, err
		}
	}
	host, path, _ := strings.Cut(repository, "/")
	return Reference{strings.ToLower(host) + "/" + strings.TrimSuffix(path, ".git"), version, subdir}, nil
}

type CIReference struct{ Host, Repository, File, Ref, Provider string }

func ParseCI(value, provider, host string) (CIReference, error) {
	c := CIReference{Provider: provider}
	if provider != "gitlab" && provider != "github" {
		return c, fmt.Errorf("CI provider must be gitlab or github")
	}
	if strings.Count(value, "@") != 1 {
		return c, fmt.Errorf("CI must be repo/path@version")
	}
	location, ref, _ := strings.Cut(value, "@")
	if !regexp.MustCompile(`^(?:v?[0-9]+(?:\.[0-9]+){0,2}(?:[-+][A-Za-z0-9.-]+)?|[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`).MatchString(ref) {
		return c, fmt.Errorf("CI version must be a version tag or full SHA")
	}
	if location == "" || strings.HasPrefix(location, "/") || strings.ContainsAny(location, ":?#\\ \t\r\n") {
		return c, fmt.Errorf("unsafe CI reference")
	}
	first, rest, _ := strings.Cut(location, "/")
	c.Host = host
	path := location
	if strings.Contains(first, ".") || first == "localhost" {
		c.Host = strings.ToLower(first)
		path = rest
	}
	if !regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?$`).MatchString(c.Host) {
		return c, fmt.Errorf("invalid CI host")
	}
	c.Ref = ref
	if provider == "github" {
		parts := strings.Split(path, "/")
		if len(parts) != 5 || parts[2] != ".github" || parts[3] != "workflows" {
			return c, fmt.Errorf("GitHub CI must be owner/repo/.github/workflows/file.yml@version")
		}
		c.Repository = strings.Join(parts[:2], "/")
		c.File = strings.Join(parts[2:], "/")
	} else if strings.Contains(path, "//") {
		c.Repository, c.File, _ = strings.Cut(path, "//")
	} else {
		i := strings.LastIndex(path, "/")
		if i < 0 {
			return c, fmt.Errorf("CI must include repository and file")
		}
		c.Repository, c.File = path[:i], path[i+1:]
	}
	for _, s := range []string{c.Repository, c.File} {
		if err := Path(s); err != nil {
			return c, err
		}
		for _, v := range strings.Split(s, "/") {
			if !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(v) {
				return c, fmt.Errorf("unsafe CI path")
			}
		}
	}
	if !strings.Contains(c.Repository, "/") || !strings.HasSuffix(c.File, ".yml") && !strings.HasSuffix(c.File, ".yaml") {
		return c, fmt.Errorf("invalid CI repository/file")
	}
	return c, nil
}
func (c CIReference) Config(moduleHost string) (string, Mapping, error) {
	target := ".gitlab-ci.yml"
	if c.Provider == "github" {
		target = GitHubCI
	}
	m := Mapping{"unmanaged": false, "project": c.Repository, "file": "/" + c.File, "ref": c.Ref, "host": c.Host}
	if moduleHost != "" && !strings.EqualFold(moduleHost, c.Host) {
		if c.Provider == "gitlab" && c.Host == "github.com" {
			m["remote"] = "https://raw.githubusercontent.com/" + c.Repository + "/" + c.Ref + "/" + c.File
		} else {
			return "", nil, fmt.Errorf("CI host must match module hosting server")
		}
	}
	return target, m, nil
}
func (t *Template) CIConfig(provider, host, explicit, legacy string) (string, Mapping, error) {
	if explicit == "" && legacy != "" {
		if provider != "gitlab" {
			return "", nil, fmt.Errorf("--ci-project is GitLab-only; use --ci repository/path@version")
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+$`).MatchString(legacy) {
			return "", nil, fmt.Errorf("invalid central CI project")
		}
		for _, p := range strings.Split(legacy, "/") {
			if p == "." || p == ".." {
				return "", nil, fmt.Errorf("invalid central CI project")
			}
		}
		return ".gitlab-ci.yml", Mapping{"unmanaged": false, "project": legacy, "file": "/ci/puppet-module.yml", "ref": "v0.6.0"}, nil
	}
	if explicit == "" {
		entry := str(t.CI[provider])
		if entry == "" {
			return "", nil, fmt.Errorf("template has no %s CI; select --ci explicitly", provider)
		}
		ref, err := ParseReference(t.Reference)
		if err != nil {
			return "", nil, err
		}
		if ref.Subdir != "" {
			entry = ref.Subdir + "/" + entry
		}
		separator := "//"
		if provider == "github" {
			separator = "/"
		}
		explicit = ref.Repository + separator + entry + "@" + t.Revision
	}
	c, err := ParseCI(explicit, provider, host)
	if err != nil {
		return "", nil, err
	}
	return c.Config(host)
}

// Package hostflow implements guarded Git and hosting operations on the host.
package hostflow

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Settings struct {
	CI       string `json:"ci"`
	Provider string `json:"provider"`
	Template string `json:"template"`
}

type Project struct {
	Root, Origin, Remote, Endpoint, ID, Name, Provider string
	Template                                           bool
}

var repositorySegment = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var repositoryName = regexp.MustCompile(`^puppet-[a-z0-9][a-z0-9_-]*$`)

func ReadSettings(root string, overrides Settings) (Settings, error) {
	result := Settings{}
	path := filepath.Join(root, ".sync.yml")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return result, fmt.Errorf(".sync.yml must be a regular file, not a symlink")
		}
	} else if !os.IsNotExist(err) {
		return result, err
	}
	body, err := os.ReadFile(filepath.Join(root, ".sync.yml")) // #nosec G304 -- The selected module owns this tracked configuration path.
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	var values map[string]any
	if err == nil {
		if err := yaml.Unmarshal(body, &values); err != nil {
			return result, fmt.Errorf(".sync.yml: expected a mapping: %w", err)
		}
	}
	for _, section := range []string{"ci", "template"} {
		raw, exists := values[section]
		if !exists {
			continue
		}
		mapping, ok := raw.(map[string]any)
		if !ok {
			return result, fmt.Errorf(".sync.yml: %s must be a mapping", section)
		}
		for key, value := range mapping {
			if key != "use" && (section != "ci" || key != "provider") {
				return result, fmt.Errorf(".sync.yml: invalid %s field %s", section, key)
			}
			if value == nil {
				continue
			}
			text, ok := value.(string)
			if !ok {
				return result, fmt.Errorf(".sync.yml: %s.%s must be a string", section, key)
			}
			switch {
			case section == "template":
				result.Template = text
			case key == "provider":
				result.Provider = text
			default:
				result.CI = text
			}
		}
	}
	if result.Provider != "" && result.Provider != "gitlab" && result.Provider != "github" {
		return result, fmt.Errorf(".sync.yml: ci.provider must be gitlab or github")
	}
	if overrides.CI != "" {
		result.CI = overrides.CI
	}
	if overrides.Template != "" {
		result.Template = overrides.Template
	}
	if overrides.Provider != "" {
		result.Provider = overrides.Provider
	}
	if result.Provider != "" && result.Provider != "gitlab" && result.Provider != "github" {
		return result, fmt.Errorf("provider must be gitlab or github")
	}
	return result, nil
}

func parseOrigin(origin string) (*url.URL, error) {
	if origin == "" {
		return nil, fmt.Errorf("missing origin URL")
	}
	value := origin
	if !strings.Contains(value, "://") {
		match := regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`).FindStringSubmatch(value)
		if match == nil {
			return nil, fmt.Errorf("origin must be an HTTP(S) or SSH repository URL")
		}
		value = "ssh://" + match[1] + "/" + match[2]
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || (u.Scheme != "ssh" && u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("origin must be an HTTP(S) or SSH repository URL")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("origin must not contain a password, query or fragment")
	}
	if u.User != nil {
		if _, found := u.User.Password(); found {
			return nil, fmt.Errorf("origin must not contain a password, query or fragment")
		}
	}
	return u, nil
}

// OriginContext selects hosting without applying module repository-name policy.
func OriginContext(origin, provider string) (string, string, error) {
	u, err := parseOrigin(origin)
	if err != nil {
		return "", "", err
	}
	host := strings.ToLower(u.Hostname())
	if provider == "" {
		provider = "gitlab"
		if host == "github.com" {
			provider = "github"
		}
	}
	if provider != "gitlab" && provider != "github" {
		return "", "", fmt.Errorf("provider must be gitlab or github")
	}
	if host == "github.com" && provider != "github" {
		return "", "", fmt.Errorf("github.com requires the github provider")
	}
	return host, provider, nil
}

func ResolveProject(root, origin string, settings Settings, getenv func(string) string) (Project, error) {
	return resolveProject(root, origin, settings, getenv, false)
}

func resolveProject(root, origin string, settings Settings, getenv func(string) string, template bool) (Project, error) {
	u, err := parseOrigin(origin)
	if err != nil {
		return Project{}, err
	}
	host := strings.ToLower(u.Hostname())
	provider := settings.Provider
	if provider == "" {
		provider = "gitlab"
		if host == "github.com" {
			provider = "github"
		}
	}
	if provider != "github" && provider != "gitlab" {
		return Project{}, fmt.Errorf("provider must be gitlab or github")
	}
	if host == "github.com" && provider != "github" {
		return Project{}, fmt.Errorf("github.com requires the github provider")
	}
	base := u.Scheme + "://" + strings.ToLower(u.Host)
	if u.Scheme == "ssh" {
		authority := host
		if strings.Contains(host, ":") {
			authority = "[" + host + "]"
		}
		base = "https://" + authority
	}
	endpoint := strings.TrimRight(getenv("GITLAB_BASE_URL"), "/")
	if provider == "github" {
		endpoint = strings.TrimRight(getenv("GITHUB_API_URL"), "/")
		if endpoint == "" {
			endpoint = base + "/api/v3"
			if host == "github.com" {
				endpoint = "https://api.github.com"
			}
		}
	} else if endpoint == "" {
		endpoint = base + "/api/v4"
	}
	api, err := url.Parse(endpoint)
	if err != nil || api.Hostname() == "" || api.User != nil || api.RawQuery != "" || api.Fragment != "" {
		return Project{}, fmt.Errorf("invalid hosting API endpoint")
	}
	path := u.Path
	if provider == "gitlab" {
		if (api.Scheme != "http" && api.Scheme != "https") || !strings.EqualFold(api.Hostname(), host) || !strings.HasSuffix(api.Path, "/api/v4") {
			return Project{}, fmt.Errorf("GITLAB_BASE_URL must be an HTTP(S) API endpoint on the origin host ending in /api/v4")
		}
		prefix := strings.TrimSuffix(api.Path, "/api/v4")
		if u.Scheme != "ssh" && prefix != "" {
			if !strings.HasPrefix(path, prefix+"/") {
				return Project{}, fmt.Errorf("origin path does not match the GitLab API URL prefix")
			}
			path = strings.TrimPrefix(path, prefix)
		}
	} else {
		validHost := strings.EqualFold(api.Hostname(), host) || (host == "github.com" && api.Hostname() == "api.github.com")
		validPath := api.Path == "/api/v3"
		if host == "github.com" {
			validPath = api.Path == "" || api.Path == "/"
		}
		if api.Scheme != "https" || !validHost || !validPath {
			return Project{}, fmt.Errorf("GITHUB_API_URL must be the HTTPS API endpoint for the origin host")
		}
	}
	id := strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(id, "/")
	if len(parts) < 2 || (provider == "github" && len(parts) != 2) {
		return Project{}, fmt.Errorf("origin must contain a namespace and repository")
	}
	for _, part := range parts {
		if part == "." || part == ".." || !repositorySegment.MatchString(part) {
			return Project{}, fmt.Errorf("invalid origin repository path")
		}
	}
	name := parts[len(parts)-1]
	if !template && !repositoryName.MatchString(name) {
		return Project{}, fmt.Errorf("origin repository name must match puppet-<module>")
	}
	remote := base + "/" + id + ".git"
	if provider == "gitlab" {
		remote = strings.TrimSuffix(endpoint, "/api/v4") + "/" + id + ".git"
	}
	return Project{Root: root, Origin: origin, Remote: remote, Endpoint: endpoint, ID: id, Name: name, Provider: provider, Template: template}, nil
}

func Identity(remote string) string {
	if u, err := parseOrigin(remote); err == nil {
		return strings.ToLower(u.Hostname()) + "/" + strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	}
	value, err := filepath.Abs(remote)
	if err != nil {
		return remote
	}
	return strings.TrimSuffix(value, ".git")
}

// Package scaffold generates a neutral, editable schema-1 template starter.
package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:embed all:starter
var starter embed.FS

type Options struct{ Name, Provider, Version, PDK, VoxBox string }

func Files(o Options) (map[string][]byte, error) {
	if !regexp.MustCompile(`^[a-z][a-z0-9_-]*$`).MatchString(o.Name) {
		return nil, fmt.Errorf("name must start with a lowercase letter and contain only lowercase letters, digits, underscores or hyphens")
	}
	if o.Provider != "gitlab" && o.Provider != "github" && o.Provider != "both" {
		return nil, fmt.Errorf("provider must be gitlab, github or both")
	}
	if !regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(o.Version) {
		return nil, fmt.Errorf("starter CI requires a stable pmtool release version")
	}
	files := map[string][]byte{}
	err := fs.WalkDir(starter, "starter", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := starter.ReadFile(path)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(path, "starter/")] = []byte(strings.ReplaceAll(string(body), "PMTOOL_TEMPLATE_NAME", o.Name))
		return nil
	})
	if err != nil {
		return nil, err
	}
	ci := map[string]string{}
	if o.Provider != "github" {
		ci["gitlab"] = "ci/puppet-module.yml"
	}
	if o.Provider != "gitlab" {
		ci["github"] = ".github/workflows/puppet-module.yml"
	}
	manifest := map[string]any{"schema": 1, "name": o.Name, "files_root": "moduleroot", "defaults": "config_defaults.yml", "contracts": []string{}, "ci": ci,
		"audit": map[string]any{"metadata": map[string]any{"semantic_version": true}, "tracked_ignored": true}}
	files["template.yml"], err = yaml.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	bootstrap := bootstrapShell(o.Version)
	if o.Provider != "github" {
		files["ci/puppet-module.yml"], err = gitlabCI(o, bootstrap, false)
		if err != nil {
			return nil, err
		}
		files[".gitlab-ci.yml"], err = gitlabCI(o, bootstrap, true)
		if err != nil {
			return nil, err
		}
	}
	if o.Provider != "gitlab" {
		files[".github/workflows/puppet-module.yml"], err = githubCI(o, bootstrap, false)
		if err != nil {
			return nil, err
		}
		files[".github/workflows/template-test.yml"], err = githubCI(o, bootstrap, true)
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

// Generate accepts a new destination only. Existing files, directories and
// symlinks are never replaced, even when they contain identical starter content.
func Generate(destination string, files map[string][]byte) ([]string, error) {
	root, err := filepath.Abs(destination)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil {
		return nil, err
	}
	if parent != filepath.Dir(root) {
		return nil, fmt.Errorf("output parent must not contain symlinks")
	}
	var names []string
	for name := range files {
		if name == "" || filepath.IsAbs(name) || strings.ContainsAny(name, "\\\x00\r\n") {
			return nil, fmt.Errorf("unsafe starter path")
		}
		for part := range strings.SplitSeq(name, "/") {
			if part == "" || part == "." || part == ".." || part == ".git" {
				return nil, fmt.Errorf("unsafe starter path")
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if err := os.Mkdir(root, 0700); err != nil {
		return nil, fmt.Errorf("template output must be a new directory: %w", err)
	}
	for _, name := range names {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		if err := writeNew(path, files[name]); err != nil {
			return nil, fmt.Errorf("starter is incomplete at %s: %w", root, err)
		}
	}
	return names, nil
}

func writeNew(path string, body []byte) (caught error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- Caller-selected new template directory; validated relative paths and exclusive creation.
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); caught == nil {
			caught = err
		}
	}()
	_, err = file.Write(body)
	return err
}

func bootstrapShell(version string) string {
	return strings.ReplaceAll(starterBootstrap, "PMTOOL_RELEASE_VERSION", version)
}

// Install with the SDK's Ruby, not curl. Both pinned SDKs contain Ruby; PDK
// supplies its own versions outside PATH. The same script runs in both CI systems.
const starterBootstrap = `set -eu
if command -v pdk >/dev/null 2>&1; then
  pmtool_ruby="$(for candidate in /opt/puppetlabs/pdk/private/ruby/*/bin/ruby; do if [ -x "$candidate" ]; then printf '%s\n' "$candidate"; fi; done | sort -V | tail -n 1)"
  test -n "$pmtool_ruby"
else
  pmtool_ruby=ruby
fi
pmtool_dir="$(mktemp -d /tmp/pmtool-ci.XXXXXX)"
"$pmtool_ruby" - "$pmtool_dir" <<'RUBY'
require 'digest'
require 'net/http'
require 'openssl'
require 'rbconfig'
require 'uri'

def download(address, remaining = 5)
  uri = URI(address)
  raise 'Expected credential-free HTTPS download' unless uri.scheme == 'https' && uri.host && !uri.userinfo && !uri.fragment
  response = Net::HTTP.start(uri.host, uri.port, use_ssl: true, verify_mode: OpenSSL::SSL::VERIFY_PEER,
                            open_timeout: 15, read_timeout: 120) { |http| http.get(uri.request_uri) }
  return response.body.b if response.is_a?(Net::HTTPSuccess) && response.body && !response.body.empty?
  if response.is_a?(Net::HTTPRedirection) && remaining.positive? && response['location']
    return download(URI.join(uri, response['location']).to_s, remaining - 1)
  end
  raise "pmtool download failed: HTTP #{response.code}"
end

architecture = case RbConfig::CONFIG.fetch('host_cpu')
               when 'x86_64', 'amd64' then 'amd64'
               when 'aarch64', 'arm64' then 'arm64'
               else raise 'Unsupported CI architecture'
               end
version = 'PMTOOL_RELEASE_VERSION'
binary = "pmtool-#{version}-linux-#{architecture}"
prefix = "https://github.com/zxzharmlesszxz/puppet-module-tooling-launcher/releases/download/#{version}"
body = download("#{prefix}/#{binary}")
entries = download("#{prefix}/SHA256SUMS").lines.select { |line| line.split.last&.delete_prefix('*') == binary }
raise 'Expected exactly one checksum entry' unless entries.length == 1
checksum = /\A([0-9a-fA-F]{64})[ \t]+\*?#{Regexp.escape(binary)}\r?\n?\z/.match(entries.first)&.captures&.first
raise 'pmtool checksum mismatch' unless checksum && Digest::SHA256.hexdigest(body) == checksum.downcase
target = File.join(ARGV.fetch(0), 'pmtool')
File.binwrite(target, body)
File.chmod(0700, target)
RUBY
export PATH="$pmtool_dir:$PATH"
if [ -n "${CI_PROJECT_URL:-}" ]; then
  git remote set-url origin "${CI_PROJECT_URL%.git}.git"
  if [ -z "${TOOLING_TEMPLATE_TOKEN:-}" ] && [ -n "${CI_JOB_TOKEN:-}" ]; then
    export TOOLING_TEMPLATE_TOKEN="$CI_JOB_TOKEN"
    export TOOLING_TEMPLATE_TOKEN_HOST="${CI_SERVER_HOST:?}"
    export TOOLING_TEMPLATE_USERNAME=gitlab-ci-token
  fi
fi
pmtool version
`

func starterJobs(template bool) map[string]string {
	if template {
		return map[string]string{"template-test": "template-test"}
	}
	return map[string]string{"module-audit": "audit", "pdk-validate": "pdk --stage validate", "pdk-unit": "pdk --stage unit", "voxbox-validate": "voxbox --stage validate", "voxbox-unit": "voxbox --stage unit"}
}

func gitlabCI(o Options, bootstrap string, template bool) ([]byte, error) {
	root := map[string]any{"stages": []string{"check"}, "workflow": map[string]any{"rules": []any{map[string]any{"if": `$CI_PIPELINE_SOURCE == "push"`}, map[string]any{"when": "never"}}}}
	for name, command := range starterJobs(template) {
		image := o.VoxBox
		if strings.HasPrefix(name, "pdk-") {
			image = o.PDK
		}
		rule := `$CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH`
		if template {
			rule = `$CI_PIPELINE_SOURCE == "push"`
		}
		root[name] = map[string]any{"stage": "check", "image": map[string]any{"name": image, "entrypoint": []string{""}}, "before_script": []string{bootstrap}, "script": []string{"pmtool --executor native " + command}, "rules": []any{map[string]any{"if": rule}}}
	}
	return yaml.Marshal(root)
}

func githubCI(o Options, bootstrap string, template bool) ([]byte, error) {
	secrets := map[string]any{}
	environment := map[string]string{}
	for _, name := range []string{"TOOLING_TEMPLATE_TOKEN", "TOOLING_TEMPLATE_TOKEN_HOST", "TOOLING_TEMPLATE_USERNAME"} {
		secrets[name] = map[string]any{"required": false}
		environment[name] = "${{ secrets." + name + " }}"
	}
	for _, name := range []string{"FORGE_URL", "FORGE_PUBLISH_TOKEN"} {
		secrets[name] = map[string]any{"required": false}
	}
	on := map[string]any{"workflow_call": map[string]any{"secrets": secrets}}
	if template {
		on = map[string]any{"push": map[string]any{}}
	}
	jobs := map[string]any{}
	for name, command := range starterJobs(template) {
		image := o.VoxBox
		if strings.HasPrefix(name, "pdk-") {
			image = o.PDK
		}
		jobs[name] = map[string]any{"runs-on": "ubuntu-latest", "container": map[string]any{"image": image}, "steps": []any{
			map[string]any{"uses": "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1", "with": map[string]any{"persist-credentials": false}},
			map[string]any{"name": "Install verified pmtool", "shell": "sh", "run": bootstrap},
			map[string]any{"name": name, "shell": "sh", "run": "pmtool --executor native " + command},
		}}
		// GITHUB_PATH persists the installer path across isolated Actions steps.
		steps := jobs[name].(map[string]any)["steps"].([]any)
		if name == "module-audit" {
			steps[2].(map[string]any)["env"] = environment
		}
		steps[1].(map[string]any)["run"] = bootstrap + "printf '%s\n' \"$pmtool_dir\" >> \"$GITHUB_PATH\"\n"
	}
	return yaml.Marshal(map[string]any{"name": "pmtool checks", "on": on, "permissions": map[string]string{"contents": "read"}, "jobs": jobs})
}

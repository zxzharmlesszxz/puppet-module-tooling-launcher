package templatekit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

type RunCheck func(context.Context, string, string, string) error
type GitCall func(...string) (string, error)
type Engine struct {
	Root, Provider, OriginHost, CI, LegacyCI, Files string
	Template                                        *Template
	Render                                          Render
	Check                                           RunCheck
	Git                                             GitCall
	Out                                             io.Writer
	Context                                         context.Context
	BeforeApply                                     func()
	Color                                           bool
}
type Change struct {
	Path              string
	Original, Content []byte
	Existed, Remove   bool
	Mode              os.FileMode
}
type Plan struct {
	Managed map[string]string
	Changes []Change
	Errors  []string
}
type Inventory struct {
	Schema   int               `json:"schema"`
	Template InventoryTemplate `json:"template"`
	Files    map[string]string `json:"files"`
}
type InventoryTemplate struct {
	Use      string `json:"use"`
	Revision string `json:"revision"`
}

func (e *Engine) inventory() (Inventory, error) {
	result := Inventory{Schema: 1, Files: map[string]string{}}
	path, err := SafePath(e.Root, Lock)
	if err != nil {
		return result, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- SafePath verified the inventory path.
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	var parsed Inventory
	if err := json.Unmarshal(data, &parsed); err != nil {
		return result, fmt.Errorf("invalid template inventory: %w", err)
	}
	if parsed.Schema != 1 || parsed.Files == nil {
		return result, fmt.Errorf("invalid template inventory")
	}
	result = parsed
	for target, digest := range result.Files {
		if err := Path(target); err != nil {
			return result, err
		}
		if target == Lock || target == ".sync.yml" || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(digest) {
			return result, fmt.Errorf("invalid template inventory target/checksum")
		}
	}
	return result, nil
}
func (e *Engine) metadata() (Mapping, string, error) {
	body, err := readFile(e.Root, "metadata.json")
	if err != nil {
		return nil, "", fmt.Errorf("metadata.json: %w", err)
	}
	var m Mapping
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, "", err
	}
	parts := strings.SplitN(str(m["name"]), "-", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("metadata.json: prepare a real owner-module name")
	}
	return m, parts[1], nil
}
func (e *Engine) Plan(sync bool) (Plan, error) {
	p := Plan{Managed: map[string]string{}}
	m, namespace, err := e.metadata()
	if err != nil {
		return p, err
	}
	if sync {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]*-[a-z][a-z0-9_]*$`).MatchString(str(m["name"])) || !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(str(m["version"])) {
			return p, fmt.Errorf("metadata.json: prepare a real module name and version before sync")
		}
	}
	if _, err := SafePath(e.Root, ".sync.yml"); err != nil {
		return p, err
	}
	overrides, err := readYAML(filepath.Join(e.Root, ".sync.yml"), true)
	if err != nil {
		return p, err
	}
	selection, err := e.Template.Select(overrides, namespace)
	if err != nil {
		return p, err
	}
	previous, err := e.inventory()
	if err != nil {
		return p, err
	}
	target, ci, err := e.Template.CIConfig(e.Provider, e.OriginHost, e.CI, e.LegacyCI)
	if err != nil {
		return p, err
	}
	defaults := map[string]Mapping{}
	maps.Copy(defaults, e.Template.Defaults)
	defaults[target] = ci
	baseline := map[string]bool{Lock: true, ".gitlab-ci.yml": true, GitHubCI: true}
	contracts := map[string]bool{}
	for name := range defaults {
		v, err := namespacePath(name, namespace)
		if err != nil {
			return p, err
		}
		baseline[v] = true
	}
	for _, g := range e.Template.Groups {
		for _, name := range g.Targets {
			v, err := namespacePath(name, namespace)
			if err != nil {
				return p, err
			}
			contracts[v] = true
		}
	}
	approved := map[string]bool{}
	for _, file := range strings.Fields(e.Files) {
		if !baseline[file] {
			if _, known := previous.Files[file]; !known {
				return p, fmt.Errorf("FILES contains non-baseline paths: %s", file)
			}
		}
		approved[file] = true
	}
	problems := e.metadataChecks(m, sync)
	p.Errors = append(p.Errors, problems...)
	for name, c := range selection.Overrides {
		if !baseline[name] {
			continue
		}
		customAllowed := truth(e.Template.Audit["allow_custom_files"]) && truth(c["custom"])
		if truth(c["unmanaged"]) && !contracts[name] && !customAllowed {
			p.Errors = append(p.Errors, "baseline target cannot be unmanaged: "+name)
		}
		if truth(c["custom"]) && !customAllowed {
			p.Errors = append(p.Errors, "baseline target cannot be custom: "+name)
		}
	}
	if rule := mapping(e.Template.Audit["coverage"]); rule != nil {
		file := str(rule["file"])
		c := merge(defaults[file], selection.Overrides[file])
		want, _ := integer(rule["minimum"])
		actual, valid := integer(c[str(rule["minimum_key"])])
		if !truth(c[str(rule["enabled_key"])]) || !valid || actual != want {
			p.Errors = append(p.Errors, fmt.Sprintf("resource coverage is mandatory and must be exactly %d%%", want))
		}
	}
	actualCI := merge(ci, selection.Overrides[target])
	if raw, ok := actualCI["unmanaged"].(bool); !ok || raw {
		p.Errors = append(p.Errors, "central CI must be managed")
	}
	for _, key := range []string{"project", "file", "ref", "host", "remote"} {
		if !reflect.DeepEqual(ci[key], actualCI[key]) {
			p.Errors = append(p.Errors, "invalid central CI "+key)
		}
	}
	for _, group := range e.Template.Groups {
		var n string
		for _, name := range group.Targets {
			path, err := namespacePath(name, namespace)
			if err != nil {
				return p, err
			}
			config := merge(defaults[name], selection.Overrides[path])
			if !truth(config["unmanaged"]) {
				if raw, exists := config["namespace"]; exists && raw != nil {
					if _, valid := raw.(string); !valid {
						p.Errors = append(p.Errors, "contract namespace must be a string")
						continue
					}
				}
				current := str(config["namespace"])
				if current == "" {
					current = namespace
				}
				if n != "" && n != current {
					p.Errors = append(p.Errors, "contract group uses different namespaces")
				}
				n = current
				if current != namespace {
					p.Errors = append(p.Errors, "contract namespace must match module namespace")
				}
			}
		}
	}
	type entry struct {
		target   string
		disabled bool
	}
	var entries []entry
	var docs []Document
	order := keysDefaults(defaults)
	for _, name := range order {
		path, err := namespacePath(name, namespace)
		if err != nil {
			return p, err
		}
		config := merge(defaults[name], selection.Overrides[path])
		if contracts[path] && str(config["namespace"]) == "" {
			config["namespace"] = namespace
		}
		if _, exists := config["unmanaged"]; exists {
			if _, valid := config["unmanaged"].(bool); !valid {
				return p, fmt.Errorf("%s.unmanaged must be Boolean", path)
			}
		}
		disabled := truth(config["unmanaged"])
		if disabled && !selection.Disabled[path] {
			if contracts[path] {
				if _, err := os.Lstat(filepath.Join(e.Root, path)); err == nil {
					p.Errors = append(p.Errors, "disabled contract file still exists: "+path)
				}
			} else if truth(config["custom"]) {
				if _, err := readFile(e.Root, path); err != nil {
					p.Errors = append(p.Errors, "custom file missing: "+path)
				}
			}
			continue
		}
		var source []byte
		if name == target {
			if e.Provider == "github" {
				source = []byte(githubCISource)
			} else {
				source = []byte(gitlabCISource)
			}
		} else {
			source, err = e.Template.Source(name)
			if err != nil {
				return p, err
			}
		}
		doc, err := e.Template.document(source, config, m)
		if err != nil {
			return p, err
		}
		docs = append(docs, doc)
		entries = append(entries, entry{path, disabled})
	}
	var rendered [][]byte
	if len(docs) > 0 {
		if e.Render == nil {
			return p, fmt.Errorf("ERB renderer is required")
		}
		rendered, err = e.Render(e.Context, docs)
		if err != nil {
			return p, err
		}
		if len(rendered) != len(entries) {
			return p, fmt.Errorf("ERB renderer returned wrong document count")
		}
	}
	inventory := Inventory{Schema: 1, Template: InventoryTemplate{e.Template.Reference, e.Template.Revision}, Files: map[string]string{}}
	released := map[string]bool{}
	for _, name := range order {
		path, _ := namespacePath(name, namespace)
		config := merge(defaults[name], selection.Overrides[path])
		if truth(config["custom"]) && truth(e.Template.Audit["allow_custom_files"]) {
			released[path] = true
		}
	}
	for i, item := range entries {
		path, err := SafePath(e.Root, item.target)
		if err != nil {
			return p, err
		}
		current, err := os.ReadFile(path) // #nosec G304 -- SafePath verified the selected module target.
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return p, err
		}
		mode := os.FileMode(0644)
		if exists {
			info, err := os.Stat(path)
			if err != nil {
				return p, err
			}
			if !info.Mode().IsRegular() {
				return p, fmt.Errorf("not a regular file: %s", item.target)
			}
			mode = info.Mode().Perm()
		}
		if item.disabled {
			if !exists {
				continue
			}
			if !sync {
				p.Errors = append(p.Errors, "disabled contract file still exists: "+item.target)
				continue
			}
			p.Changes = append(p.Changes, Change{item.target, current, nil, true, true, mode})
			if !reflect.DeepEqual(current, rendered[i]) && !approved[item.target] {
				p.Errors = append(p.Errors, "conflict: "+item.target+"; locally changed file is preserved; approve removal with --files")
			}
			continue
		}
		inventory.Files[item.target] = hash(rendered[i])
		p.Managed[item.target] = inventory.Files[item.target]
		if exists && string(current) == string(rendered[i]) {
			continue
		}
		p.Changes = append(p.Changes, Change{item.target, current, rendered[i], exists, false, mode})
		if !sync {
			p.Errors = append(p.Errors, "managed file drift or missing: "+item.target)
		} else if exists && !approved[item.target] {
			p.Errors = append(p.Errors, "conflict: "+item.target+"; review diff and explicitly approve replacement with --files")
		}
	}
	for target, digest := range previous.Files {
		if _, managed := inventory.Files[target]; managed || released[target] {
			continue
		}
		already := false
		for _, c := range p.Changes {
			already = already || c.Path == target
		}
		if already {
			continue
		}
		path, err := SafePath(e.Root, target)
		if err != nil {
			return p, err
		}
		current, err := os.ReadFile(path) // #nosec G304 -- SafePath verified the selected module target.
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return p, err
		}
		if !sync {
			p.Errors = append(p.Errors, "retired template file still exists: "+target)
			continue
		}
		p.Changes = append(p.Changes, Change{target, current, nil, true, true, 0644})
		if hash(current) != digest && !approved[target] {
			p.Errors = append(p.Errors, "conflict: "+target+"; locally changed file is preserved; approve removal with --files")
		}
	}
	content, err := jsonBytes(inventory)
	if err != nil {
		return p, err
	}
	path, err := SafePath(e.Root, Lock)
	if err != nil {
		return p, err
	}
	current, err := os.ReadFile(path) // #nosec G304 -- SafePath verified the selected module target.
	if err != nil && !os.IsNotExist(err) {
		return p, err
	}
	lockMode := os.FileMode(0644)
	if err == nil {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return p, statErr
		}
		lockMode = info.Mode().Perm()
	}
	if string(current) != string(content) {
		if !sync {
			p.Errors = append(p.Errors, "managed file drift or missing: "+Lock)
		}
		p.Changes = append(p.Changes, Change{Lock, current, content, err == nil, false, lockMode})
	}
	sort.SliceStable(p.Changes, func(i, j int) bool { return p.Changes[i].Path < p.Changes[j].Path })
	return p, nil
}
func keysDefaults(m map[string]Mapping) []string {
	var r []string
	for k := range m {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
func (e *Engine) metadataChecks(m Mapping, sync bool) []string {
	var problems []string
	rules := mapping(e.Template.Audit["metadata"])
	version := str(m["version"])
	if truth(rules["semantic_version"]) && !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version) {
		problems = append(problems, "metadata.json: invalid semantic version")
	}
	for _, key := range mustStrings(rules["forbidden_keys"]) {
		if _, exists := m[key]; exists {
			problems = append(problems, "metadata.json: remove "+key)
		}
	}
	if sync {
		return problems
	}
	if rule := mapping(rules["changelog"]); rule != nil {
		data, err := readFile(e.Root, str(rule["file"]))
		heading := strings.ReplaceAll(str(rule["heading"]), "%{version}", version)
		if err != nil || !regexp.MustCompile(`(?m)^## `+regexp.QuoteMeta(heading)+`(?:\s|$)`).Match(data) {
			problems = append(problems, str(rule["file"])+": missing "+heading)
		}
	}
	for _, key := range mustStrings(rules["url_fields"]) {
		if strings.Contains(str(m[key]), "://git@") {
			problems = append(problems, "metadata.json: invalid "+key)
		}
	}
	if truth(rules["no_self_symlinks"]) {
		path, err := SafePath(e.Root, ".fixtures.yml")
		var fixtures Mapping
		if err == nil {
			fixtures, err = readYAML(path, true)
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf(".fixtures.yml: %v", err))
		} else if symlinks, err := fixtureSymlinks(fixtures); err != nil {
			problems = append(problems, err.Error())
		} else if len(symlinks) > 0 {
			problems = append(problems, ".fixtures.yml: self symlinks are not allowed")
		}
	}
	if truth(e.Template.Audit["tracked_ignored"]) {
		if e.Git == nil {
			problems = append(problems, "Git audit is required")
		} else {
			files, err := e.Git("ls-files", "-ci", "--exclude-standard")
			if err != nil {
				problems = append(problems, err.Error())
			}
			for _, file := range strings.Split(strings.TrimSpace(files), "\n") {
				if file != "" {
					problems = append(problems, "tracked ignored file: "+file)
				}
			}
		}
	}
	return problems
}
func mustStrings(v any) []string { r, _ := stringsList(v); return r }

const gitlabCISource = "include:\n<% if @configs['remote'] -%>\n  - remote: '<%= @configs.fetch('remote') %>'\n<% else -%>\n  - project: '<%= @configs.fetch('project') %>'\n    ref: '<%= @configs.fetch('ref') %>'\n    file: '<%= @configs.fetch('file') %>'\n<% end -%>\n"
const githubCISource = "name: Puppet module\n'on':\n  push:\n    branches: ['**']\n    tags: ['v*']\npermissions:\n  contents: read\n  actions: read\njobs:\n  module:\n    uses: <%= @configs.fetch('project') %><%= @configs.fetch('file') %>@<%= @configs.fetch('ref') %>\n    secrets:\n      FORGE_URL: ${{ secrets.FORGE_URL }}\n      FORGE_PUBLISH_TOKEN: ${{ secrets.FORGE_PUBLISH_TOKEN }}\n"

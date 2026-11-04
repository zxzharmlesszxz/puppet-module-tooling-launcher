// Package templatekit implements versioned template loading, sync and general audit.
package templatekit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Mapping = map[string]any

func mapping(v any) Mapping     { m, _ := v.(map[string]any); return m }
func str(v any) string          { s, _ := v.(string); return s }
func truth(v any) bool          { b, _ := v.(bool); return b }
func integer(v any) (int, bool) { n, ok := v.(int); return n, ok }
func stringsList(v any) ([]string, bool) {
	if v == nil {
		return nil, false
	}
	list, ok := v.([]any)
	if !ok {
		if a, yes := v.([]string); yes {
			return a, true
		}
		return nil, false
	}
	r := make([]string, len(list))
	for i, v := range list {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		r[i] = s
	}
	return r, true
}
func listField(m Mapping, key string) ([]string, bool) {
	raw, exists := m[key]
	if !exists {
		return []string{}, true
	}
	return stringsList(raw)
}
func contains(values []string, name string) bool {
	return slices.Contains(values, name)
}
func unique(values []string) bool {
	seen := map[string]bool{}
	for _, v := range values {
		if seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
func allowed(m Mapping, names ...string) bool {
	for k := range m {
		if !contains(names, k) {
			return false
		}
	}
	return true
}
func merge(a, b Mapping) Mapping {
	r := Mapping{}
	maps.Copy(r, a)
	maps.Copy(r, b)
	return r
}
func hash(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
func YAML(data []byte) (Mapping, error) {
	m := Mapping{}
	if len(data) == 0 {
		return m, nil
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("expected YAML mapping: %w", err)
	}
	if m == nil {
		m = Mapping{}
	}
	return m, nil
}
func readYAML(path string, optional bool) (Mapping, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- Caller supplies a module/template-scoped path; symlink components are checked.
	if optional && os.IsNotExist(err) {
		return Mapping{}, nil
	}
	if err != nil {
		return nil, err
	}
	return YAML(data)
}
func Path(name string) error {
	if name == "" || filepath.IsAbs(name) || strings.ContainsAny(name, "\\\x00\r\n") {
		return fmt.Errorf("unsafe template path: %q", name)
	}
	for _, p := range strings.Split(name, "/") {
		if p == "" || p == "." || p == ".." || p == ".git" {
			return fmt.Errorf("unsafe template path: %q", name)
		}
	}
	return nil
}
func SafePath(root, name string) (string, error) {
	if err := Path(name); err != nil {
		return "", err
	}
	current := root
	for i, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing symlink: %s", name)
		}
		if i < len(strings.Split(name, "/"))-1 && !info.IsDir() {
			return "", fmt.Errorf("not a directory: %s", current)
		}
	}
	return current, nil
}
func readFile(root, name string) ([]byte, error) {
	path, err := SafePath(root, name)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", name)
	}
	return os.ReadFile(path) // #nosec G304 -- SafePath and the regular-file check above constrain the template/module read.
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var versionPattern = regexp.MustCompile(`^(?:v?[0-9]+(?:\.[0-9]+){0,2}(?:[-+][A-Za-z0-9.-]+)?|[a-fA-F0-9]{40})$`)

const Lock = ".sync.lock.json"
const GitHubCI = ".github/workflows/puppet-module.yml"

type Group struct {
	Targets        []string `json:"targets"`
	Types          []string `json:"types"`
	LegacyOptional []string `json:"legacy_optional_targets"`
}
type Template struct {
	Root, Reference, Revision, FilesRoot string
	Defaults                             map[string]Mapping
	Groups                               map[string]Group
	Contracts                            []string
	GroupOrder                           []string
	AllowOverrides                       bool
	Audit, CI                            Mapping
}

func Load(root, reference, revision string) (*Template, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	body, err := readFile(root, "template.yml")
	if err != nil {
		return nil, err
	}
	manifest, err := YAML(body)
	if err != nil {
		return nil, err
	}
	schema, ok := integer(manifest["schema"])
	if !ok || schema != 1 || !allowed(manifest, "schema", "name", "files_root", "defaults", "contracts", "contract_groups", "audit", "allow_contract_overrides", "ci") {
		return nil, fmt.Errorf("template.yml: expected schema 1 mapping with known fields")
	}
	t := &Template{Root: root, Reference: reference, Revision: revision, FilesRoot: str(manifest["files_root"]), Defaults: map[string]Mapping{}, Groups: map[string]Group{}, Audit: Mapping{}, CI: Mapping{}}
	if err := Path(t.FilesRoot); err != nil {
		return nil, err
	}
	defaultsFile := str(manifest["defaults"])
	data, err := readFile(root, defaultsFile)
	if err != nil {
		return nil, err
	}
	defaults, err := YAML(data)
	if err != nil {
		return nil, err
	}
	for target, value := range defaults {
		if err := Path(target); err != nil {
			return nil, err
		}
		if contains([]string{Lock, ".sync.yml", ".gitlab-ci.yml", GitHubCI}, target) {
			return nil, fmt.Errorf("reserved engine file: %s", target)
		}
		config := mapping(value)
		if config == nil {
			return nil, fmt.Errorf("template defaults must be a file mapping")
		}
		t.Defaults[target] = config
		if _, err := t.Source(target); err != nil {
			return nil, err
		}
	}
	if raw, exists := manifest["contract_groups"]; exists {
		groups := mapping(raw)
		if groups == nil {
			return nil, fmt.Errorf("contract_groups must be a mapping")
		}
		owned := map[string]bool{}
		for name, raw := range groups {
			g := mapping(raw)
			if !namePattern.MatchString(name) || g == nil || !allowed(g, "files", "types", "legacy_optional_files") {
				return nil, fmt.Errorf("invalid contract group: %s", name)
			}
			files, ok := listField(g, "files")
			if !ok || len(files) == 0 {
				return nil, fmt.Errorf("contract files must be a nonempty array")
			}
			types, ok := listField(g, "types")
			if !ok {
				return nil, fmt.Errorf("contract types must be names")
			}
			optional, ok := listField(g, "legacy_optional_files")
			if !ok {
				return nil, fmt.Errorf("invalid optional contract files")
			}
			for _, target := range files {
				if _, ok := t.Defaults[target]; !ok {
					return nil, fmt.Errorf("unknown contract file: %s", target)
				}
				if owned[target] {
					return nil, fmt.Errorf("template contract files overlap")
				}
				owned[target] = true
			}
			for _, v := range types {
				if !namePattern.MatchString(v) {
					return nil, fmt.Errorf("contract types must be names")
				}
			}
			for _, v := range optional {
				if !contains(files, v) {
					return nil, fmt.Errorf("invalid optional contract files")
				}
			}
			t.Groups[name] = Group{files, types, optional}
		}
	}
	var node yaml.Node
	if err := yaml.Unmarshal(body, &node); err != nil {
		return nil, err
	}
	if len(node.Content) > 0 {
		top := node.Content[0]
		for i := 0; i+1 < len(top.Content); i += 2 {
			if top.Content[i].Value == "contract_groups" {
				g := top.Content[i+1]
				for j := 0; j+1 < len(g.Content); j += 2 {
					t.GroupOrder = append(t.GroupOrder, g.Content[j].Value)
				}
			}
		}
	}
	t.Contracts = t.groupNames()
	if raw, exists := manifest["contracts"]; exists {
		var ok bool
		t.Contracts, ok = stringsList(raw)
		if !ok || !unique(t.Contracts) {
			return nil, fmt.Errorf("invalid template contracts")
		}
		for _, v := range t.Contracts {
			if _, ok := t.Groups[v]; !ok {
				return nil, fmt.Errorf("unknown template contract: %s", v)
			}
		}
	}
	if raw, exists := manifest["allow_contract_overrides"]; exists {
		var ok bool
		t.AllowOverrides, ok = raw.(bool)
		if !ok {
			return nil, fmt.Errorf("allow_contract_overrides must be Boolean")
		}
	}
	if raw, exists := manifest["ci"]; exists {
		t.CI = mapping(raw)
		if t.CI == nil || !allowed(t.CI, "gitlab", "github") {
			return nil, fmt.Errorf("template CI must map providers to files")
		}
		for _, v := range t.CI {
			file := str(v)
			if !strings.HasSuffix(file, ".yml") && !strings.HasSuffix(file, ".yaml") {
				return nil, fmt.Errorf("template CI must be YAML")
			}
			if _, err := readFile(root, file); err != nil {
				return nil, err
			}
		}
	}
	if raw, exists := manifest["audit"]; exists {
		t.Audit = mapping(raw)
		if t.Audit == nil {
			return nil, fmt.Errorf("template audit must be a mapping")
		}
	}
	if err := t.validateAudit(); err != nil {
		return nil, err
	}
	if t.Revision == "" {
		var paths []string
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("template symlink is not supported")
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Strings(paths)
		var contents []byte
		for _, path := range paths {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(path) // #nosec G304 -- Sorted template files; the walk rejects symlinks.
			if err != nil {
				return nil, err
			}
			contents = append(contents, []byte(filepath.ToSlash(rel)+"\x00")...)
			contents = append(contents, data...)
		}
		t.Revision = hash(contents)
	}
	return t, nil
}
func keysGroups(groups map[string]Group) []string {
	var names []string
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
func (t *Template) Source(target string) ([]byte, error) {
	return readFile(t.Root, t.FilesRoot+"/"+target+".erb")
}
func namespacePath(target, namespace string) (string, error) {
	if strings.Contains(target, "%{namespace}") {
		if !namePattern.MatchString(namespace) {
			return "", fmt.Errorf("invalid module namespace for baseline path")
		}
		target = strings.ReplaceAll(target, "%{namespace}", namespace)
	}
	return target, Path(target)
}
func jsonBytes(value any) ([]byte, error) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

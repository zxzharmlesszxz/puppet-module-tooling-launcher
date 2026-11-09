package fixturekit

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/templatekit"
	"go.yaml.in/yaml/v3"
)

const containerDirectory = "/pmtool-forge"

//go:embed resolve.rb
var dependencyInstaller []byte

var bearerToken = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]+$`)
var protectedFlag = regexp.MustCompile(`(?i)(^|\s)--?(config|confdir|module[-_]repository|forge[-_]authorization|http[-_]debug)(=|\s|$)`)

type Options struct {
	Native bool
	Getenv func(string) string
}

// Prepared holds standard installer inputs, never downloaded archive implementations.
type Prepared struct {
	Directory        string
	Auth             string
	Modules          string
	Manifest         string
	Installer        string
	link             string
	restoreDirectory bool
	restoreMode      os.FileMode
}

func (p *Prepared) Close() error {
	if p == nil {
		return nil
	}
	return errors.Join(p.removeNativeLink(), os.RemoveAll(p.Directory))
}

func (p *Prepared) RemoveCredentials() error { return os.RemoveAll(p.Auth) }

func readMapping(root, name string) (templatekit.Mapping, error) {
	filename, err := templatekit.SafePath(root, name)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, fmt.Errorf("fixture manifest must be a regular file of at most 1 MiB")
	}
	file, err := os.Open(filename) // #nosec G304 -- SafePath and the ordinary-file check constrain this module-scoped read.
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("fixture manifest exceeds 1 MiB")
	}
	return templatekit.YAML(data)
}

func fixtureManifest(root string) (templatekit.Mapping, map[string]any, error) {
	data, err := readMapping(root, ".fixtures.yml")
	if err != nil {
		return nil, nil, err
	}
	if data == nil {
		data, err = readMapping(root, ".fixtures.yaml")
		if err != nil {
			return nil, nil, err
		}
	}
	if len(data) == 0 {
		return data, nil, nil
	}
	fixtures, ok := data["fixtures"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("fixture manifest requires a fixtures mapping")
	}
	raw := fixtures["forge_modules"]
	if raw == nil {
		return data, nil, nil
	}
	modules, ok := raw.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("fixtures.forge_modules must be a mapping")
	}
	return data, modules, nil
}

func resolveDependencies(data templatekit.Mapping, modules map[string]any) (bool, error) {
	fixtures, _ := data["fixtures"].(map[string]any)
	value, exists := fixtures["resolve_dependencies"]
	if !exists {
		return false, nil
	}
	enabled, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("fixtures.resolve_dependencies must be a boolean")
	}
	if enabled && len(modules) == 0 {
		return false, fmt.Errorf("fixtures.resolve_dependencies requires forge_modules")
	}
	return enabled, nil
}

func forgeSettings(getenv func(string) string) (string, string, error) {
	if getenv == nil {
		return "", "", nil
	}
	repository, token := getenv("FORGE_URL"), getenv("FORGE_TOKEN")
	if repository == "" && token == "" {
		return "", "", nil
	}
	var missing []string
	if repository == "" {
		missing = append(missing, "FORGE_URL")
	}
	if token == "" {
		missing = append(missing, "FORGE_TOKEN")
	}
	if len(missing) != 0 {
		return "", "", fmt.Errorf("missing required inputs: %s", strings.Join(missing, ", "))
	}
	u, err := url.Parse(repository)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(repository, "\r\n\t $\\") {
		return "", "", fmt.Errorf("FORGE_URL must be a credential-free HTTPS URL without query or fragment")
	}
	if !bearerToken.MatchString(token) {
		return "", "", fmt.Errorf("FORGE_TOKEN contains invalid bearer-token characters")
	}
	return strings.TrimRight(repository, "/"), token, nil
}

func configFlags(value any, config string) (any, error) {
	var text string
	switch flags := value.(type) {
	case nil:
		return []string{"--config", config}, nil
	case string:
		text = flags
	case []any:
		for _, flag := range flags {
			s, ok := flag.(string)
			if !ok {
				return nil, fmt.Errorf("forge fixture flags must contain strings")
			}
			text += s + " "
		}
	default:
		return nil, fmt.Errorf("forge fixture flags must be a string or array")
	}
	normalized := strings.NewReplacer("'", "", "\"", "", "\\", "").Replace(text)
	if protectedFlag.MatchString(normalized) || strings.Contains(normalized, "FORGE_TOKEN") || strings.Contains(normalized, "FORGE_PUBLISH_TOKEN") {
		return nil, fmt.Errorf("forge fixture flags must not override proxy configuration or expose credentials")
	}
	if flags, ok := value.([]any); ok {
		return append(slices.Clone(flags), "--config", config), nil
	}
	return text + " --config '" + strings.ReplaceAll(config, "'", "'\"'\"'") + "'", nil
}

func configureManifest(data templatekit.Mapping, modules map[string]any, config string) error {
	fixtures := data["fixtures"].(map[string]any)
	var defaults any
	if groups, ok := fixtures["defaults"].(map[string]any); ok {
		if forge, ok := groups["forge_modules"].(map[string]any); ok {
			defaults = forge["flags"]
		}
	}
	for name, raw := range modules {
		var entry map[string]any
		switch value := raw.(type) {
		case string:
			entry = map[string]any{"repo": value}
		case map[string]any:
			entry = value
		default:
			return fmt.Errorf("forge fixture %s must be a module name or mapping", name)
		}
		repo, ok := entry["repo"].(string)
		if !ok || repo == "" || strings.ContainsAny(repo, ":@\r\n\t ") {
			return fmt.Errorf("forge fixture %s requires an owner/module name", name)
		}
		flags, exists := entry["flags"]
		if !exists {
			flags = defaults
		}
		configured, err := configFlags(flags, config)
		if err != nil {
			return fmt.Errorf("forge fixture %s: %w", name, err)
		}
		entry["flags"] = configured
		modules[name] = entry
	}
	return nil
}

func rejectRetiredSettings(root string) error {
	settings, err := readMapping(root, ".sync.yml")
	if err != nil {
		return err
	}
	if _, exists := settings["test_fixtures"]; exists {
		return fmt.Errorf("test_fixtures was removed; declare dependencies in .fixtures.yml forge_modules and configure FORGE_URL/FORGE_TOKEN")
	}
	return nil
}

// Validate checks local settings before a worker or an image lookup starts.
func Validate(root string, getenv func(string) string) error {
	if err := rejectRetiredSettings(root); err != nil {
		return err
	}
	data, modules, err := fixtureManifest(root)
	if err != nil {
		return err
	}
	resolve, err := resolveDependencies(data, modules)
	if err != nil || len(modules) == 0 {
		return err
	}
	repository, _, err := forgeSettings(getenv)
	if err != nil || (repository == "" && !resolve) {
		return err
	}
	return configureManifest(data, modules, "/pmtool-forge/puppet.conf")
}

func Prepare(root string, options Options) (prepared *Prepared, caught error) {
	if err := rejectRetiredSettings(root); err != nil {
		return nil, err
	}
	data, modules, err := fixtureManifest(root)
	if err != nil {
		return nil, err
	}
	resolve, err := resolveDependencies(data, modules)
	if err != nil || len(modules) == 0 {
		return nil, err
	}
	repository, token, err := forgeSettings(options.Getenv)
	if err != nil || (repository == "" && !resolve) {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "pmtool-forge-")
	if err != nil {
		return nil, err
	}
	prepared = &Prepared{Directory: directory, Auth: filepath.Join(directory, "auth"), Modules: filepath.Join(directory, "modules")}
	defer func() {
		if caught != nil {
			caught = errors.Join(caught, prepared.Close())
			prepared = nil
		}
	}()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return prepared, err
	}
	canonicalDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return prepared, err
	}
	relative, err := filepath.Rel(canonicalRoot, canonicalDirectory)
	if err != nil {
		return prepared, err
	}
	if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return prepared, fmt.Errorf("temporary Forge configuration must be outside the module; select an external TMPDIR")
	}
	for _, path := range []string{prepared.Auth, prepared.Modules} {
		if err := os.Mkdir(path, 0700); err != nil {
			return prepared, err
		}
	}
	manifestDirectory := containerDirectory
	if options.Native {
		manifestDirectory = prepared.Auth
		if _, err := templatekit.SafePath(root, "spec/fixtures/modules"); err != nil {
			return prepared, err
		}
	}
	if err := configureManifest(data, modules, filepath.Join(manifestDirectory, "puppet.conf")); err != nil {
		return prepared, err
	}
	rendered, err := yaml.Marshal(data)
	if err != nil {
		return prepared, err
	}
	contents := "[main]\n"
	if repository != "" {
		contents += "module_repository = " + repository + "\nforge_authorization = Bearer " + token + "\n"
	}
	files := map[string][]byte{"puppet.conf": []byte(contents), "fixtures.yml": rendered}
	if resolve {
		files["resolve.rb"] = dependencyInstaller
		prepared.Installer = filepath.Join(manifestDirectory, "resolve.rb")
	}
	for name, body := range files {
		path := filepath.Join(prepared.Auth, name)
		if err := os.WriteFile(path, body, 0600); err != nil { // #nosec G304 -- Fixed filenames in a new private directory outside the checkout.
			return prepared, err
		}
	}
	prepared.Manifest = filepath.Join(manifestDirectory, "fixtures.yml")
	if resolve && options.Native {
		if err := prepared.linkNativeModules(root); err != nil {
			return prepared, err
		}
	}
	return prepared, nil
}

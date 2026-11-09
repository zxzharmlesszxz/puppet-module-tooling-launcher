package fixturekit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/templatekit"
	"go.yaml.in/yaml/v3"
)

const maximumArchive = 256 << 20

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var modulePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*-[a-z][a-z0-9_]*$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Entry struct {
	Source  string `yaml:"source"`
	Module  string `yaml:"module"`
	Version string `yaml:"version"`
	SHA256  string `yaml:"sha256"`
	Auth    string `yaml:"auth,omitempty"`
}
type Options struct {
	Native bool
	Getenv func(string) string
	Client *http.Client
}
type Prepared struct {
	Manifest  string
	Directory string
}

func (p *Prepared) Close() error {
	if p == nil {
		return nil
	}
	return os.RemoveAll(p.Directory)
}
func readMapping(root, name string, optional bool) (templatekit.Mapping, error) {
	filename, err := templatekit.SafePath(root, name)
	if err != nil {
		return nil, err
	}
	info, statErr := os.Lstat(filename)
	if optional && errors.Is(statErr, os.ErrNotExist) {
		return templatekit.Mapping{}, nil
	}
	if statErr != nil {
		return nil, statErr
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", name)
	}
	file, err := os.Open(filename) // #nosec G304 -- Config paths pass SafePath; archive metadata paths are confined to private staging.
	if optional && errors.Is(err, os.ErrNotExist) {
		return templatekit.Mapping{}, nil
	}
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 1<<20+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("%s exceeds 1 MiB", name)
	}
	return templatekit.YAML(data)
}
func entries(root string) (map[string]Entry, error) {
	settings, err := readMapping(root, ".sync.yml", true)
	if err != nil {
		return nil, err
	}
	raw, ok := settings["test_fixtures"]
	if !ok {
		return nil, nil
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var result map[string]Entry
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid test_fixtures: %w", err)
	}
	if result == nil || len(result) > 32 {
		return nil, fmt.Errorf("test_fixtures must be a mapping of at most 32 modules")
	}
	for name, item := range result {
		_, suffix, _ := strings.Cut(item.Module, "-")
		if !namePattern.MatchString(name) || !modulePattern.MatchString(item.Module) || name != suffix || !versionPattern.MatchString(item.Version) || !digestPattern.MatchString(item.SHA256) || item.Auth != "" && item.Auth != "forge" {
			return nil, fmt.Errorf("invalid pinned test fixture %q", name)
		}
		if _, err := sourceURL(item.Source); err != nil {
			return nil, err
		}
	}
	return result, nil
}
func sourceURL(source string) (*url.URL, error) {
	u, err := url.Parse(source)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(source, "\r\n\t ") {
		return nil, fmt.Errorf("fixture source must be a credential-free HTTPS URL without query or fragment")
	}
	return u, nil
}
func download(ctx context.Context, item Entry, options Options) ([]byte, error) {
	u, err := sourceURL(item.Source)
	if err != nil {
		return nil, err
	}
	token, err := fixtureToken(u, options.Getenv, item.Auth == "forge")
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Accept", "application/octet-stream")
	client := &http.Client{Timeout: 2 * time.Minute}
	if options.Client != nil {
		*client = *options.Client
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fixture download failed")
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, maximumArchive+1))
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fixture download refused: HTTP %d (redirects are not followed)", response.StatusCode)
	}
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maximumArchive {
		return nil, fmt.Errorf("fixture archive is empty or exceeds 256 MiB")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != item.SHA256 {
		return nil, fmt.Errorf("fixture SHA256 mismatch for %s@%s", item.Module, item.Version)
	}
	return data, nil
}
func unpack(data []byte, item Entry, root string) error {
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	expanded, readErr := io.ReadAll(io.LimitReader(compressed, maximumArchive+1))
	closeErr := compressed.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return fmt.Errorf("invalid fixture gzip: %w", err)
	}
	if len(expanded) > maximumArchive {
		return fmt.Errorf("unpacked fixture exceeds 256 MiB")
	}
	return unpackTar(expanded, item, root)
}
func unpackTar(data []byte, item Entry, root string) error {
	reader := tar.NewReader(bytes.NewReader(data))
	prefix := item.Module + "-" + item.Version
	seen := map[string]bool{}
	metadataFound := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || path.Clean(name) != name || strings.Contains(name, "\\") || strings.ContainsAny(name, "\r\n\x00") || seen[name] || (name != prefix && !strings.HasPrefix(name, prefix+"/")) || header.Size > 32<<20 || len(seen) >= 100000 {
			return fmt.Errorf("unsafe or duplicate fixture archive path")
		}
		seen[name] = true
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("fixture links and special files are forbidden")
		}
		if name == prefix {
			if header.Typeflag != tar.TypeDir {
				return fmt.Errorf("fixture root must be a directory")
			}
			continue
		}
		relative := strings.TrimPrefix(name, prefix+"/")
		if relative == ".git" || strings.HasPrefix(relative, ".git/") {
			return fmt.Errorf("fixture archive must not contain Git metadata")
		}
		destination := filepath.Join(root, filepath.FromSlash(relative))
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(destination, 0700); err != nil {
				return err
			}
			continue
		}
		if err := writeEntry(destination, header, reader); err != nil {
			return err
		}
		if relative == "metadata.json" {
			if err := validateMetadata(destination, item); err != nil {
				return err
			}
			metadataFound = true
		}
	}
	if !metadataFound {
		return fmt.Errorf("fixture metadata.json is missing")
	}
	return nil
}
func writeEntry(destination string, header *tar.Header, reader io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode&0777)) // #nosec G304 -- Canonical archive paths under a new private staging root; links/special files are refused.
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, reader)
	return errors.Join(copyErr, file.Close())
}
func validateMetadata(filename string, item Entry) error {
	file, err := os.Open(filename) // #nosec G304 -- Config paths pass SafePath; archive metadata paths are confined to private staging.
	if err != nil {
		return err
	}
	info, statErr := file.Stat()
	if statErr != nil || info.Size() > 1<<20 {
		return errors.Join(fmt.Errorf("fixture metadata exceeds 1 MiB"), file.Close())
	}
	var metadata struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decodeErr := decoder.Decode(&metadata)
	trailingErr := decoder.Decode(new(any))
	closeErr := file.Close()
	if decodeErr != nil || trailingErr != io.EOF || closeErr != nil || metadata.Name != item.Module || metadata.Version != item.Version {
		return fmt.Errorf("fixture metadata identity/version mismatch")
	}
	return nil
}
func staging(root string) (string, error) {
	vendor := filepath.Join(root, "vendor")
	if err := os.Mkdir(vendor, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(vendor)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("fixture staging requires an ordinary vendor directory")
	}
	return os.MkdirTemp(vendor, ".pmtool-fixtures-")
}
func Prepare(ctx context.Context, root string, options Options) (prepared *Prepared, caught error) {
	selected, err := entries(root)
	if err != nil || len(selected) == 0 {
		return nil, err
	}
	directory, err := staging(root)
	if err != nil {
		return nil, err
	}
	prepared = &Prepared{Directory: directory}
	defer func() {
		if caught != nil {
			caught = errors.Join(caught, prepared.Close())
			prepared = nil
		}
	}()
	config, err := readMapping(root, ".fixtures.yml", true)
	if err != nil {
		return prepared, err
	}
	raw, exists := config["fixtures"]
	fixtures, ok := raw.(map[string]any)
	if !exists {
		fixtures = map[string]any{}
		config["fixtures"] = fixtures
	} else if !ok {
		return prepared, fmt.Errorf("fixtures must be a mapping")
	}
	links, err := fixtureLinks(fixtures)
	if err != nil {
		return prepared, err
	}
	sourceRoot := root
	if !options.Native {
		sourceRoot = "/repo"
	}
	metadataPath, err := templatekit.SafePath(root, "metadata.json")
	if err != nil {
		return prepared, err
	}
	metadata, err := os.ReadFile(metadataPath) // #nosec G304 -- Module-scoped metadata path passes SafePath.
	if err != nil {
		return prepared, err
	}
	var identity struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(metadata, &identity); err != nil {
		return prepared, err
	}
	if !modulePattern.MatchString(identity.Name) {
		return prepared, fmt.Errorf("invalid module fixture identity")
	}
	_, name, _ := strings.Cut(identity.Name, "-")
	if _, exists := links[name]; exists {
		return prepared, fmt.Errorf("fixture manifest must not override the module self-link")
	}
	links[name] = sourceRoot
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, fixtureName := range names {
		if fixtureName == name {
			return prepared, fmt.Errorf("a module cannot be its own test fixture")
		}
		if err := rejectCollision(fixtures, links, fixtureName); err != nil {
			return prepared, err
		}
		item := selected[fixtureName]
		data, err := download(ctx, item, options)
		if err != nil {
			return prepared, err
		}
		target := filepath.Join(directory, fixtureName)
		if err := os.Mkdir(target, 0700); err != nil {
			return prepared, err
		}
		if err := unpack(data, item, target); err != nil {
			return prepared, err
		}
		relative, err := filepath.Rel(root, target)
		if err != nil {
			return prepared, err
		}
		links[fixtureName] = filepath.ToSlash(filepath.Join(sourceRoot, relative))
	}
	fixtures["symlinks"] = links
	rendered, err := yaml.Marshal(config)
	if err != nil {
		return prepared, err
	}
	manifest := filepath.Join(directory, "fixtures.yml")
	if err := os.WriteFile(manifest, rendered, 0600); err != nil { // #nosec G304 -- Fixed filename in the new private staging directory.
		return prepared, err
	}
	relative, err := filepath.Rel(root, manifest)
	if err != nil {
		return prepared, err
	}
	prepared.Manifest = filepath.ToSlash(filepath.Join(sourceRoot, relative))
	return prepared, nil
}
func fixtureLinks(fixtures map[string]any) (map[string]any, error) {
	raw, exists := fixtures["symlinks"]
	if !exists {
		return map[string]any{}, nil
	}
	links, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("fixture symlinks must be a mapping")
	}
	return links, nil
}
func rejectCollision(fixtures, links map[string]any, name string) error {
	if _, exists := links[name]; exists {
		return fmt.Errorf("duplicate test fixture %s", name)
	}
	for _, category := range []string{"forge_modules", "repositories"} {
		if values, ok := fixtures[category].(map[string]any); ok {
			if _, exists := values[name]; exists {
				return fmt.Errorf("duplicate test fixture %s", name)
			}
		}
	}
	return nil
}

func fixtureToken(u *url.URL, getenv func(string) string, required bool) (string, error) {
	if !required {
		return "", nil
	}
	if getenv == nil {
		return "", fmt.Errorf("missing required inputs: FORGE_TOKEN, FORGE_TOKEN_HOST")
	}
	token, host := getenv("FORGE_TOKEN"), getenv("FORGE_TOKEN_HOST")
	if token == "" {
		if host == "" {
			return "", fmt.Errorf("missing required inputs: FORGE_TOKEN, FORGE_TOKEN_HOST")
		}
		return "", fmt.Errorf("missing required input: FORGE_TOKEN")
	}
	if token != "" && host == "" {
		return "", fmt.Errorf("missing required input: FORGE_TOKEN_HOST")
	}
	if token != "" && (!strings.EqualFold(host, u.Host) || strings.ContainsAny(token, "\r\n\x00")) {
		return "", fmt.Errorf("FORGE_TOKEN requires matching FORGE_TOKEN_HOST and no control characters")
	}
	return token, nil
}
func Validate(root string, getenv func(string) string) error {
	selected, err := entries(root)
	if err != nil {
		return err
	}
	for _, item := range selected {
		u, err := sourceURL(item.Source)
		if err != nil {
			return err
		}
		if _, err := fixtureToken(u, getenv, item.Auth == "forge"); err != nil {
			return err
		}
	}
	return nil
}

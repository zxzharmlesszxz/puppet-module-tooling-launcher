package fixturekit

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/templatekit"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func environment(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func authenticated() func(string) string {
	return environment(map[string]string{"FORGE_URL": "https://forge.example.test/proxy/", "FORGE_TOKEN": "fixture-read-secret"})
}

const fixtures = `fixtures:
  defaults:
    forge_modules:
      flags: --verbose
  forge_modules:
    common:
      repo: team/common
      ref: '1.0.0'
    stdlib: puppetlabs/stdlib
    apt:
      repo: puppetlabs/apt
      ref: '11.4.0'
      flags: ['--ignore-requirements']
  repositories:
    another: https://git.example.test/team/another.git
  symlinks:
    local: /module/local
notes: keep
`

func TestStandardManifestAndPrivatePuppetConfiguration(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "docker", true: "native"}[native], func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, ".fixtures.yml"), fixtures)
			prepared, err := Prepare(root, Options{Native: native, Getenv: authenticated()})
			if err != nil || prepared == nil {
				t.Fatal(prepared, err)
			}
			t.Cleanup(func() {
				if err := prepared.Close(); err != nil {
					t.Error(err)
				}
			})
			if strings.HasPrefix(prepared.Directory, root+string(filepath.Separator)) {
				t.Fatal("credentials staged in the module checkout")
			}
			config, err := os.ReadFile(filepath.Join(prepared.Auth, "puppet.conf"))
			if err != nil || string(config) != "[main]\nmodule_repository = https://forge.example.test/proxy\nforge_authorization = Bearer fixture-read-secret\n" {
				t.Fatal("incorrect native Puppet settings", err)
			}
			for _, name := range []string{prepared.Directory, prepared.Auth, filepath.Join(prepared.Auth, "puppet.conf"), filepath.Join(prepared.Auth, "fixtures.yml")} {
				info, statErr := os.Stat(name)
				if statErr != nil || info.Mode().Perm()&0077 != 0 {
					t.Fatal("private installer inputs have unsafe permissions", statErr)
				}
			}
			manifest, err := os.ReadFile(filepath.Join(prepared.Auth, "fixtures.yml"))
			if err != nil || strings.Contains(string(manifest), "fixture-read-secret") || strings.Contains(string(manifest), "forge_authorization") {
				t.Fatal("temporary fixture manifest contains credentials", err)
			}
			data, err := templatekit.YAML(manifest)
			if err != nil {
				t.Fatal(err)
			}
			values := data["fixtures"].(map[string]any)
			modules := values["forge_modules"].(map[string]any)
			if len(modules) != 3 || data["notes"] != "keep" || values["repositories"].(map[string]any)["another"] != "https://git.example.test/team/another.git" || values["symlinks"].(map[string]any)["local"] != "/module/local" {
				t.Fatal("unrelated fixture configuration changed", data)
			}
			directory := containerDirectory
			if native {
				directory = prepared.Auth
			}
			if prepared.Manifest != filepath.Join(directory, "fixtures.yml") {
				t.Fatal("incorrect executor path", prepared.Manifest)
			}
			for _, name := range []string{"common", "stdlib", "apt"} {
				entry := modules[name].(map[string]any)
				if !strings.Contains(strings.ReplaceAll(formatFlags(entry["flags"]), "'", ""), "--config "+filepath.Join(directory, "puppet.conf")) {
					t.Fatal("a Forge module bypassed the shared config", name, entry)
				}
			}
			if modules["common"].(map[string]any)["ref"] != "1.0.0" || !strings.Contains(formatFlags(modules["common"].(map[string]any)["flags"]), "--verbose") || !strings.Contains(formatFlags(modules["apt"].(map[string]any)["flags"]), "--ignore-requirements") {
				t.Fatal("module versions or ordinary flags lost")
			}
			original, err := os.ReadFile(filepath.Join(root, ".fixtures.yml"))
			if err != nil || string(original) != fixtures {
				t.Fatal("tracked manifest was changed", err)
			}
			if err := prepared.RemoveCredentials(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(prepared.Auth); !os.IsNotExist(err) {
				t.Fatal("secret config survived preparation", err)
			}
			if _, err := os.Stat(prepared.Modules); err != nil {
				t.Fatal("shared Docker fixtures removed before specs", err)
			}
			if err := prepared.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(prepared.Directory); !os.IsNotExist(err) {
				t.Fatal("temporary installer data survived cleanup", err)
			}
		})
	}
}

func formatFlags(value any) string {
	if flags, ok := value.(string); ok {
		return flags
	}
	var parts []string
	for _, flag := range value.([]any) {
		parts = append(parts, flag.(string))
	}
	return strings.Join(parts, " ")
}

func TestPublicFixturesAndNoForgeModulesNeedNoCredentials(t *testing.T) {
	for _, body := range []string{"", "fixtures: {}\n", "fixtures:\n  repositories: {sample: https://git.example.test/sample.git}\n", fixtures} {
		root := t.TempDir()
		if body != "" {
			write(t, filepath.Join(root, ".fixtures.yml"), body)
		}
		if err := Validate(root, environment(nil)); err != nil {
			t.Fatal(err)
		}
		if prepared, err := Prepare(root, Options{Getenv: environment(nil)}); err != nil || prepared != nil {
			t.Fatal("ordinary public fixture behavior changed", prepared, err)
		}
	}
}

func TestProxyPreflightReportsOnlyRequiredNames(t *testing.T) {
	for _, row := range []struct {
		env     map[string]string
		missing string
	}{
		{map[string]string{"FORGE_URL": "https://forge.example.test"}, "FORGE_TOKEN"},
		{map[string]string{"FORGE_TOKEN": "fixture-read-secret"}, "FORGE_URL"},
	} {
		root := t.TempDir()
		write(t, filepath.Join(root, ".fixtures.yml"), fixtures)
		err := Validate(root, environment(row.env))
		if err == nil || !strings.Contains(err.Error(), row.missing) || strings.Contains(err.Error(), "fixture-read-secret") {
			t.Fatal("unsafe or missing diagnostic", err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 {
			t.Fatal("preflight wrote installer state", err)
		}
	}
}

func TestProxyURLAndTokenInjectionAreRejected(t *testing.T) {
	for _, url := range []string{"http://forge.test", "https://u:p@forge.test", "https://forge.test?key=x", "https://forge.test?", "https://forge.test/#x", "https://forge.test/$vardir", "https://forge.test/\nforge_authorization=x"} {
		_, _, err := forgeSettings(environment(map[string]string{"FORGE_URL": url, "FORGE_TOKEN": "secret"}))
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("unsafe URL accepted or credential disclosed", err)
		}
	}
	for _, token := range []string{"secret\nserver=bad", "secret\r", "secret\x00", "Bearer secret", "secret#comment", "secret$setting"} {
		if _, _, err := forgeSettings(environment(map[string]string{"FORGE_URL": "https://forge.test", "FORGE_TOKEN": token})); err == nil || strings.Contains(err.Error(), token) {
			t.Fatal("config injection accepted or disclosed", err)
		}
	}
}

func TestManifestGuardsAndConflictingProxyFlags(t *testing.T) {
	for _, body := range []string{"fixtures: []\n", "fixtures: {forge_modules: []}\n", "fixtures: {forge_modules: {common: false}}\n", "fixtures: {forge_modules: {common: {repo: team/common, flags: false}}}\n", "fixtures: {forge_modules: {common: {repo: https://elsewhere.test/module}}}\n", "fixtures: {}\nfixtures: {}\n"} {
		root := t.TempDir()
		write(t, filepath.Join(root, ".fixtures.yml"), body)
		if err := Validate(root, authenticated()); err == nil {
			t.Fatal("invalid manifest accepted", body)
		}
	}
	for _, flags := range []any{"--config /etc/puppet.conf", "--module_repository=https://elsewhere.test", "'--forge_authorization' secret", []any{"--http_debug"}, "--value ${FORGE_TOKEN}", []any{false}} {
		if _, err := configFlags(flags, "/private/puppet.conf"); err == nil {
			t.Fatal("conflicting or credential-bearing flags accepted", flags)
		}
	}
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(t.TempDir(), "manifest"), filepath.Join(root, ".fixtures.yml")); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, authenticated()); err == nil {
		t.Fatal("symlink manifest accepted")
	}
}

func TestAlternativeManifestAndNativePathGuards(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".fixtures.yaml"), fixtures)
	data, modules, err := fixtureManifest(root)
	if err != nil || len(modules) != 3 || data["notes"] != "keep" {
		t.Fatal(data, err)
	}
	if err := os.Mkdir(filepath.Join(root, "spec"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "spec", "fixtures")); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, ".fixtures.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if prepared, err := Prepare(root, Options{Native: true, Getenv: authenticated()}); err == nil || prepared != nil {
		t.Fatal("native fixture path escaped checkout", prepared, err)
	}
	after, err := os.ReadFile(filepath.Join(root, ".fixtures.yaml"))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed preparation rewrote manifest", err)
	}
}

func TestRetiredFixtureSchemaFailsInsteadOfSilentlyDroppingDependencies(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sync.yml"), "test_fixtures: {common: {module: team-common}}\n")
	write(t, filepath.Join(root, ".fixtures.yml"), "fixtures: {}\n")
	if err := Validate(root, environment(nil)); err == nil || !strings.Contains(err.Error(), ".fixtures.yml forge_modules") {
		t.Fatal("removed dependency declaration was silently ignored", err)
	}
	if p, err := Prepare(root, Options{Getenv: authenticated()}); err == nil || p != nil {
		t.Fatal("retired declaration reached fixture execution", p, err)
	}
}

func TestTemporaryCredentialsCannotBeStagedInCheckout(t *testing.T) {
	for _, alias := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "symlink"}[alias], func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, ".fixtures.yml"), fixtures)
			temporary := root
			if alias {
				temporary = filepath.Join(t.TempDir(), "checkout")
				if err := os.Symlink(root, temporary); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("TMPDIR", temporary)
			if prepared, err := Prepare(root, Options{Getenv: authenticated()}); err == nil || prepared != nil || !strings.Contains(err.Error(), "external TMPDIR") {
				t.Fatal("secret staging inside checkout was permitted", prepared, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 || entries[0].Name() != ".fixtures.yml" {
				t.Fatal("failed preparation left credentials in the checkout", entries, err)
			}
		})
	}
}

func TestResolutionRequiresBooleanAndForgeModules(t *testing.T) {
	for _, manifest := range []string{
		"fixtures: {resolve_dependencies: 'true', forge_modules: {common: team/common}}",
		"fixtures: {resolve_dependencies: true}",
	} {
		root := t.TempDir()
		write(t, filepath.Join(root, ".fixtures.yml"), manifest)
		if err := Validate(root, environment(nil)); err == nil || !strings.Contains(err.Error(), "resolve_dependencies") {
			t.Fatalf("invalid resolver config passed preflight: %v", err)
		}
	}
}

func TestNativeResolutionPreservesExistingFixtureDirectory(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".fixtures.yml"), "fixtures: {resolve_dependencies: true, forge_modules: {common: team/common}}")
	fixtures := filepath.Join(root, "spec", "fixtures", "modules")
	if err := os.MkdirAll(fixtures, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(fixtures, "existing")
	write(t, sentinel, "keep")
	if prepared, err := Prepare(root, Options{Native: true}); err == nil || prepared != nil {
		t.Fatalf("resolver took over a populated fixture tree: %v", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep" {
		t.Fatalf("resolver modified existing fixtures: %v", err)
	}
	if err := os.Remove(sentinel); err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(root, Options{Native: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := prepared.Close(); err != nil {
			t.Error(err)
		}
	})
	if target, err := os.Readlink(fixtures); err != nil || target != prepared.Modules {
		t.Fatalf("expected isolated fixture link, got %q: %v", target, err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(fixtures)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatalf("expected original empty directory: %v, %v", info, err)
	}
}

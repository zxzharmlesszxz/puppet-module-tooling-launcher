package pmtool

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const imageID = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeDriver struct {
	calls       [][]string
	plans       []plan
	queries     []plan
	status      int
	missing     bool
	origin      string
	origins     map[string]string
	results     []int
	rendered    []byte
	renderError error
}

func (d *fakeDriver) capture(name string, args []string) ([]byte, error) {
	d.calls = append(d.calls, append([]string{name}, args...))
	if name == "git" {
		if len(args) >= 3 && args[len(args)-1] == "origin" {
			if origin, found := d.origins[args[1]]; found {
				return []byte(origin), nil
			}
			if d.origin != "" {
				return []byte(d.origin), nil
			}
			return []byte("git@git.example.test:team/puppet-sample.git"), nil
		}
		return []byte("Host Author\n"), nil
	}
	if len(args) > 0 && args[0] == "image" {
		if d.missing {
			d.missing = false
			return nil, errors.New("image missing")
		}
		return []byte(imageID + "\n"), nil
	}
	if len(args) > 0 && args[0] == "pull" {
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected capture: %s", name)
}
func (d *fakeDriver) capturePlan(p plan) ([]byte, error) {
	d.queries = append(d.queries, p)
	return d.rendered, d.renderError
}
func (d *fakeDriver) execute(p plan) int {
	d.plans = append(d.plans, p)
	if len(d.results) > 0 {
		code := d.results[0]
		d.results = d.results[1:]
		return code
	}
	return d.status
}

type testExit struct{ code int }

func (e testExit) Error() string { return fmt.Sprintf("command failed with exit %d", e.code) }
func (e testExit) ExitCode() int { return e.code }
func testApp(t *testing.T) (app, *fakeDriver, *bytes.Buffer) {
	t.Helper()
	d := &fakeDriver{}
	output := &bytes.Buffer{}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := app{driver: d, home: dir, cwd: dir, getenv: func(string) string { return "" }, stdout: output, stderr: output}
	a.templateOperation = func(o operationOptions, kind string, _ bool) error {
		code := d.execute(plan{Docker: "pmtool", Args: []string{kind, o.Module}})
		if code != 0 {
			return testExit{code}
		}
		return nil
	}
	return a, d, output
}
func expectFailure(t *testing.T, message string, action func()) {
	t.Helper()
	defer func() {
		caught := recover()
		f, ok := asFailure(caught)
		if !ok || !strings.Contains(f.Error(), message) {
			t.Fatalf("wanted %q, got %v", message, caught)
		}
	}()
	action()
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestLocalHelpDoesNotNeedDockerOrConfig(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"docs", "launcher"}, {"version"}} {
		a, d, output := testApp(t)
		if a.run(args) != 0 || len(d.calls) != 0 || output.Len() == 0 {
			t.Fatal("local help contacted Docker or failed")
		}
		if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
			t.Fatal("help created configuration")
		}
	}
}

func TestConfigCreationPreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "config.json")
	if !createConfig(path, config{Image: "tooling:first"}) || createConfig(path, config{Image: "tooling:second"}) {
		t.Fatal("config init is not exclusive")
	}
	if loadConfig(path).Image != "tooling:first" {
		t.Fatal("existing config overwritten")
	}
	for target, mode := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
		info, err := os.Stat(target)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("wrong permissions for %s", target)
		}
	}
}

func TestDryRunDoesNotExecuteOrPrintToken(t *testing.T) {
	a, d, output := testApp(t)
	a.getenv = func(key string) string {
		if key == "GITLAB_TOKEN" {
			return "secret-token"
		}
		return ""
	}
	a.run([]string{"--image", "tooling:local", "--dry-run", "pipeline-jobs"})
	if len(d.plans) != 0 || strings.Contains(output.String(), "secret-token") {
		t.Fatal("dry-run executed or disclosed token")
	}
}

func TestWrappedLauncherFailure(t *testing.T) {
	cause := errors.New("output unavailable")
	wrapped := fmt.Errorf("context: %w", failure{cause})
	f, ok := asFailure(wrapped)
	if !ok || !errors.Is(f, cause) {
		t.Fatal("wrapped launcher failure or its cause was lost")
	}
	for _, unexpected := range []any{nil, "panic", cause} {
		if _, ok := asFailure(unexpected); ok {
			t.Fatal("unexpected panic classified as a launcher failure")
		}
	}
}

func TestConsoleWriteFailures(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"help"}, {"ai", "help"}, {"config", "path"}} {
		a, _, _ := testApp(t)
		a.stdout = failingWriter{}
		expectFailure(t, "output unavailable", func() { a.run(args) })
	}
	a, _, _ := testApp(t)
	a.stderr = failingWriter{}
	expectFailure(t, "output unavailable", func() { a.notice("OK", "done", "1;32") })
}

func TestPMToolIdentityAndExistingConfigPath(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"help"}, {"ai", "help"}} {
		a, driver, output := testApp(t)
		if a.run(args) != 0 || !strings.Contains(output.String(), "pmtool") {
			t.Fatalf("pmtool identity missing from %v: %s", args, output)
		}
		if len(driver.calls) != 0 {
			t.Fatal("local identity/help must not contact Docker")
		}
		if strings.Contains(output.String(), "puppet-module-tooling audit") || strings.Contains(output.String(), "puppet-module-tooling ai export") {
			t.Fatal("help still advertises the old executable")
		}
	}
	a, _, output := testApp(t)
	if a.run([]string{"config", "path"}) != 0 || strings.TrimSpace(output.String()) != filepath.Join(a.home, ".config", "puppet-module-tooling", "config.json") {
		t.Fatal("binary rename must preserve the configured settings location")
	}
}

func TestConfigInitUsesDefaultImageAndAcceptsOverride(t *testing.T) {
	for _, custom := range []string{"", "registry.example/custom:v2"} {
		a, driver, _ := testApp(t)
		args := []string{"config", "init"}
		expected := defaultImage
		if custom != "" {
			args = append(args, "--image", custom)
			expected = custom
		}
		if a.run(args) != 0 {
			t.Fatal("config init failed")
		}
		path := filepath.Join(a.home, ".config", "puppet-module-tooling", "config.json")
		if loadConfig(path).Image != expected || len(driver.calls) != 0 {
			t.Fatal("config init lost the selected image or contacted Docker")
		}
		a.run([]string{"config", "init", "--image", "registry.example/do-not-overwrite:v3"})
		if loadConfig(path).Image != expected {
			t.Fatal("config init overwrote an existing image choice")
		}
	}
}

func TestConfigInitDefaultsToPinnedUpstreamVoxBox(t *testing.T) {
	a, driver, _ := testApp(t)
	if a.run([]string{"config", "init"}) != 0 {
		t.Fatal("config init failed")
	}
	path := filepath.Join(a.home, ".config", "puppet-module-tooling", "config.json")
	if loadConfig(path).Image != voxboxImage || len(driver.calls) != 0 {
		t.Fatal("config init did not select pinned upstream VoxBox locally")
	}
}

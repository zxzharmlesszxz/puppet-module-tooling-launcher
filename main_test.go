package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const imageID = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeDriver struct {
	calls     [][]string
	plans     []plan
	status    int
	missing   bool
	protocol  int
	document       string
	docsError      error
	origin         string
	origins        map[string]string
	moduleSettings map[string]workflowSettings
	settingsJSON   string
}

func capabilities() manifest {
	return manifest{Protocol: 1, Options: []string{"module", "host-module", "version", "ref", "ci", "provider", "template"}, Commands: map[string]profile{
		"audit":          {Module: true, ReadOnly: true},
		"check":          {Module: true, Socket: true},
		"pipeline-jobs":  {Module: true, ReadOnly: true, Token: true},
		"release-commit": {Module: true, Author: true},
		"release-submit": {Module: true, Token: true, Author: true},
		"settings":       {Module: true, ReadOnly: true},
		"help":           {}, "docs": {},
	}}
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
	if args[0] == "image" {
		if d.missing {
			d.missing = false
			return nil, errors.New("image missing")
		}
		return []byte(imageID + "\n"), nil
	}
	if args[0] == "pull" {
		return nil, nil
	}
	if len(args) >= 2 && args[len(args)-2] == "docs" && args[len(args)-1] == "ai" {
		return []byte(d.document), d.docsError
	}
	for i, arg := range args {
		if arg != imageID || i+1 >= len(args) || args[i+1] != "settings" {
			continue
		}
		if d.settingsJSON != "" {
			return []byte(d.settingsJSON), nil
		}
		values := map[string]string{}
		for index := i+2; index+1 < len(args); index += 2 {
			values[args[index]] = args[index+1]
		}
		module := values["--module"]
		selected := d.moduleSettings[module]
		if values["--ci"] != "" {
			selected.CI = values["--ci"]
		}
		if values["--provider"] != "" {
			selected.Provider = values["--provider"]
		}
		if values["--template"] != "" {
			selected.Template = values["--template"]
		}
		if selected.Provider == "" {
			origin := d.origin
			if value, ok := d.origins[module]; ok {
				origin = value
			}
			selected.Provider = "gitlab"
			if strings.Contains(origin, "github.com") {
				selected.Provider = "github"
			}
		}
		return json.Marshal(selected)
	}
	m := capabilities()
	if d.protocol != 0 {
		m.Protocol = d.protocol
	}
	return json.Marshal(m)
}

func (d *fakeDriver) execute(p plan) int {
	d.plans = append(d.plans, p)
	return d.status
}

func testApp(t *testing.T) (app, *fakeDriver, *bytes.Buffer) {
	t.Helper()
	d := &fakeDriver{document: "# Fixture workflow\n\nUse audit.\n"}
	output := &bytes.Buffer{}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := app{driver: d, home: dir, cwd: dir, getenv: func(string) string { return "" }, stdout: output, stderr: output}
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

func TestRuntimeUsesSameImageIDAndPropagatesExitCode(t *testing.T) {
	a, d, _ := testApp(t)
	d.status = 17
	if a.run([]string{"--image", "tooling:local", "audit"}) != 17 {
		t.Fatal("exit status lost")
	}
	query := d.calls[1]
	if query[len(query)-2] != imageID || query[len(query)-1] != "manifest" || !strings.Contains(strings.Join(query, " "), "--network none") {
		t.Fatal("manifest discovery must have no module/network/socket")
	}
	p := d.plans[0]
	if !strings.Contains(strings.Join(p.Args, " "), imageID+" audit") {
		t.Fatal("execution did not pin the queried image")
	}
	if strings.Contains(strings.Join(p.Args, " "), "docker.sock") {
		t.Fatal("audit received a socket")
	}
}

func TestPermissionsComeFromManifest(t *testing.T) {
	a, _, _ := testApp(t)
	a.getenv = func(key string) string {
		if key == "GITLAB_TOKEN" {
			return "secret-never-in-argv"
		}
		return ""
	}
	cfg := config{Docker: "docker", Socket: "/custom/docker.sock"}
	for _, command := range []string{"audit", "check", "pipeline-jobs", "release-commit", "release-submit"} {
		p := a.makePlan(cfg, imageID, capabilities(), []string{command})
		argv := strings.Join(p.Args, " ")
		if strings.Contains(argv, "dst=/var/run/docker.sock") != (command == "check") {
			t.Fatal("socket mismatch:", command)
		}
		if strings.Contains(argv, "--env GITLAB_TOKEN") != (command == "pipeline-jobs" || command == "release-submit") || strings.Contains(argv, "secret-never") {
			t.Fatal("token leak or wrong token scope")
		}
		if (p.Environment["GIT_AUTHOR_NAME"] != "") != (command == "release-commit" || command == "release-submit") {
			t.Fatal("author scope mismatch")
		}
	}
}

func TestPhysicalPathAndMountEscaping(t *testing.T) {
	a, _, _ := testApp(t)
	module := filepath.Join(a.cwd, "clone, with space")
	if err := os.Mkdir(module, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(a.cwd, "link")
	if err := os.Symlink(module, link); err != nil {
		t.Fatal(err)
	}
	p := a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"audit", "--module", link})
	index := 0
	for i, arg := range p.Args {
		if arg == "--mount" {
			index = i + 1
			break
		}
	}
	fields, err := csv.NewReader(strings.NewReader(p.Args[index])).Read()
	if err != nil || !reflect.DeepEqual(fields, []string{"type=bind", "src=" + module, "dst=" + module, "readonly"}) {
		t.Fatalf("unsafe mount encoding: %v %v", fields, err)
	}
}

func TestCombinedMRWritableMountWithoutSocketOrAuthor(t *testing.T) {
	a, _, _ := testApp(t)
	a.getenv = func(key string) string {
		if key == "GITLAB_TOKEN" {
			return "secret-never-in-argv"
		}
		return ""
	}
	m := capabilities()
	m.Commands["mr"] = profile{Module: true, Token: true}
	p := a.makePlan(config{Docker: "docker", Socket: "/custom/docker.sock"}, imageID, m, []string{"mr"})
	argv := strings.Join(p.Args, " ")
	if !strings.Contains(argv, "--env GITLAB_TOKEN") || strings.Contains(argv, "secret-never") || strings.Contains(argv, "docker.sock") {
		t.Fatal("combined MR token/socket scope mismatch")
	}
	if p.Environment["GIT_AUTHOR_NAME"] != "" || p.Environment["GIT_AUTHOR_EMAIL"] != "" {
		t.Fatal("combined MR must not receive a commit author")
	}
	found := false
	for i, arg := range p.Args {
		if arg != "--mount" {
			continue
		}
		fields, err := csv.NewReader(strings.NewReader(p.Args[i+1])).Read()
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range fields {
			if field == "readonly" {
				t.Fatal("combined MR clone mount must allow main-sync")
			}
		}
		found = true
	}
	if !found {
		t.Fatal("combined MR clone mount missing")
	}
}

func TestRuntimeGuards(t *testing.T) {
	a, _, _ := testApp(t)
	cfg := config{Docker: "docker"}
	for _, test := range []struct {
		args    []string
		message string
	}{
		{[]string{"unknown"}, "not supported"},
		{[]string{"audit", "--host-module", "/wrong"}, "managed by the launcher"},
		{[]string{"audit", "--module", "/"}, "other than /"},
		{[]string{"audit", "--unknown", "x"}, "unsupported runtime option"},
		{[]string{"audit", "--module"}, "missing value"},
		{[]string{"pipeline-jobs"}, "GITLAB_TOKEN is required"},
	} {
		t.Run(test.message, func(t *testing.T) {
			expectFailure(t, test.message, func() { a.makePlan(cfg, imageID, capabilities(), test.args) })
		})
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

func TestPullAndProtocolCompatibility(t *testing.T) {
	a, d, _ := testApp(t)
	d.missing = true
	a.run([]string{"--image", "tooling:local", "audit"})
	if d.calls[1][1] != "pull" || len(d.plans) != 1 {
		t.Fatal("missing image was not pulled")
	}
	a, d, _ = testApp(t)
	d.protocol = 2
	expectFailure(t, "unsupported runtime manifest protocol", func() { a.run([]string{"--image", "tooling:local", "audit"}) })
	if len(d.plans) != 0 {
		t.Fatal("incompatible image executed")
	}
}

func TestNewRuntimeCommandRequiresNoLauncherCatalog(t *testing.T) {
	a, _, _ := testApp(t)
	m := capabilities()
	m.Commands["future-command"] = profile{Module: true, ReadOnly: true}
	p := a.makePlan(config{Docker: "docker"}, imageID, m, []string{"future-command"})
	if !strings.Contains(strings.Join(p.Args, " "), imageID+" future-command") {
		t.Fatal("launcher duplicated the runtime command catalog")
	}
}

func TestRuntimeDocsDoNotMountClone(t *testing.T) {
	a, _, _ := testApp(t)
	p := a.makePlan(config{Docker: "docker"}, imageID, capabilities(), []string{"docs", "onboarding"})
	if strings.Contains(strings.Join(p.Args, " "), "--mount") {
		t.Fatal("docs received a clone mount")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

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

func TestProviderTokenIsolation(t *testing.T) {
	for _, origin := range []string{"git@github.com:team_b/puppet-sample.git", "https://github.com/team_a/puppet-sample.git", "ssh://git@github.com/team_b/puppet-sample.git"} {
		a, d, _ := testApp(t)
		d.origin = origin
		a.getenv = func(key string) string {
			if key == "GITHUB_TOKEN" || key == "GITLAB_TOKEN" {
				return "secret-not-in-plan"
			}
			return ""
		}
		m := capabilities()
		m.Options = append(m.Options, "ci", "provider")
		p := a.makePlan(config{Docker: "docker"}, imageID, m, []string{"pipeline-jobs", "--ci", "github.com/team_b/pipelines/.github/workflows/puppet.yml@v1"})
		argv := strings.Join(p.Args, " ")
		if !strings.Contains(argv, "--env GITHUB_TOKEN") || strings.Contains(argv, "GITLAB_TOKEN") || strings.Contains(argv, "secret-not") || strings.Contains(argv, "docker.sock") {
			t.Fatal("provider token escaped its boundary")
		}
		if p.Environment["TOOLING_PROVIDER"] != "github" {
			t.Fatal("resolved provider was not forwarded")
		}
	}
}

func TestExplicitEnterpriseProvider(t *testing.T) {
	a, _, _ := testApp(t)
	m := capabilities()
	m.Options = append(m.Options, "provider")
	a.getenv = func(key string) string {
		if key == "GITHUB_TOKEN" {
			return "token"
		}
		return ""
	}
	p := a.makePlan(config{Docker: "docker"}, imageID, m, []string{"pipeline-jobs", "--provider", "github"})
	if p.Environment["TOOLING_PROVIDER"] != "github" {
		t.Fatal("enterprise provider ignored")
	}
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

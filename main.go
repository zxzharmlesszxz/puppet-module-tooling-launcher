package main

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

var version = "dev"
var defaultImage string

//go:embed manual.md
var manual string

type failure struct{ error }

func (f failure) Unwrap() error { return f.error }

func asFailure(caught any) (failure, bool) {
	err, ok := caught.(error)
	var expected failure
	matched := ok && errors.As(err, &expected)
	return expected, matched
}

func check(err error) {
	if err != nil {
		panic(failure{err})
	}
}

func must[T any](value T, err error) T {
	check(err)
	return value
}

func fail(format string, args ...any) { panic(failure{fmt.Errorf(format, args...)}) }

type config struct {
	Image  string `json:"image"`
	Docker string `json:"docker,omitempty"`
	Socket string `json:"socket,omitempty"`
}

type profile struct {
	Module   bool `json:"module"`
	ReadOnly bool `json:"read_only"`
	Socket   bool `json:"socket"`
	Token    bool `json:"token"`
	Author   bool `json:"author"`
}

type manifest struct {
	Protocol int                `json:"protocol"`
	Options  []string           `json:"options"`
	Commands map[string]profile `json:"commands"`
}

type plan struct {
	Docker      string            `json:"docker"`
	Args        []string          `json:"args"`
	Environment map[string]string `json:"environment,omitempty"`
}

type driver interface {
	capture(string, []string) ([]byte, error)
	execute(plan) int
}

type realDriver struct{}

func (realDriver) capture(name string, args []string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", name, strings.TrimSpace(stderr.String()), err)
	}
	return output, nil
}

func (realDriver) execute(p plan) int {
	cmd := exec.Command(p.Docker, p.Args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	for key, value := range p.Environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	check(cmd.Start())
	signals := make(chan os.Signal, 2)
	done := make(chan struct{})
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		for {
			select {
			case sig := <-signals:
				_ = cmd.Process.Signal(sig)
			case <-done:
				return
			}
		}
	}()
	err := cmd.Wait()
	signal.Stop(signals)
	close(done)
	if err == nil {
		return 0
	}
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		if status, ok := exited.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exited.ExitCode()
	}
	check(err)
	return 1
}

type app struct {
	driver  driver
	home    string
	cwd     string
	getenv  func(string) string
	stdout  io.Writer
	stderr  io.Writer
	tty     bool
	color   string
	columns int
}

type options struct {
	config, image, docker, socket, color string
	dryRun, help, version                bool
}

func decode(data []byte, target any) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	check(decoder.Decode(target))
	if err := decoder.Decode(new(any)); err != io.EOF {
		fail("expected one JSON document")
	}
}

func initialConfig() config { return config{Image: defaultImage} }

func createConfig(path string, cfg config) bool {
	check(os.MkdirAll(filepath.Dir(path), 0700))
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return false
	}
	check(err)
	data := append(must(json.MarshalIndent(cfg, "", "  ")), '\n')
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	check(writeErr)
	check(closeErr)
	return true
}

func loadConfig(path string) config {
	createConfig(path, initialConfig())
	var cfg config
	decode(must(os.ReadFile(path)), &cfg)
	return cfg
}

func (a *app) run(args []string) int {
	var opts options
	flags := flag.NewFlagSet("pmtool", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.config, "config", filepath.Join(a.home, ".config", "puppet-module-tooling", "config.json"), "launcher config")
	flags.StringVar(&opts.image, "image", "", "image for this invocation")
	flags.StringVar(&opts.docker, "docker", "", "Docker executable")
	flags.StringVar(&opts.socket, "socket", "", "socket bind source for commands requiring Docker")
	flags.StringVar(&opts.color, "color", "auto", "auto, always or never")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "show the Docker plan without running the module command")
	flags.BoolVar(&opts.help, "help", false, "launcher help")
	flags.BoolVar(&opts.help, "h", false, "launcher help")
	flags.BoolVar(&opts.version, "version", false, "launcher version")
	check(flags.Parse(args))
	args = flags.Args()
	if opts.color != "auto" && opts.color != "always" && opts.color != "never" {
		fail("color must be auto, always or never")
	}
	a.color = opts.color
	if opts.color == "auto" && a.getenv("TOOLING_COLOR") != "" {
		a.color = a.getenv("TOOLING_COLOR")
	}
	if a.color != "auto" && a.color != "always" && a.color != "never" {
		fail("TOOLING_COLOR must be auto, always or never")
	}
	if opts.version || (len(args) == 1 && args[0] == "version") {
		must(fmt.Fprintf(a.stdout, "pmtool %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH))
		return 0
	}
	if opts.help || len(args) == 0 || (len(args) == 1 && args[0] == "help") || (len(args) >= 2 && args[0] == "docs" && args[1] == "launcher") {
		section := ""
		if len(args) >= 2 && args[0] == "docs" {
			docFlags := flag.NewFlagSet("docs launcher", flag.ContinueOnError)
			docFlags.SetOutput(io.Discard)
			docFlags.StringVar(&section, "section", "", "section title or list")
			check(docFlags.Parse(args[2:]))
			if docFlags.NArg() != 0 {
				fail("unexpected documentation arguments")
			}
		}
		must(fmt.Fprint(a.stdout, a.document(manual, section)))
		return 0
	}
	var ai *aiOptions
	aiOutput := "-"
	if args[0] == "ai" {
		export := parseAI(args[1:])
		if export.help {
			must(fmt.Fprint(a.stdout, aiHelp))
			return 0
		}
		aiOutput = a.exportPath(export)
		ai = &export
	}
	if !filepath.IsAbs(opts.config) {
		opts.config = filepath.Join(a.cwd, opts.config)
	}
	if args[0] == "config" {
		return a.configure(opts, args[1:])
	}
	if len(args) == 2 && args[0] == "help" && args[1] == "runtime" {
		args = []string{"help"}
	}
	cfg := loadConfig(opts.config)
	if opts.image != "" {
		cfg.Image = opts.image
	}
	if opts.docker != "" {
		cfg.Docker = opts.docker
	}
	if opts.socket != "" {
		cfg.Socket = opts.socket
	}
	if cfg.Docker == "" {
		cfg.Docker = "docker"
	}
	if cfg.Socket == "" {
		cfg.Socket = "/var/run/docker.sock"
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]*$`).MatchString(cfg.Image) {
		fail("set a valid image in %s or use --image before the command", opts.config)
	}
	image := a.resolveImage(cfg)
	data := must(a.driver.capture(cfg.Docker, []string{"run", "--rm", "--read-only", "--tmpfs", "/tmp:exec", "--network", "none", image, "manifest"}))
	var capabilities manifest
	decode(data, &capabilities)
	if capabilities.Protocol != 1 {
		fail("unsupported runtime manifest protocol %d; update the launcher/image pair", capabilities.Protocol)
	}
	if ai != nil {
		return a.exportAI(cfg, image, capabilities, *ai, aiOutput, opts.dryRun)
	}
	p := a.makePlan(cfg, image, capabilities, args)
	if opts.dryRun {
		must(fmt.Fprintln(a.stdout, string(must(json.MarshalIndent(p, "", "  ")))))
		return 0
	}
	code := a.driver.execute(p)
	if code != 0 {
		a.notice("FAIL", fmt.Sprintf("%s: container exited with code %d", args[0], code), "1;31")
	}
	return code
}

func (a *app) configure(opts options, args []string) int {
	if len(args) == 0 {
		fail("use config init, config path or config show")
	}
	switch args[0] {
	case "path":
		if len(args) != 1 {
			fail("config path takes no arguments")
		}
		must(fmt.Fprintln(a.stdout, opts.config))
	case "init":
		flags := flag.NewFlagSet("config init", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		image := opts.image
		if image == "" {
			image = defaultImage
		}
		flags.StringVar(&image, "image", image, "initial image")
		check(flags.Parse(args[1:]))
		if flags.NArg() != 0 {
			fail("unexpected config init arguments")
		}
		created := createConfig(opts.config, config{Image: image})
		must(fmt.Fprintf(a.stdout, "Config %s: %s\n", map[bool]string{true: "created", false: "already exists; preserved"}[created], opts.config))
	case "show":
		if len(args) != 1 {
			fail("config show takes no arguments")
		}
		must(fmt.Fprintln(a.stdout, string(must(json.MarshalIndent(loadConfig(opts.config), "", "  ")))))
	default:
		fail("unknown config command: %s", args[0])
	}
	return 0
}

func (a *app) resolveImage(cfg config) string {
	query := []string{"image", "inspect", "--format", "{{.Id}}", cfg.Image}
	data, err := a.driver.capture(cfg.Docker, query)
	if err != nil {
		must(fmt.Fprintln(a.stderr, "Pulling tooling image:", cfg.Image))
		_, pullErr := a.driver.capture(cfg.Docker, []string{"pull", cfg.Image})
		check(pullErr)
		data = must(a.driver.capture(cfg.Docker, query))
	}
	id := strings.TrimSpace(string(data))
	if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(id) {
		fail("Docker returned an invalid image ID")
	}
	return id
}

func mount(source, destination string, readOnly bool) string {
	fields := []string{"type=bind", "src=" + source, "dst=" + destination}
	if readOnly {
		fields = append(fields, "readonly")
	}
	var output strings.Builder
	writer := csv.NewWriter(&output)
	check(writer.Write(fields))
	writer.Flush()
	check(writer.Error())
	return strings.TrimSuffix(output.String(), "\n")
}

func (a *app) makePlan(cfg config, image string, capabilities manifest, args []string) plan {
	command := args[0]
	permission, ok := capabilities.Commands[command]
	if !ok {
		fail("command %q is not supported by this image; use help runtime", command)
	}
	allowed := make(map[string]bool)
	for _, name := range capabilities.Options {
		allowed["--"+name] = true
	}
	forwarded := []string{command}
	module := a.cwd
	var cli workflowSettings
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--help" || arg == "-h" {
			return a.makePlan(cfg, image, capabilities, []string{"help"})
		}
		if !strings.HasPrefix(arg, "--") {
			if command != "docs" {
				fail("unexpected argument: %s", arg)
			}
			forwarded = append(forwarded, arg)
			continue
		}
		key, value, equal := strings.Cut(arg, "=")
		if !allowed[key] {
			fail("unsupported runtime option: %s", key)
		}
		if !equal {
			i++
			if i == len(args) {
				fail("missing value for %s", key)
			}
			value = args[i]
		}
		if key == "--host-module" {
			fail("--host-module is managed by the launcher; use --module")
		}
		if key == "--module" {
			module = value
			continue
		}
		if key == "--ci" || key == "--provider" || key == "--template" {
			if value == "" {
				fail("%s must not be empty", key)
			}
			if key == "--ci" {
				cli.CI = value
			} else if key == "--provider" {
				cli.Provider = value
			} else {
				cli.Template = value
			}
			continue
		}
		forwarded = append(forwarded, key, value)
	}
	p := plan{Docker: cfg.Docker, Args: []string{"run", "--rm", "--read-only", "--tmpfs", "/tmp:exec"}, Environment: map[string]string{}}
	if a.tty && isTerminal(os.Stdin) {
		p.Args = append(p.Args, "--interactive", "--tty")
	}
	p.Environment["TOOLING_COLOR"] = "never"
	if a.colorEnabled() {
		p.Environment["TOOLING_COLOR"] = "always"
	}
	p.Args = append(p.Args, "--env", "TOOLING_COLOR")
	if a.columns > 0 {
		p.Environment["COLUMNS"] = fmt.Sprint(a.columns)
		p.Args = append(p.Args, "--env", "COLUMNS")
	} else if a.getenv("COLUMNS") != "" {
		p.Args = append(p.Args, "--env", "COLUMNS")
	}
	if permission.Module {
		if !filepath.IsAbs(module) {
			module = filepath.Join(a.cwd, module)
		}
		module = must(filepath.EvalSymlinks(module))
		info := must(os.Stat(module))
		if !info.IsDir() || module == string(filepath.Separator) {
			fail("module must be a directory other than /")
		}
		p.Args = append(p.Args, "--mount", mount(module, module, permission.ReadOnly), "--workdir", module)
		forwarded = append(forwarded, "--module", module)
	}
	if permission.Socket {
		if !filepath.IsAbs(cfg.Socket) {
			fail("Docker socket bind source must be absolute")
		}
		p.Args = append(p.Args, "--mount", mount(cfg.Socket, "/var/run/docker.sock", false))
	}
	var selected workflowSettings
	if permission.Module {
		selected = a.resolveWorkflowSettings(cfg, image, module, capabilities, cli)
		for _, option := range []struct{ key, value string }{{"--ci", selected.CI}, {"--provider", selected.Provider}, {"--template", selected.Template}} {
			if option.value == "" {
				continue
			}
			if !allowed[option.key] {
				fail("configured %s is not supported by this runtime image", option.key)
			}
			forwarded = append(forwarded, option.key, option.value)
		}
	}
	provider := selected.Provider
	if permission.Token {
		key := "GITLAB_TOKEN"
		optional := []string{"GITLAB_USERNAME", "GITLAB_BASE_URL"}
		if provider == "github" {
			key = "GITHUB_TOKEN"
			optional = []string{"GITHUB_API_URL"}
		}
		if a.getenv(key) == "" {
			fail("%s is required for %s", key, command)
		}
		p.Environment["TOOLING_PROVIDER"] = provider
		p.Args = append(p.Args, "--env", "TOOLING_PROVIDER", "--env", key)
		for _, name := range optional {
			if a.getenv(name) != "" {
				p.Args = append(p.Args, "--env", name)
			}
		}
	} else if provider != "" {
		if provider != "gitlab" && provider != "github" {
			fail("provider must be gitlab or github")
		}
		p.Environment["TOOLING_PROVIDER"] = provider
		p.Args = append(p.Args, "--env", "TOOLING_PROVIDER")
	}
	if permission.Author {
		for _, field := range []struct{ env, git string }{{"GIT_AUTHOR_NAME", "user.name"}, {"GIT_AUTHOR_EMAIL", "user.email"}} {
			value := a.getenv(field.env)
			if value == "" {
				data, err := a.driver.capture("git", []string{"-C", module, "config", "--get", field.git})
				if err == nil {
					value = strings.TrimSpace(string(data))
				}
			}
			if value != "" {
				p.Environment[field.env] = value
				p.Args = append(p.Args, "--env", field.env)
			}
		}
	}
	for _, key := range []string{"NO_COLOR", "TERM", "PDK_IMAGE", "VOXBOX_IMAGE"} {
		if a.getenv(key) != "" {
			p.Args = append(p.Args, "--env", key)
		}
	}
	p.Args = append(p.Args, image)
	p.Args = append(p.Args, forwarded...)
	return p
}

func entry(args []string) (code int) {
	application := app{driver: realDriver{}, getenv: os.Getenv, stdout: os.Stdout, stderr: os.Stderr, tty: isTerminal(os.Stdout), columns: terminalColumns(os.Stdout)}
	defer func() {
		if caught := recover(); caught != nil {
			if expected, ok := asFailure(caught); ok {
				code = 2
				if _, err := fmt.Fprintln(application.stderr, application.paint("[ERROR] "+expected.Error(), "1;31")); err != nil {
					return
				}
			} else {
				panic(caught)
			}
		}
	}()
	application.home, application.cwd = must(os.UserHomeDir()), must(os.Getwd())
	return application.run(args)
}

func main() { os.Exit(entry(os.Args[1:])) }

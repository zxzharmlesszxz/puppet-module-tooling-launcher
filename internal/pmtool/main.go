package pmtool

import (
	"bytes"
	"context"
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

	documentation "github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/docs"
	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/hostflow"
)

var version = "dev"
var defaultImage = voxboxImage

var manual = documentation.Read("manual.md")
var shortHelp = documentation.Read("HELP.md")

type failure struct{ error }

func (f failure) Unwrap() error { return f.error }

func asFailure(caught any) (failure, bool) {
	err, ok := caught.(error)
	if !ok {
		return failure{}, false
	}
	return errors.AsType[failure](err)
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
	TemplateToken bool `json:"template_token"`
	Module        bool `json:"module"`
	ReadOnly      bool `json:"read_only"`
	Socket        bool `json:"socket"`
	Token         bool `json:"token"`
	Author        bool `json:"author"`
}

type manifest struct {
	Protocol int                `json:"protocol"`
	Options  []string           `json:"options"`
	Commands map[string]profile `json:"commands"`
}

type plan struct {
	Context     context.Context   `json:"-"`
	Docker      string            `json:"docker"`
	Args        []string          `json:"args"`
	Environment map[string]string `json:"environment,omitempty"`
	Input       []byte            `json:"-"`
	Directory   string            `json:"directory,omitempty"`
}

type driver interface {
	capture(string, []string) ([]byte, error)
	capturePlan(plan) ([]byte, error)
	execute(plan) int
}

type realDriver struct{}

func (realDriver) capture(name string, args []string) ([]byte, error) {
	cmd := exec.Command(name, args...) // #nosec G204 -- Host-selected Docker/Git executable and separate argv; no shell interpolation.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", name, strings.TrimSpace(stderr.String()), err)
	}
	return output, nil
}

func commandForPlan(p plan) *exec.Cmd {
	ctx := p.Context
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, p.Docker, p.Args...) // #nosec G204 -- Local Docker executable and launcher-owned argv; no shell interpolation.
	cmd.Stdin = os.Stdin
	cmd.Dir = p.Directory
	if len(p.Input) != 0 {
		cmd.Stdin = bytes.NewReader(p.Input)
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := p.Environment[key]; !replaced {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	for key, value := range p.Environment {
		if value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	return cmd
}

func (realDriver) execute(p plan) int {
	cmd := commandForPlan(p)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
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
	if exited, ok := errors.AsType[*exec.ExitError](err); ok {
		if status, ok := exited.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exited.ExitCode()
	}
	check(err)
	return 1
}

type app struct {
	driver            driver
	home              string
	cwd               string
	getenv            func(string) string
	stdout            io.Writer
	stderr            io.Writer
	tty               bool
	color             string
	columns           int
	templateOperation func(operationOptions, string, bool) error
}

type options struct {
	config, image, docker, socket, color, executor string
	dryRun, help, version                          bool
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
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- Local --config intentionally selects this path; exclusive creation preserves existing files.
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
	decode(must(os.ReadFile(path)), &cfg) // #nosec G304 -- Read the local user-selected config file, not a path from the container or module.
	return cfg
}

func (a *app) run(args []string) int {
	var opts options
	flags := flag.NewFlagSet("pmtool", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.config, "config", filepath.Join(a.home, ".config", "puppet-module-tooling", "config.json"), "launcher config")
	flags.StringVar(&opts.image, "image", "", "image for this invocation")
	flags.StringVar(&opts.docker, "docker", "", "Docker executable")
	flags.StringVar(&opts.socket, "socket", "", "legacy compatibility setting; no socket is mounted")
	flags.StringVar(&opts.executor, "executor", "docker", "docker or native (CI tool environment)")
	flags.StringVar(&opts.color, "color", "auto", "auto, always or never")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "preview the selected operation without executing its workflow")
	flags.BoolVar(&opts.help, "help", false, "launcher help")
	flags.BoolVar(&opts.help, "h", false, "launcher help")
	flags.BoolVar(&opts.version, "version", false, "launcher version")
	check(flags.Parse(args))
	args = flags.Args()
	if opts.executor != "docker" && opts.executor != "native" {
		fail("executor must be docker or native")
	}
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
	if opts.help || len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		must(fmt.Fprint(a.stdout, a.usage()))
		return 0
	}
	if args[0] == "docs" {
		return a.localDocument(args[1:])
	}
	if len(args) == 2 && args[0] == "help" && args[1] == "runtime" {
		return a.localDocument([]string{"usage"})
	}
	if args[0] == "manifest" {
		must(fmt.Fprintln(a.stdout, string(must(json.Marshal(launcherManifest())))))
		return 0
	}
	if args[0] == "template-init" {
		return a.initTemplate(args[1:], opts.dryRun)
	}
	if args[0] == "settings" {
		o := a.parseOperation(args)
		selected := a.resolveWorkflowSettings(o.Module, o.Settings)
		must(fmt.Fprintln(a.stdout, string(must(json.MarshalIndent(selected, "", "  ")))))
		return 0
	}
	if args[0] == "completion" {
		return a.printCompletion(args[1:])
	}
	if args[0] == "__complete" {
		return a.complete(opts, args[1:])
	}
	var ai *aiOptions
	var aiOutput string
	if args[0] == "ai" {
		export := parseAI(args[1:])
		if export.help {
			must(fmt.Fprint(a.stdout, aiHelp))
			return 0
		}
		aiOutput = a.exportPath(export)
		ai = &export
		return a.exportAI(launcherManifest(), *ai, aiOutput, opts.dryRun)
	}
	if args[0] == "doctor" {
		return a.doctor(opts, args[1:])
	}
	if args[0] == "forge-publish" {
		if opts.executor == "native" {
			fail("forge-publish is a host operation; omit --executor native")
		}
		return a.publishForge(args, opts.dryRun)
	}
	if !hostflow.IsCommand(args[0]) && launcherManifest().Commands[args[0]].Module && args[0] != "template-test" {
		check(a.preflightWorkflow(args, opts.dryRun))
	}
	if opts.executor == "native" && args[0] != "config" {
		if args[0] == "template-test" {
			return a.runTemplateTest(config{}, "", args[1:], true, opts.dryRun)
		}
		return a.runNative(args, opts.dryRun)
	}
	if hostflow.IsCommand(args[0]) {
		return a.runHost(args, opts.dryRun)
	}
	if !filepath.IsAbs(opts.config) {
		opts.config = filepath.Join(a.cwd, opts.config)
	}
	if args[0] == "config" {
		return a.configure(opts, args[1:])
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
	image := ""
	needsSDK := args[0] == "check" || args[0] == "audit" || args[0] == "audit-local" || args[0] == "sync" || args[0] == "sync-noop" || args[0] == "bootstrap-local" || args[0] == "bootstrap-local-noop" || args[0] == "template-test"
	if needsSDK && !opts.dryRun {
		if !containerImagePattern.MatchString(cfg.Image) {
			fail("set a valid image in %s or use --image before the command", opts.config)
		}
		image = a.resolveImage(cfg)
	}
	if args[0] == "template-test" {
		return a.runTemplateTest(cfg, image, args[1:], false, opts.dryRun)
	}

	return a.runWorkflow(cfg, image, args, opts.dryRun)
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

func Main(args []string) int { return entry(args) }

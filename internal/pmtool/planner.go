package pmtool

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
)

const voxboxImage = "ghcr.io/voxpupuli/voxbox:10.1.0-openvox8@sha256:01bb8ce5e24d5b111934660f75e2786b4d7c7adcaa7518a564856d1d3041d10d"

var containerImagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]*$`)

type operationOptions struct {
	Command, Module, Files, Stage, Name, VoxBox, CIProject string
	Settings                                               workflowSettings
}

func (a *app) parseOperation(args []string) operationOptions {
	o := operationOptions{Command: args[0], Module: a.cwd, VoxBox: voxboxImage}
	aliases := map[string]string{"bootstrap-local": "sync", "bootstrap-local-noop": "sync-noop", "audit-local": "audit"}
	if target := aliases[o.Command]; target != "" {
		o.Command = target
	}
	if v := a.getenv("VOXBOX_IMAGE"); v != "" {
		o.VoxBox = v
	}
	for i := 1; i < len(args); i++ {
		key, value, equal := strings.Cut(args[i], "=")
		if !equal {
			i++
			if i >= len(args) {
				fail("missing value for %s", key)
			}
			value = args[i]
		}
		if key == "--host-module" {
			fail("--host-module is managed by the launcher; use --module")
		}
		if value == "" {
			fail("%s must not be empty", key)
		}
		switch key {
		case "--module":
			o.Module = value
		case "--files":
			o.Files = value
		case "--stage":
			o.Stage = value
		case "--name":
			o.Name = value
		case "--voxbox-image":
			o.VoxBox = value
		case "--ci":
			o.Settings.CI = value
		case "--template":
			o.Settings.Template = value
		case "--provider":
			o.Settings.Provider = value
		case "--ci-project":
			o.CIProject = value
		default:
			fail("unsupported runtime option: %s", key)
		}
	}
	if !filepath.IsAbs(o.Module) {
		o.Module = filepath.Join(a.cwd, o.Module)
	}
	o.Module = must(filepath.EvalSymlinks(o.Module))
	if !must(os.Stat(o.Module)).IsDir() || o.Module == "/" {
		fail("module must be a directory other than /")
	}
	if o.Stage != "" && ((o.Command != "voxbox") || (o.Stage != "validate" && o.Stage != "unit")) {
		fail("--stage validate|unit is supported only for voxbox")
	}
	if o.Command == "module-new" {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(o.Name) {
			fail("module-new requires --name with a lowercase module name")
		}
		if _, err := os.Lstat(filepath.Join(o.Module, o.Name)); !os.IsNotExist(err) {
			fail("module-new destination must not exist")
		}
	} else if o.Name != "" {
		fail("--name is supported only for module-new")
	}
	return o
}
func (a *app) sequence(o operationOptions) workflowSequence {
	host := func(kind, label string) workflowStep {
		return workflowStep{Command: []string{"pmtool", kind, "--module", o.Module}, Label: label, Kind: kind, Permission: profile{Module: true, ReadOnly: kind == "audit" || kind == "sync-noop", TemplateToken: kind != "migrate"}}
	}
	tool := func(name, label string, args ...string) workflowStep {
		image, directory := o.VoxBox, "/repo"
		if !containerImagePattern.MatchString(image) {
			fail("a valid tool image is required")
		}
		command := []string{"docker", "run", "--rm", "--volume", o.Module + ":" + directory, "--workdir", directory}
		if a.tty && a.colorEnabled() {
			command = append(command, "--tty")
		}
		if containsArgument(args, "unit") || containsArgument(args, "spec") || containsArgument(args, "check") {
			command = append(command, "--tmpfs", directory+"/spec/fixtures/modules")
		}
		if a.getenv("NO_COLOR") != "" {
			command = append(command, "--env", "NO_COLOR")
		}
		if name == "jig" {
			command = append(command, "--entrypoint", "jig")
		}
		command = append(command, image)
		command = append(command, args...)
		return workflowStep{Command: command, Label: label, Tool: name, Permission: profile{Module: true}}
	}
	vox := []workflowStep{tool("voxbox", "VoxBox validation", "validate", "lint", "rubocop"), tool("voxbox", "VoxBox specs", "spec")}
	release := tool("voxbox", "Release/changelog checks", "check", "check_changelog")
	var steps []workflowStep
	switch o.Command {
	case "audit", "sync", "sync-noop", "migrate":
		steps = []workflowStep{host(o.Command, o.Command)}
	case "voxbox":
		steps = vox
	case "release-checks":
		steps = []workflowStep{release}
	case "module-new":
		steps = []workflowStep{tool("jig", "Generate module scaffold", "new", "module", o.Name, "--skip-interview")}
	case "module-build":
		steps = []workflowStep{tool("jig", "Build module archive", "build")}
	case "module-convert", "module-convert-noop":
		if _, err := os.Lstat(filepath.Join(o.Module, ".sync.lock.json")); !os.IsNotExist(err) {
			fail("module conversion is only for initial onboarding; use sync for a managed baseline")
		}
		if info, err := os.Stat(filepath.Join(o.Module, "manifests")); err != nil || !info.IsDir() {
			fail("missing module manifests directory")
		}
		arguments := []string{"convert", "--skip-interview"}
		if strings.HasSuffix(o.Command, "-noop") {
			arguments = append(arguments, "--dry-run")
		}
		steps = []workflowStep{tool("jig", "Convert module scaffold", arguments...)}
	case "check":
		steps = append([]workflowStep{host("audit", "Module audit")}, vox...)
		steps = append(steps, release)
	default:
		fail("unsupported validation command: %s", o.Command)
	}
	if o.Stage != "" {
		index := 0
		if o.Stage == "unit" {
			index = 1
		}
		steps = []workflowStep{steps[index]}
	}
	return workflowSequence{Steps: steps}
}
func containsArgument(values []string, value string) bool {
	return slices.Contains(values, value)
}
func (a *app) toolPlan(cfg config, step workflowStep, native bool) plan {
	if len(step.Command) < 2 || step.Command[0] != "docker" {
		fail("invalid external tool plan")
	}
	if native {
		p := nativeStep(step)
		environment := privateValidationEnvironment()
		maps.Copy(environment, p.Environment)
		p.Environment = environment
		return p
	}
	for _, arg := range step.Command {
		if strings.Contains(arg, "docker.sock") {
			fail("tool containers must not receive a Docker socket")
		}
	}
	return plan{Docker: cfg.Docker, Args: step.Command[1:]}
}
func operationExit(err error) int {
	if err == nil {
		return 0
	}
	if exited, ok := errors.AsType[*exec.ExitError](err); ok {
		if status, ok := exited.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
	}
	var status interface{ ExitCode() int }
	if errors.As(err, &status) {
		if code := status.ExitCode(); code > 0 {
			return code
		}
	}
	return 1
}

type commandFailure struct {
	operation string
	code      int
}

func (e commandFailure) Error() string {
	return fmt.Sprintf("%s failed with exit %d", e.operation, e.code)
}
func (e commandFailure) ExitCode() int { return e.code }
func (a *app) executeSequence(cfg config, image string, args []string, native, dryRun bool) int {
	o := a.parseOperation(args)
	sequence := a.sequence(o)
	if dryRun {
		return a.printSequence(sequence)
	}
	a.notice("RUN", o.Command, "1;36")
	for i, step := range sequence.Steps {
		if len(sequence.Steps) > 1 {
			a.notice("STEP", fmt.Sprintf("%d/%d %s", i+1, len(sequence.Steps), step.Label), "36")
		}
		code := 0
		if step.Kind != "" {
			err := a.executeTemplate(cfg, image, o, step.Kind, native)
			code = operationExit(err)
			if err != nil {
				a.notice("FAIL", err.Error(), "1;31")
			}
		} else {
			code = a.executeToolStep(cfg, o, step, native)
		}
		if code != 0 {
			a.notice("FAIL", fmt.Sprintf("%s: %s (exit %d)", o.Command, step.Label, code), "1;31")
			a.workflowHint(args, o.Command, false)
			return code
		}
		if len(sequence.Steps) > 1 {
			a.notice("PASS", step.Label, "32")
		}
	}
	a.notice("OK", o.Command+" completed", "1;32")
	a.workflowHint(args, o.Command, true)
	return 0
}

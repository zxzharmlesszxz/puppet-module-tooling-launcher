package pmtool

import (
	"os"
	"path/filepath"
	"strings"
)

func toolArguments(command []string) []string {
	if len(command) < 3 || command[0] != "docker" || command[1] != "run" {
		fail("invalid external tool plan")
	}
	for index := 2; index < len(command); index++ {
		switch command[index] {
		case "--rm", "--tty", "--interactive":
		case "--volume", "--mount", "--workdir", "--env", "--tmpfs", "--entrypoint":
			index++
		default:
			if strings.HasPrefix(command[index], "-") {
				fail("unsupported external tool option")
			}
			return command[index+1:]
		}
	}
	fail("external tool image is missing")
	return nil
}

func nativeStep(step workflowStep) plan {
	if len(step.Command) == 0 {
		fail("empty native workflow command")
	}
	if step.Command[0] != "docker" {
		fail("native validation requires an external tool step")
	}
	arguments := toolArguments(step.Command)
	if step.Tool == "jig" {
		return plan{Docker: "jig", Args: arguments}
	}
	if step.Tool != "voxbox" {
		fail("unsupported native tool %q", step.Tool)
	}
	return plan{Docker: "bundle", Args: append([]string{"exec", "rake", "-f", "/opt/voxbox/Rakefile"}, arguments...), Environment: map[string]string{"BUNDLE_IGNORE_CONFIG": "1", "BUNDLE_GEMFILE": "/opt/voxbox/Gemfile", "BUNDLE_PATH": "/opt/voxbox/vendor/bundle"}}
}

func moduleArgument(cwd string, args []string) string {
	module := cwd
	for index := 0; index < len(args); index++ {
		if args[index] == "--module" {
			if index+1 >= len(args) {
				fail("--module requires a path")
			}
			index++
			module = args[index]
		} else if value, ok := strings.CutPrefix(args[index], "--module="); ok {
			module = value
		}
	}
	if !filepath.IsAbs(module) {
		module = filepath.Join(cwd, module)
	}
	module = must(filepath.EvalSymlinks(module))
	if !must(os.Stat(module)).IsDir() || module == string(filepath.Separator) {
		fail("module must be a directory other than /")
	}
	return module
}

func (a *app) runNative(args []string, dryRun bool) int {
	allowed := map[string]bool{"audit": true, "module-build": true, "voxbox": true, "release-checks": true, "check": true}
	if !allowed[args[0]] {
		fail("native executor supports validation commands only (plus module-build); run Git/API commands without --executor native")
	}
	return a.executeSequence(config{}, "", args, true, dryRun)
}

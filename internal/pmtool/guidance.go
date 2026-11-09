package pmtool

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/hostflow"
)

func (a *app) printGuidance(message string, args []string) {
	text := strings.TrimSpace(message + " " + quotedCommand(args))
	if text != "" {
		a.notice("NEXT", text, "1;33")
	}
}
func hintOptions(args []string, target, cwd string) hostflow.Options {
	o := hostflow.Options{Command: target, Module: cwd}
	for i := 1; i < len(args); i++ {
		key, value, equal := strings.Cut(args[i], "=")
		if !equal {
			i++
			if i >= len(args) {
				break
			}
			value = args[i]
		}
		switch key {
		case "--module":
			o.Module = value
		case "--branch":
			o.Branch = value
		case "--ref":
			o.Ref = value
		case "--title":
			o.Title = value
		case "--files":
			o.Files = value
		case "--version":
			o.Version = value
		case "--ci":
			o.Settings.CI = value
		case "--template":
			o.Settings.Template = value
		case "--provider":
			o.Settings.Provider = value
		case "--ci-project":
			o.CIProject = value
		case "--watch":
			if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 && seconds <= 86400 {
				o.Watch = time.Duration(seconds) * time.Second
			}
		}
	}
	if !filepath.IsAbs(o.Module) {
		o.Module = filepath.Join(cwd, o.Module)
	}
	return o
}
func (a *app) workflowHint(args []string, target string, success bool) {
	// Advisory local reads cannot change the command's exit code or make API calls.
	o := hintOptions(args, target, a.cwd)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	safeEnv := func(key string) string {
		switch key {
		case "GITLAB_TOKEN", "GITHUB_TOKEN", "TOOLING_TEMPLATE_TOKEN":
			return ""
		}
		return a.getenv(key)
	}
	e, err := hostflow.New(ctx, o, safeEnv, a.stdout)
	if err != nil {
		message, next := hostflow.Guidance(o, o.Module, hostflow.GuidanceContext{}, success)
		a.printGuidance(message, next)
		return
	}
	message, next := e.Advice(o, success)
	a.printGuidance(message, next)
}

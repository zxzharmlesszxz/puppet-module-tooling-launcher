package pmtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/hostflow"
)

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

func quotedCommand(args []string) string {
	result := make([]string, len(args))
	for i, arg := range args {
		if shellSafe.MatchString(arg) {
			result[i] = arg
		} else {
			result[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		}
	}
	return strings.Join(result, " ")
}
func (a *app) runHost(args []string, dryRun bool) int {
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-h" {
			return a.localDocument([]string{"usage"})
		}
	}
	opts := must(hostflow.Parse(args, a.cwd))
	if dryRun {
		must(fmt.Fprintln(a.stdout, string(must(json.MarshalIndent(struct {
			Executor  string           `json:"executor"`
			Operation hostflow.Options `json:"operation"`
		}{"host", opts}, "", "  ")))))
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	engine := must(hostflow.New(ctx, opts, a.getenv, a.stdout))
	if a.colorEnabled() {
		engine.Color = "always"
	}
	a.notice("RUN", args[0], "1;36")
	if err := engine.Run(opts); err != nil {
		a.notice("FAIL", args[0]+": "+err.Error(), "1;31")
		message, next := engine.Advice(opts, false)
		a.printGuidance(message, next)
		if ctx.Err() != nil {
			return 130
		}
		if exited, ok := errors.AsType[*exec.ExitError](err); ok && exited.ExitCode() > 0 {
			return exited.ExitCode()
		}
		return 1
	}
	a.notice("OK", args[0]+" completed", "1;32")
	message, next := engine.Advice(opts, true)
	a.printGuidance(message, next)
	return 0
}

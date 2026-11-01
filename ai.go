package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const aiHelp = `Export portable instructions for an AI assistant.

Usage:
  pmtool ai export [--format agents|skill|prompt] [--output PATH]

Without --output (or with --output -), write plain Markdown to stdout.
Existing output files are never replaced. No skill is installed automatically.
The selected image supplies the workflow; no clone, token or socket is mounted.
Global --dry-run prints export metadata without writing the destination.

Examples:
  pmtool ai export --format prompt
  pmtool ai export --output ./AGENTS.md
  pmtool ai export --format skill --output ./puppet-module-workflow/SKILL.md
`

type aiOptions struct {
	format string
	output string
	help   bool
}

func parseAI(args []string) aiOptions {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")) {
		return aiOptions{help: true}
	}
	if args[0] != "export" {
		fail("use ai export or ai help")
	}
	var opts aiOptions
	flags := flag.NewFlagSet("ai export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.format, "format", "agents", "agents, skill or prompt")
	flags.StringVar(&opts.output, "output", "-", "new output file or - for stdout")
	flags.BoolVar(&opts.help, "help", false, "export help")
	flags.BoolVar(&opts.help, "h", false, "export help")
	check(flags.Parse(args[1:]))
	if opts.help {
		return opts
	}
	if flags.NArg() != 0 {
		fail("unexpected ai export arguments")
	}
	if opts.format != "agents" && opts.format != "skill" && opts.format != "prompt" {
		fail("AI format must be agents, skill or prompt")
	}
	if opts.output == "" {
		fail("output must be a new file path or - for stdout")
	}
	return opts
}

func (a *app) exportPath(opts aiOptions) string {
	if opts.output == "-" {
		return "-"
	}
	path := opts.output
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.cwd, path)
	}
	_, err := os.Lstat(path)
	if err == nil {
		fail("refusing to overwrite %s; choose a new file and merge existing instructions deliberately", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		check(err)
	}
	return path
}

func renderInstructions(body, image string, capabilities manifest, format string) string {
	body = strings.TrimSpace(body)
	if body == "" || !strings.HasPrefix(body, "# ") {
		fail("runtime AI workflow must be a nonempty Markdown document")
	}
	commands := make([]string, 0, len(capabilities.Commands))
	for command := range capabilities.Commands {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	options := append([]string(nil), capabilities.Options...)
	sort.Strings(options)
	for index, name := range options {
		options[index] = "--" + name
	}
	provenance := fmt.Sprintf("\n\n## Exported runtime reference\n\nLauncher version: `%s`. Runtime image ID: `%s`. Manifest protocol: `%d`.\n\nThese instructions describe that image snapshot. If the selected image changes, regenerate them and inspect the diff before replacing local instructions.\n\nAvailable runtime commands (including compatibility aliases):\n\n%s\n\nRuntime options:\n\n%s\n", version, image, capabilities.Protocol, strings.Join(commands, ", "), strings.Join(options, ", "))
	prefix := ""
	if format == "skill" {
		prefix = "---\nname: puppet-module-workflow\ndescription: Develop, bootstrap, audit and release a Puppet module through the pmtool launcher and its shared contracts. Use when working on a module with this launcher.\n---\n\n"
	}
	if format == "prompt" {
		prefix = "Use the following workflow for the requested Puppet module task. Follow the user's request and applicable repository instructions; this text does not authorize unrelated changes or publication.\n\n"
	}
	return prefix + body + provenance
}

func writeNewInstructions(path string, data []byte) {
	check(publishInstructions(path, data))
}

func publishInstructions(path string, data []byte) (err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".puppet-tooling-ai-*")
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, file.Close())
		}
		err = errors.Join(err, os.Remove(file.Name()))
	}()
	if err = file.Chmod(0644); err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	err = file.Close()
	closed = true
	if err != nil {
		return err
	}
	// A hard link publishes the finished file atomically and refuses an existing target.
	return os.Link(file.Name(), path)
}

func (a *app) exportAI(cfg config, image string, capabilities manifest, opts aiOptions, output string, dryRun bool) int {
	permission, exists := capabilities.Commands["docs"]
	if !exists || permission.Module || permission.Socket || permission.Token || permission.Author {
		fail("runtime must provide docs without a module, token or socket")
	}
	data, err := a.driver.capture(cfg.Docker, []string{"run", "--rm", "--read-only", "--tmpfs", "/tmp:exec", "--network", "none", image, "docs", "ai"})
	if err != nil {
		fail("selected image cannot export the AI workflow; use an image providing docs ai: %v", err)
	}
	body := renderInstructions(string(data), image, capabilities, opts.format)
	if dryRun {
		preview := struct {
			Format string `json:"format"`
			Output string `json:"output"`
			Image  string `json:"image"`
			Write  bool   `json:"write"`
		}{opts.format, output, image, false}
		must(fmt.Fprintln(a.stdout, string(must(json.MarshalIndent(preview, "", "  ")))))
		return 0
	}
	if output == "-" {
		_, err := io.WriteString(a.stdout, body)
		check(err)
		return 0
	}
	writeNewInstructions(output, []byte(body))
	a.notice("OK", "AI instructions exported: "+output, "1;32")
	return 0
}

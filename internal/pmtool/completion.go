package pmtool

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/hostflow"
)

const bashCompletion = `_pmtool() {
  local cur="${COMP_WORDS[COMP_CWORD]}" prev="${COMP_WORDS[COMP_CWORD-1]}" choices
  local index
  local -a args
  args=()
  for ((index=1; index<=COMP_CWORD; index++)); do args+=("${COMP_WORDS[index]}"); done
  local IFS=$'\n'
  case "$prev" in
    --config|--module|--output|--file|--docker|--socket)
      COMPREPLY=( $(compgen -f -- "$cur") )
      return
      ;;
  esac
  choices="$("${COMP_WORDS[0]}" __complete "${args[@]}" 2>/dev/null)" || return
  COMPREPLY=( $(compgen -W "$choices" -- "$cur") )
}
complete -F _pmtool pmtool
`

const zshCompletion = `#compdef pmtool
_pmtool() {
  local -a choices
  case "${words[CURRENT-1]}" in
    --config|--module|--output|--file|--docker|--socket)
      _files
      return
      ;;
  esac
  choices=("${(@f)$("${words[1]}" __complete "${words[@]:1:$((CURRENT-1))}" 2>/dev/null)}")
  compadd -- "${choices[@]}"
}
if [[ "${funcstack[1]}" == _pmtool ]]; then
  _pmtool "$@"
else
  compdef _pmtool pmtool
fi
`

var localCompletionCommands = []string{"help", "docs", "version", "config", "ai", "completion"}
var globalCompletionFlags = []string{"--image", "--config", "--docker", "--socket", "--executor", "--color", "--dry-run", "--help", "--version"}
var completionName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func (a *app) printCompletion(args []string) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "help")) {
		must(fmt.Fprintln(a.stdout, "Usage: pmtool completion bash|zsh\nPrint a completion script to stdout; no Docker or configuration required."))
		return 0
	}
	if len(args) != 1 {
		fail("use completion bash or completion zsh")
	}
	switch args[0] {
	case "bash":
		must(fmt.Fprint(a.stdout, bashCompletion))
	case "zsh":
		must(fmt.Fprint(a.stdout, zshCompletion))
	default:
		fail("unsupported completion shell: %s; choose bash or zsh", args[0])
	}
	return 0
}

func (a *app) completionManifest(_ options) manifest { return launcherManifest() }

func completionEnum(flag string) []string {
	switch flag {
	case "--executor":
		return []string{"docker", "native"}
	case "--stage":
		return []string{"validate", "unit"}
	case "--color":
		return []string{"auto", "always", "never"}
	case "--provider":
		return []string{"gitlab", "github"}
	case "--format":
		return []string{"agents", "skill", "prompt"}
	case "--section":
		return []string{"list"}
	}
	return nil
}

func (a *app) complete(opts options, words []string) int {
	if len(words) == 0 {
		words = []string{""}
	}
	current := words[len(words)-1]
	previous := ""
	if len(words) > 1 {
		previous = words[len(words)-2]
	}
	command := ""
	var rest []string
	for index := 0; index < len(words)-1; index++ {
		word := words[index]
		if command != "" {
			rest = append(rest, word)
			continue
		}
		if !strings.HasPrefix(word, "-") {
			command = word
			continue
		}
		key, value, equal := strings.Cut(word, "=")
		if key == "--dry-run" || key == "--help" || key == "--version" {
			continue
		}
		if !equal && index+1 < len(words)-1 {
			index++
			value = words[index]
		}
		switch key {
		case "--config":
			opts.config = value
		case "--image":
			opts.image = value
		case "--docker":
			opts.docker = value
		}
	}
	if !strings.Contains(current, "=") && len(completionEnum(previous)) == 0 {
		switch previous {
		case "--image", "--config", "--docker", "--socket", "--executor", "--stage", "--module", "--output", "--file", "--files", "--ci", "--template", "--ref", "--branch", "--title", "--watch", "--version", "--voxbox-image", "--ci-project", "--name", "--launcher-version", "--origin", "--pipeline", "--job", "--tail":
			return 0
		}
	}
	var candidates []string
	if command == "docs" && previous == "--section" {
		text := manual
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "--") {
			if _, ok := documentationTopics[rest[0]]; ok {
				text = a.workflowDocument(rest[0])
			} else if rest[0] != "launcher" {
				return 0
			}
		}
		candidates = []string{"list"}
		for heading := range strings.SplitSeq(a.document(text, "list"), "\n") {
			if title := strings.TrimSpace(heading); title != "" {
				candidates = append(candidates, title)
			}
		}
	} else if key, prefix, equal := strings.Cut(current, "="); equal && strings.HasPrefix(key, "--") {
		values := completionEnum(key)
		if key == "--provider" && command == "template-init" {
			values = append(values, "both")
		}
		for _, value := range values {
			if strings.HasPrefix(value, prefix) {
				candidates = append(candidates, key+"="+value)
			}
		}
	} else if values := completionEnum(previous); len(values) > 0 {
		candidates = values
		if previous == "--provider" && command == "template-init" {
			candidates = append(candidates, "both")
		}
	} else {
		switch command {
		case "":
			candidates = append(append([]string(nil), localCompletionCommands...), globalCompletionFlags...)
			for name := range a.completionManifest(opts).Commands {
				if completionName.MatchString(name) {
					candidates = append(candidates, name)
				}
			}
		case "completion":
			if len(rest) == 0 {
				candidates = []string{"bash", "zsh"}
			}
		case "config":
			if len(rest) == 0 {
				candidates = []string{"init", "path", "show"}
			} else if rest[0] == "init" {
				candidates = []string{"--image", "--help"}
			}
		case "ai":
			if len(rest) == 0 {
				candidates = []string{"export", "help"}
			} else if rest[0] == "export" {
				candidates = []string{"--format", "--output", "--help"}
			}
		case "help":
			if len(rest) == 0 {
				candidates = []string{"runtime"}
			}
		case "docs":
			if len(rest) == 0 {
				candidates = []string{"launcher", "--section"}
				for topic := range documentationTopics {
					candidates = append(candidates, topic)
				}
			} else {
				candidates = []string{"--section"}
			}
		case "template-init":
			candidates = []string{"--name", "--output", "--provider", "--launcher-version", "--origin", "--help"}
		case "version":
		default:
			candidates = a.commandCompletionFlags(opts, command)
		}
	}
	sort.Strings(candidates)
	last := ""
	for _, value := range candidates {
		if value != last && strings.HasPrefix(value, current) {
			must(fmt.Fprintln(a.stdout, value))
		}
		last = value
	}
	return 0
}

func (a *app) commandCompletionFlags(opts options, command string) []string {
	candidates := []string{"--help"}
	if hostflow.IsCommand(command) {
		for _, name := range a.completionManifest(opts).Options {
			if hostflow.AllowsOption(command, "--"+name) {
				candidates = append(candidates, "--"+name)
			}
		}
		return candidates
	}
	aliases := map[string]string{"bootstrap-local": "sync", "bootstrap-local-noop": "sync-noop", "audit-local": "audit"}
	if actual, ok := aliases[command]; ok {
		command = actual
	}
	flags := "module"
	switch command {
	case "sync", "sync-noop":
		flags += " files ci provider template ci-project voxbox-image"
	case "audit", "check":
		flags += " ci provider template ci-project voxbox-image"
	case "voxbox":
		flags += " stage voxbox-image"
	case "module-new":
		flags += " name voxbox-image"
	case "module-build", "module-convert", "module-convert-noop", "release-checks":
		flags += " voxbox-image"
	case "template-test", "forge-publish":
	default:
		return candidates
	}
	for name := range strings.FieldsSeq(flags) {
		candidates = append(candidates, "--"+name)
	}
	return candidates
}

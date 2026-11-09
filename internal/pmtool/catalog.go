package pmtool

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"strings"

	documentation "github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/docs"
	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/workflow"
)

func launcherManifest() manifest {
	var result manifest
	decode(must(workflow.Files.ReadFile("catalog.json")), &result)
	return result
}

func (realDriver) capturePlan(p plan) ([]byte, error) {
	cmd := commandForPlan(p)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", p.Docker, strings.TrimSpace(stderr.String()), err)
	}
	return output, nil
}

var documentationTopics = map[string]string{"usage": "USAGE.md", "onboarding": "MODULE-ONBOARDING.md", "ai": "AI-WORKFLOW.md", "ci": "CI-PROVIDERS.md", "templates": "TEMPLATES.md", "tokens": "TOKENS.md", "inputs": "COMMAND-INPUTS.md", "fixtures": "FIXTURES.md"}

func (a *app) workflowDocument(name string) string {
	file, ok := documentationTopics[name]
	if !ok {
		fail("unknown document %q; choose launcher, usage, onboarding, ai, ci, templates, tokens, inputs or fixtures", name)
	}
	return documentation.Read(file)
}

func (a *app) localDocument(args []string) int {
	name, section := "launcher", ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		name, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("docs", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&section, "section", "", "section title or list")
	check(flags.Parse(args))
	if flags.NArg() != 0 {
		fail("use docs [DOCUMENT] [--section TITLE|list]")
	}
	if name == "launcher" {
		must(fmt.Fprint(a.stdout, a.document(manual, section)))
	} else {
		must(fmt.Fprint(a.stdout, a.document(a.workflowDocument(name), section)))
	}
	return 0
}

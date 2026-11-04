package pmtool

import (
	"bytes"
	"fmt"
	"strings"

	documentation "github.com/zxzharmlesszxz/puppet-module-tooling-launcher/docs"
	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/internal/workflow"
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

func (a *app) workflowDocument(name string) string {
	file, ok := map[string]string{"usage": "USAGE.md", "onboarding": "MODULE-ONBOARDING.md", "ai": "AI-WORKFLOW.md", "ci": "CI-PROVIDERS.md", "templates": "TEMPLATES.md"}[name]
	if !ok {
		fail("unknown document %q; choose launcher, usage, onboarding, ai, ci or templates", name)
	}
	return documentation.Read(file)
}

func (a *app) localDocument(args []string) int {
	name, section := "usage", ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		name, args = args[0], args[1:]
	}
	if len(args) > 0 {
		if len(args) != 2 || args[0] != "--section" {
			fail("use docs DOCUMENT [--section TITLE|list]")
		}
		section = args[1]
	}
	if name == "launcher" {
		must(fmt.Fprint(a.stdout, a.document(manual, section)))
	} else {
		must(fmt.Fprint(a.stdout, a.document(a.workflowDocument(name), section)))
	}
	return 0
}

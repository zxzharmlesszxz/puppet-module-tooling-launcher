package pmtool

import (
	"encoding/json"
	"fmt"
)

type workflowStep struct {
	Command    []string `json:"command"`
	Label      string   `json:"label"`
	Tool       string   `json:"tool,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Permission profile  `json:"permission"`
}

type workflowSequence struct {
	Steps []workflowStep `json:"steps"`
}

func (a *app) runWorkflow(cfg config, image string, args []string, dryRun bool) int {
	return a.executeSequence(cfg, image, args, false, dryRun)
}

func (a *app) printSequence(sequence workflowSequence) int {
	data := must(json.MarshalIndent(sequence, "", "  "))
	must(fmt.Fprintln(a.stdout, string(data)))
	return 0
}

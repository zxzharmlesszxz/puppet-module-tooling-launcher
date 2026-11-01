package main

import "strings"

type workflowSettings struct {
	CI       string `json:"ci"`
	Provider string `json:"provider"`
	Template string `json:"template"`
}

// The selected runtime parses module YAML once, without secrets, network or socket.
func (a *app) resolveWorkflowSettings(cfg config, image, module string, capabilities manifest, cli workflowSettings) workflowSettings {
	permission, ok := capabilities.Commands["settings"]
	if !ok || !permission.Module || !permission.ReadOnly || permission.Socket || permission.Token || permission.Author {
		fail("runtime must provide read-only module settings without a token or socket")
	}
	args := []string{"run", "--rm", "--read-only", "--tmpfs", "/tmp:exec", "--network", "none", "--mount", mount(module, module, true), "--workdir", module, image, "settings", "--module", module}
	if cli.CI != "" {
		args = append(args, "--ci", cli.CI)
	}
	if cli.Provider != "" {
		args = append(args, "--provider", cli.Provider)
	}
	if cli.Template != "" {
		args = append(args, "--template", cli.Template)
	}
	data := must(a.driver.capture(cfg.Docker, args))
	var selected workflowSettings
	decode(data, &selected)
	if selected.Provider != "gitlab" && selected.Provider != "github" {
		fail("runtime settings provider must be gitlab or github")
	}
	if (cli.CI != "" && selected.CI != cli.CI) || (cli.Provider != "" && selected.Provider != cli.Provider) || (cli.Template != "" && selected.Template != cli.Template) {
		fail("runtime settings did not preserve CLI overrides")
	}
	if strings.ContainsAny(selected.CI, "\r\n") {
		fail("CI reference must be a single line")
	}
	return selected
}

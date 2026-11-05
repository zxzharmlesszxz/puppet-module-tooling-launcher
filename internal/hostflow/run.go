package hostflow

import (
	"encoding/json"
	"fmt"
)

func (e *Engine) Run(o Options) (err error) {
	defer func() {
		if err == nil {
			err = e.outputErr
		}
	}()
	switch o.Command {
	case "settings":
		settings, err := ReadSettings(e.Project.Root, o.Settings)
		if err != nil {
			return err
		}
		if settings.Provider == "" {
			settings.Provider = e.Project.Provider
		}
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(e.Out, string(data))
		return err
	case "status", "diff":
		args := []string{"-c", "color.ui=" + e.color(), "status", "--short", "--branch"}
		if o.Command == "diff" {
			args = []string{"--no-pager", "diff", "--no-ext-diff", "--no-textconv", "--color=" + e.color(), "--"}
			if o.File != "" {
				args = append(args, o.File)
			}
		}
		data, err := e.Git.Run(args...)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(e.Out, data)
		return err
	case "main-sync":
		return e.syncMain()
	case "bootstrap-prepare":
		return e.prepare(o, true)
	case "release-prepare":
		return e.prepare(o, false)
	case "bootstrap-commit", "bootstrap-commit-noop":
		return e.commit(o, true)
	case "release-commit", "release-commit-noop":
		return e.commit(o, false)
	case "release-submit":
		if err := e.token(); err != nil {
			return err
		}
		if err := e.commit(o, false); err != nil {
			return err
		}
		if err := e.push(o, false); err != nil {
			e.Recovery = "release-push"
			return fmt.Errorf("release commit is preserved; retry release-push: %w", err)
		}
		return nil
	case "bootstrap-push", "bootstrap-push-noop":
		return e.push(o, true)
	case "release-push", "release-push-noop":
		return e.push(o, false)
	case "release-tag", "release-tag-noop":
		return e.tag(o)
	case "cleanup", "cleanup-noop":
		return e.cleanup(o)
	case "mr-create":
		return e.request(o, true, false)
	case "mr-merge":
		return e.request(o, false, true)
	case "mr":
		if err := e.request(o, true, true); err != nil {
			return err
		}
		if err := e.syncMain(); err != nil {
			e.Recovery = "main-sync"
			return fmt.Errorf("merge succeeded; retry main-sync without repeating merge: %w", err)
		}
		return nil
	case "pipeline-logs":
		return e.pipelineLogs(o)
	case "pipeline-jobs":
		return e.pipelineJobs(o)
	case "policy", "policy-noop", "policy-delete-master":
		if err := e.token(); err != nil {
			return err
		}
		levels, err := e.policyLevels()
		if err != nil {
			return err
		}
		return e.API.Policy(e.Context, o.Noop, o.Command == "policy-delete-master", e.Out, levels)
	}
	return fmt.Errorf("unsupported host workflow: %s", o.Command)
}
func (e *Engine) color() string {
	if e.Color == "always" {
		return "always"
	}
	return "never"
}

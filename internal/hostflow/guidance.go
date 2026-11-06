package hostflow

import (
	"strconv"
	"strings"
	"time"
)

// GuidanceContext is advisory local state; obtaining it never contacts a provider.
type GuidanceContext struct {
	Branch, Version string
	Dirty, Pushed   bool
}

func (e *Engine) guidanceContext() GuidanceContext {
	c := GuidanceContext{}
	c.Branch, _ = e.branch()
	if metadata, err := e.metadata(""); err == nil {
		c.Version = metadata.Version
	}
	status, err := e.Git.Text("status", "--porcelain", "--untracked-files=all")
	c.Dirty = err == nil && status != ""
	head, ok := e.Git.Optional("rev-parse", "--verify", "HEAD")
	if ok && c.Branch != "" {
		tracked, exists := e.Git.Optional("rev-parse", "--verify", "refs/remotes/origin/"+c.Branch)
		c.Pushed = exists && head == tracked
	}
	return c
}
func (e *Engine) Next(o Options, success bool) []string {
	_, args := Guidance(o, e.Project.Root, e.guidanceContext(), success)
	return args
}
func (e *Engine) Advice(o Options, success bool) (string, []string) {
	if !success && e.Recovery != "" {
		o.Command = e.Recovery
		if e.RecoveryRef != "" {
			o.Ref = e.RecoveryRef
			o.Watch = 5e9
		}
	}
	return Guidance(o, e.Project.Root, e.guidanceContext(), success)
}

// Guidance selects the next reviewed operation. It never executes that operation.
func Guidance(o Options, root string, c GuidanceContext, success bool) (string, []string) {
	selected := func(target string, flags ...string) []string {
		args := []string{"pmtool", target, "--module", root}
		for _, v := range []struct{ key, value string }{{"--template", o.Settings.Template}, {"--ci", o.Settings.CI}, {"--provider", o.Settings.Provider}} {
			if v.value != "" {
				args = append(args, v.key, v.value)
			}
		}
		if o.Settings.CI == "" && o.CIProject != "" {
			args = append(args, "--ci-project", o.CIProject)
		}
		return append(args, flags...)
	}
	branch := o.Branch
	if branch == "" {
		branch = c.Branch
	}
	if !success {
		if o.Command == "release-tag" && c.Version != "" {
			return "Inspect the failure and tag state. If pushed, preserve the tag and inspect publication:", selected("pipeline-jobs", "--ref", "v"+c.Version, "--watch", "5")
		}
		var flags []string
		switch o.Command {
		case "release-prepare":
			flags = []string{"--version", o.Version}
		case "bootstrap-prepare", "cleanup", "cleanup-noop", "mr-merge":
			flags = []string{"--branch", branch}
		case "mr", "mr-create":
			flags = []string{"--branch", branch, "--title", o.Title}
		case "pipeline-logs":
			if o.Ref != "" {
				flags = append(flags, "--ref", o.Ref)
			}
			if o.PipelineID != 0 {
				flags = append(flags, "--pipeline", strconv.FormatInt(o.PipelineID, 10))
			}
			if o.JobID != 0 {
				flags = append(flags, "--job", strconv.FormatInt(o.JobID, 10))
			}
			if o.Failed {
				flags = append(flags, "--failed")
			}
			flags = append(flags, "--tail", strconv.Itoa(o.Tail))
			return "Inspect log access/selection; choose a new output path if the previous file exists:", selected(o.Command, flags...)
		case "pipeline-jobs":
			flags = []string{"--ref", o.Ref, "--watch", formatSeconds(o.Watch)}
		default:
			if o.Files != "" && (strings.Contains(o.Command, "-commit") || o.Command == "release-submit" || o.Command == "sync" || o.Command == "sync-noop") {
				flags = []string{"--files", o.Files}
			}
		}
		return "Fix the reported failure, then retry:", selected(o.Command, flags...)
	}
	switch o.Command {
	case "status", "diff":
		return "Review clone state and changes before choosing the next operation.", nil
	case "bootstrap-prepare":
		return "Inspect metadata/fixtures, migrate legacy settings and synchronize the baseline:", selected("sync")
	case "release-prepare":
		return "Update metadata/changelog for " + o.Version + "; synchronize the baseline:", selected("sync")
	case "migrate":
		return "", selected("sync")
	case "module-convert-noop":
		return "Review the conversion preview before applying:", selected("module-convert")
	case "module-convert":
		return "Review metadata/fixtures, then migrate legacy settings:", selected("migrate")
	case "sync-noop":
		var flags []string
		if o.Files != "" {
			flags = []string{"--files", o.Files}
		}
		return "If changed, review then apply; if unchanged, continue with check:", selected("sync", flags...)
	case "sync", "main-sync", "mr", "audit":
		return "", selected("check")
	case "module-new", "module-build":
		return "Review generated files and metadata before choosing the next operation.", nil
	case "voxbox":
		return "", selected("release-checks")
	case "bootstrap-commit":
		return "", selected("bootstrap-push")
	case "release-commit":
		return "", selected("release-push")
	case "bootstrap-push":
		return "", selected("mr", "--title", "[UPD] Update module")
	case "release-submit", "release-push":
		return "", selected("mr", "--title", "[REL] Release "+c.Version)
	case "mr-create":
		return "", selected("mr-merge", "--branch", branch)
	case "mr-merge":
		return "", selected("main-sync")
	case "release-tag":
		return "Publication pipeline and forge-publish succeeded; verify the Forge artifact and canary. Preview cleanup of the actual completed branch:", selected("cleanup-noop", "--branch", "release/"+c.Version)
	case "cleanup":
		return "Branch cleanup is complete; release tags are preserved. Verify the canary or select the next module.", nil
	case "policy-noop":
		return "Review policy changes before applying:", selected("policy")
	case "policy":
		return "Prepare an explicit bootstrap branch for initial migration, or a release for an existing module.", selected("status")
	case "policy-delete-master":
		return "", selected("status")
	case "check", "release-checks":
		if c.Branch == "main" {
			if c.Dirty {
				return "Move pending main changes to the appropriate work branch before continuing.", nil
			}
			return "", selected("release-tag")
		}
		for _, prefix := range []string{"release", "bootstrap"} {
			if strings.HasPrefix(c.Branch, prefix+"/") {
				if c.Dirty {
					target := "bootstrap-commit"
					if prefix == "release" {
						target = "release-submit"
					}
					return "", selected(target)
				}
				if !c.Pushed {
					return "", selected(prefix + "-push")
				}
				title := "[UPD] Update module"
				if prefix == "release" {
					title = "[REL] Release " + c.Version
				}
				return "", selected("mr", "--branch", c.Branch, "--title", title)
			}
		}
		return "Review the work branch and choose its bootstrap or release workflow.", nil
	case "pipeline-logs":
		return "Review the selected job logs; fix the cause before retrying the failed operation.", nil
	case "pipeline-jobs":
		ref := o.Ref
		if ref == "" {
			ref = branch
		}
		prefix := ""
		if o.Watch == 0 {
			prefix = "If every required job succeeded for the current SHA:"
		}
		if ref == "main" && c.Branch == "main" {
			return prefix, selected("release-tag")
		}
		if strings.HasPrefix(ref, "v") && validVersion(strings.TrimPrefix(ref, "v")) == nil {
			return "Verify the Forge release and canary; select the actual completed branch before cleanup:", selected("cleanup-noop", "--branch", "release/"+strings.TrimPrefix(ref, "v"))
		}
		if strings.HasPrefix(ref, "release/") || strings.HasPrefix(ref, "bootstrap/") {
			return prefix, selected("mr-merge", "--branch", ref)
		}
		return "Review the selected pipeline/ref before proceeding; no merge or tag was performed.", nil
	}
	if strings.HasSuffix(o.Command, "-noop") {
		var flags []string
		if o.Command == "cleanup-noop" {
			flags = []string{"--branch", branch}
		}
		if strings.Contains(o.Command, "-commit") && o.Files != "" {
			flags = append(flags, "--files", o.Files)
		}
		return "Review the preview before applying:", selected(strings.TrimSuffix(o.Command, "-noop"), flags...)
	}
	return "Read the workflow:", []string{"pmtool", "docs", "onboarding", "--section", "list"}
}

func formatSeconds(delay time.Duration) string {
	return strconv.FormatInt(int64(delay/time.Second), 10)
}

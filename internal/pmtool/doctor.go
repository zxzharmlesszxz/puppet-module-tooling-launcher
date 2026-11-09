package pmtool

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/fixturekit"
	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/hostflow"
	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/templatekit"
)

const doctorUsage = "Usage: pmtool doctor [--module PATH] [--template REPO@VERSION] [--provider gitlab|github] [--voxbox-image IMAGE]"

func (a *app) doctor(opts options, args []string) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var module, template, provider string
	sdk := a.getenv("VOXBOX_IMAGE")
	if sdk == "" {
		sdk = voxboxImage
	}
	flags.StringVar(&module, "module", a.cwd, "module checkout")
	flags.StringVar(&template, "template", "", "template reference")
	flags.StringVar(&provider, "provider", "", "hosting provider")
	flags.StringVar(&sdk, "voxbox-image", sdk, "validation SDK image")
	if err := flags.Parse(args); errors.Is(err, flag.ErrHelp) {
		must(fmt.Fprintln(a.stdout, doctorUsage))
		return 0
	} else {
		check(err)
	}
	if flags.NArg() != 0 || module == "" {
		fail("%s", doctorUsage)
	}
	root := moduleArgument(a.cwd, []string{"--module", module})
	if opts.dryRun {
		must(fmt.Fprintln(a.stdout, "Would check origin, template access, VoxBox and Forge reads for", root))
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	must(fmt.Fprintln(a.stdout, a.paint("[RUN] doctor", "1;36")))

	settings, settingsErr := hostflow.ReadSettings(root, hostflow.Settings{Provider: provider, Template: template})
	var failed bool
	report := func(label, detail string, err error) {
		if err != nil {
			failed = true
			a.notice("FAIL", label+": "+err.Error(), "1;31")
		} else if detail == "" {
			a.notice("SKIP", label+": no Forge fixtures configured", "36")
		} else {
			a.notice("OK", label+": "+detail, "32")
		}
	}
	if settingsErr != nil {
		report("Origin", "", settingsErr)
		report("Template", "", settingsErr)
	} else {
		origin, err := a.doctorOrigin(root, settings)
		report("Origin", origin, err)
		detail, err := a.doctorTemplate(ctx, settings.Template)
		report("Template", detail, err)
	}
	detail, err := a.doctorSDK(ctx, opts, sdk)
	report("VoxBox", detail, err)
	forgeCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	forge, err := fixturekit.DiagnoseForge(forgeCtx, root, a.getenv)
	detail = ""
	if err == nil && forge.Catalogs > 0 {
		detail = fmt.Sprintf("%d release catalog(s), %d exact archive(s) readable", forge.Catalogs, forge.Archives)
		if forge.Unpinned > 0 {
			detail += fmt.Sprintf("; %d unpinned or range reference(s) checked at catalog level", forge.Unpinned)
		}
	}
	report("Forge", detail, err)
	if ctx.Err() != nil {
		return 130
	}
	if failed {
		return 1
	}
	return 0
}

func (a *app) doctorOrigin(root string, settings hostflow.Settings) (string, error) {
	origin, err := a.configuredOrigin(root)
	if err != nil {
		return "", fmt.Errorf("cannot read a single configured Git origin")
	}
	project, err := hostflow.ResolveProject(root, origin, settings, a.getenv)
	if err != nil {
		return "", err
	}
	return project.Provider + " " + project.ID, nil
}

func (a *app) doctorTemplate(ctx context.Context, reference string) (detail string, err error) {
	if reference == "" {
		return "", fmt.Errorf("set template.use in .sync.yml or pass --template")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err = templatekit.Checkout(ctx, reference, a.getenv, nil, func(t *templatekit.Template) error {
		detail = reference + " (" + t.Revision + ")"
		return nil
	})
	return detail, err
}

func doctorConfig(path string) (config, error) {
	cfg := initialConfig()
	data, err := os.ReadFile(path) // #nosec G304 -- A local, user-selected launcher configuration path.
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid launcher config: %w", err)
	}
	return cfg, nil
}

var doctorImageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func (a *app) doctorSDK(ctx context.Context, opts options, sdk string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if opts.executor == "native" {
		env := privateValidationEnvironment()
		env["BUNDLE_IGNORE_CONFIG"] = "1"
		env["BUNDLE_GEMFILE"] = "/opt/voxbox/Gemfile"
		env["BUNDLE_PATH"] = "/opt/voxbox/vendor/bundle"
		env["BUNDLE_FROZEN"] = "1"
		_, err := a.driver.capturePlan(plan{Context: ctx, Docker: "bundle", Args: []string{"exec", "rake", "-f", "/opt/voxbox/Rakefile", "-T"}, Environment: env})
		if err != nil {
			return "", fmt.Errorf("native VoxBox bundle or Rakefile is unavailable")
		}
		return "native bundle and Rakefile available", nil
	}
	path := opts.config
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.cwd, path)
	}
	cfg, err := doctorConfig(path)
	if err != nil {
		return "", err
	}
	if opts.docker != "" {
		cfg.Docker = opts.docker
	}
	if cfg.Docker == "" {
		cfg.Docker = "docker"
	}
	if opts.image != "" {
		cfg.Image = opts.image
	}
	env := privateValidationEnvironment()
	if _, err := a.driver.capturePlan(plan{Context: ctx, Docker: cfg.Docker, Args: []string{"info", "--format", "{{.ServerVersion}}"}, Environment: env}); err != nil {
		return "", fmt.Errorf("cannot reach Docker; check the daemon and --docker")
	}
	images := slices.Compact([]string{cfg.Image, sdk})
	for _, image := range images {
		if !containerImagePattern.MatchString(image) {
			return "", fmt.Errorf("set a valid --image or --voxbox-image")
		}
		args := []string{"image", "inspect", "--format", "{{.Id}}", image}
		result, err := a.driver.capturePlan(plan{Context: ctx, Docker: cfg.Docker, Args: args, Environment: env})
		if err != nil {
			return "", fmt.Errorf("image %s is not available locally; run docker pull %s", image, image)
		}
		if !doctorImageID.MatchString(strings.TrimSpace(string(result))) {
			return "", fmt.Errorf("invalid image ID from Docker for %s", image)
		}
	}
	return "Docker and required local images available", nil
}

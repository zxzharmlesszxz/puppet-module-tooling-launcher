# pmtool

[Українська](README.uk.md)

Versioned Puppet module workflows for GitLab and GitHub. `pmtool` owns commands,
validation sequences, Git/release guards, terminal guidance and AI instructions.
The pinned upstream [VoxBox](https://github.com/voxpupuli/container-voxbox) image
supplies Ruby and validation dependencies. No custom runtime image is required.
Team templates supply module baselines, contracts and audit policy.

## Use

Install the binary for Linux/macOS, amd64/arm64, as `pmtool`. Docker CLI and a local
Docker daemon are required for the default Docker validation executor; native CI
uses preinstalled tools. Git/API and release operations run in
Go on the host with the installed Git executable. From a module clone:

```sh
pmtool config init
pmtool check
```

The default image is `ghcr.io/voxpupuli/voxbox:10.1.0-openvox8@sha256:01bb8ce5e24d5b111934660f75e2786b4d7c7adcaa7518a564856d1d3041d10d`.
The image in `~/.config/puppet-module-tooling/config.json` selects ERB/audit/template-test
tools only. PDK and VoxBox validation have separate pins and --pdk-image/--voxbox-image
overrides; changing config.image does not replace them. Image and launcher versions
are independent. See [image selectors](docs/manual.md#image-selectors-and-environment).
Configuration contains host execution settings, never project settings or tokens.

Module settings belong to `.sync.yml`:

```yaml
template:
  use: 'git.example.com/team/module-template@v1.0.0'
ci:
  provider: gitlab
```

CLI overrides `.sync.yml`; an omitted provider is inferred from `origin`.
CI defaults to the selected template's resolved version; `--ci repo/path@version`
selects a different CI source. No fleet, team inventory or separate team checkout is needed.

Repository identity comes from one repository-local `remote.origin.url`.
Runner authentication rewrites stay transport configuration. The configured URL
must be free of passwords, query strings and fragments; multiple origin URLs are
rejected. Host Git/API commands currently require a repository named
`puppet-<module>`, with matching module metadata for lifecycle operations. See the [CI guide](docs/CI-PROVIDERS.md#provider-and-credentials).

## Documentation and AI

These commands work without Docker, an image pull, module access or credentials:

```sh
pmtool help
pmtool docs onboarding --section list
pmtool docs launcher
pmtool ai export --format prompt
pmtool completion bash
pmtool completion zsh
```

AI exports describe the installed launcher version. `--output PATH` exclusively
creates a new file; existing files and symlinks are preserved. No skill is installed
automatically. Read the [manual](docs/manual.md), [workflow](docs/USAGE.md),
[template format](docs/TEMPLATES.md) and [development guide](docs/DEVELOPMENT.md).

## Create a template and inspect CI logs

```sh
pmtool template-init --name team_template --output ./new-template --provider both
pmtool template-test --module ./new-template
pmtool pipeline-jobs --ref main
pmtool pipeline-logs --ref main --failed --tail 100
pmtool pipeline-logs --pipeline 123 --job 456 --tail 0 --output ./job.log
```

`template-init` is local and needs no module, Docker, network or credentials. The
output directory must be new. Its validation starter is editable and has no team
contracts or Forge publishing job; see [template authoring](docs/TEMPLATES.md#create-a-template).
Generated CI pins the stable launcher that generated it; development builds need
an explicit `--launcher-version vX.Y.Z`.

`pipeline-logs` uses the current module's provider token and never modifies Git.
IDs come from `pipeline-jobs`. The default is all selected jobs' last 100 lines;
`--failed` filters failed jobs and `--tail 0` reads full bounded logs. Existing
output files and symlinks are refused. Known tokens are redacted, but review CI
logs for other secrets before sharing them. Failed CI in `mr`/`release-tag` prints
failed-job tails automatically; it still fails and preserves merge/tag guards.

## Release a module

Run each step only after reviewing the preceding result and within the authorized
scope. Update module metadata/changelog after preparation:

```text
release-prepare → sync → edit → check → release-submit → mr → check → release-tag
```

`check` stops at the first failed audit/PDK/VoxBox/release check. `release-submit`
commits then pushes; `mr` waits for exact-head CI, merges and synchronizes main;
`release-tag` waits for exact-main CI, pushes the annotated tag and confirms the
tag publication job. Optional `*-noop` commands remain explicit previews. Cleanup
requires a separate reviewed `cleanup-noop` / `cleanup`; release tags are preserved.

## Execution boundaries

Git/API and release commands run directly in Go using host Git and provider REST
APIs. They require neither Docker nor a Ruby SDK and do not create image config.
Template checkout, sync, general audit and validation planning also run in Go.
The pinned tool image runs the JSON/ERB worker and template-owned Puppet/RSpec
checks. Settings and planning require no Ruby process or helper archive.

PDK/VoxBox containers are started directly by pmtool through the host Docker CLI.
The tool environment never receives a Docker socket. Provider tokens, template
credentials and commit author data are scoped to the individual workflow step;
PDK/VoxBox steps receive none of them. Secret values are never command arguments.

For CI jobs already running in an appropriate tool image, use the same launcher
plan with the explicit native executor:

```sh
pmtool --executor native pdk --stage validate
pmtool --executor native pdk --stage unit
pmtool --executor native voxbox
pmtool --executor native audit
```

Native mode uses installed tools and the current user's filesystem/environment;
it is not a container sandbox. It supports validation and template tests, not
host Git/API commands. A job image must provide the tools required by its selected
check; full `check` requires both PDK and VoxBox. See [CI integration](docs/CI-PROVIDERS.md).

## Repository structure

```text
cmd/pmtool/              executable entry point
internal/pmtool/         CLI, configuration, plans, execution and Go tests
internal/hostflow/       native Git, hosting API, release guards and regressions
internal/templatekit/    template fetch/auth, sync, ownership and general audit
internal/scaffold/       neutral template starter and CI generation
internal/workflow/       embedded command catalog
docs/                   embedded manuals, workflow and AI documentation
test/                   source-release Make target regressions
.github/                launcher build/release workflow and dependency updates
Makefile                build, validation, security and source release targets
Dockerfile.checks        developer Go lint/security tool environment
```

## Develop and publish

```sh
make format
make build VERSION=v1.1.0
```

Build runs Go format/vet/lint/coverage/race checks, security scans, real ERB
integration, strict Ruby-asset lint, cross-compilation and native smoke checks. It uses
containers; host Go/Ruby is not required. `WORKFLOW_IMAGE` defaults to the pinned
upstream VoxBox release for OpenVox 8. The separate `pmtool-checks` development
image supplies Go/lint/security tools and is not the module runtime. The launcher source CI repository variable TOOLING_IMAGE overrides WORKFLOW_IMAGE
for SDK regression tests. It does not change the compiled default selected by IMAGE.

With VERSION=v1.1.0, `dist/v1.1.0/` contains four `pmtool-v1.1.0-<os>-<arch>` binaries, `LICENSE` and
`SHA256SUMS`. `IMAGE` sets the tool image default compiled into the binary.

Source releases use Git and GitHub CLI through Make targets. Start from clean main:

```sh
make release-prepare VERSION=v1.2.3
```

After editing code and `## Unreleased` notes:

```sh
make check
make release-submit VERSION=v1.2.3 MESSAGE='Describe the implementation'
make mr VERSION=v1.2.3
```

Submission creates the implementation commit, a separate `chore(release): prepare
v1.2.3` commit that only finalizes the changelog heading, and pushes the branch
without tags. `mr` creates/reuses the PR, waits for exact-head push CI, merges that
SHA, synchronizes main and downloads/verifies its commit CI artifacts.
There is no VERSION file. Corrective commits on an unmerged release branch keep
the same version; retrying a failed push preserves the existing commits.

Inspect CI and logs without ad hoc API calls:

```sh
make pipeline-jobs
make pipeline-jobs REF=main WATCH=5
make pipeline-logs REF=main TAIL=100
```

`REF` defaults to the current branch; the workflow is pinned to that local ref's
SHA. Optional `RUN_ID` must match it. `JOB` selects a job ID belonging to that run;
`FAILED_ONLY=0` includes successful-job logs. Reading CI never changes Git refs.

Test the downloaded native binary under `dist/ci/<SHA>/` with your template/module.
Then publish the reviewed version:

```sh
make push-release VERSION=v1.2.3
```

The artifact gate, local preflight, exact-main checks, immutable annotated tag and
draft-until-upload publication remain mandatory. `make ci-artifacts` retries
artifact verification; `make push-main` remains available for an already prepared
clean main. Optional `release-prepare-noop` and `release-submit-noop` are read-only
previews. See the [source workflow](docs/DEVELOPMENT.md#versions-and-source-publication)
for separate commit/finalization/push targets and failure recovery.
Build/check/clean never publish. Cleaning distribution files removes CI artifacts,
so download/verify them again before tagging.

## License

MIT License. Copyright (c) 2026 Denys Razumov. See [LICENSE](LICENSE).

## Evaluate standard VoxBox

`make voxbox-check` runs Go template regressions with real ERB and strict Ruby-asset lint and an image security
scan on the pinned OpenVox 8 release of upstream VoxBox, without building our environment.
Image vulnerability findings print `[WARN]` and return success; image download or
scanner failures remain errors. Scanning is explicit and never runs on every pmtool
command. Our Go/source security checks remain mandatory. The active
VoxBox bundle/lock are preserved for template tests. Stock VoxBox is the default
runtime; tests and releases do not build or publish a custom runtime image.
See [evaluation details](docs/DEVELOPMENT.md#stock-voxbox-compatibility-evaluation).

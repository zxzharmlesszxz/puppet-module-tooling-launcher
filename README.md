# pmtool

[Українська](README.uk.md)

Versioned Puppet module workflows for GitLab and GitHub. `pmtool` owns commands,
validation sequences, Git/release guards, terminal guidance and AI instructions.
The pinned upstream [VoxBox](https://github.com/voxpupuli/container-voxbox) image
supplies Ruby and validation dependencies. No custom runtime image is required.
Team templates supply module baselines, contracts and audit policy.

## Use

Install the binary for Linux/macOS, amd64/arm64, as `pmtool`. Docker CLI and a local
Docker daemon are required for validation. Git/API and release operations run in
Go on the host with the installed Git executable. From a module clone:

```sh
pmtool config init
pmtool check
```

The default image is `ghcr.io/voxpupuli/voxbox:10.1.0-openvox8@sha256:01bb8ce5e24d5b111934660f75e2786b4d7c7adcaa7518a564856d1d3041d10d`.
Pin an image digest or release in `~/.config/puppet-module-tooling/config.json`
when reproducibility is required. Image and launcher versions are independent.
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
selects a different CI source. No inventory, fleet or team checkout is needed.

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
Git/API mutations. A job image must provide the tools required by its selected
check; full `check` requires both PDK and VoxBox. See [CI integration](docs/CI-PROVIDERS.md).

## Repository structure

```text
cmd/pmtool/              executable entry point
internal/pmtool/         CLI, configuration, plans, execution and Go tests
internal/hostflow/       native Git, hosting API, release guards and regressions
internal/templatekit/    template fetch/auth, sync, ownership and general audit
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
make build VERSION=v1.0.0
```

Build runs Go format/vet/lint/coverage/race checks, security scans, real ERB
integration, strict Ruby-asset lint, cross-compilation and native smoke checks. It uses
containers; host Go/Ruby is not required. `WORKFLOW_IMAGE` defaults to the pinned
upstream VoxBox release for OpenVox 8. The separate `pmtool-checks` development
image supplies Go/lint/security tools and is not the module runtime. CI uses the
same VoxBox default; `TOOLING_IMAGE` is an optional explicit override.

`dist/v1.0.0/` contains four `pmtool-v1.0.0-<os>-<arch>` binaries, `LICENSE` and
`SHA256SUMS`. `IMAGE` sets the tool image default compiled into the binary.

Commit implementation changes with the changelog under `## Unreleased`. Prepare
the release in a separate commit: finalize that heading as `## Release 1.0.0` and
commit the release-only diff as `chore(release): prepare v1.0.0`. There is no VERSION
file; the binary version still comes from the build parameter/Git tag.

After both local commits, publish the prepared clean main commit first:

```sh
make push-main
```

This pushes main without tags, waits for its exact-SHA CI, downloads the
`pmtool-dev-<commit>` Actions artifact and verifies every checksum, LICENSE,
source/run identity and the native binary's version/help. Branch builds use
`dev-<full commit SHA>`; they do not create a GitHub Release or prerelease.
Verified binaries are kept in `dist/ci/<commit>/`. Test the downloaded native
binary with your team template and module before choosing the stable release.
`make ci-artifacts` repeats download/verification without pushing main.

After that review, publish the stable version:

```sh
make push-release VERSION=v1.0.0
```

This requires the verified commit artifacts and synchronized clean main, runs
local preflight, creates/pushes the annotated stable tag and waits for its release
workflow. It does not push main. Direct tag targets enforce the same artifact gate.
Tag CI checks current main and its successful commit CI before building. GitHub
publication creates a draft and makes it public only after asset uploads succeed.
There is no separate `rc.N` workflow. Preserve a tag after interrupted publication;
inspect/retry the failed workflow, then use `push-release-tag` for the identical tag.
Build/check/clean never publish; clean preserves Docker images and caches and
removes downloaded CI artifacts along with the generated distribution files.

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

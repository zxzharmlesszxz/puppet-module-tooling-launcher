# pmtool manual

[Українська](manual.uk.md)

## Responsibilities

`pmtool` owns the workflow, checks, command permissions, guidance and documentation.
Its binary contains the Go workflow, command catalog and minimal Ruby rendering assets. The tool image supplies
executables/dependencies only. Team templates supply contracts and audit policy.

Git/API and release operations are implemented in Go and use the host Git
executable plus GitLab/GitHub REST APIs. They need no Docker, Ruby SDK or image
configuration. Template checkout, sync, audit and planning are Go-owned. Puppet parsing and ERB use Ruby
in the selected tool environment. No launcher source is baked into that image.
See [native workflow guarantees](GO-WORKFLOW.md) for guards and recovery.

## Local commands

Help, docs, command completion and AI exports need no Docker, image, module, token
or socket. They do not create host configuration:

```sh
pmtool help
pmtool help runtime
pmtool docs usage
pmtool docs onboarding --section list
pmtool docs onboarding --section 'Release'
pmtool manifest
pmtool version
pmtool ai help
pmtool ai export --format agents
pmtool ai export --format skill --output ./new-skill/SKILL.md
pmtool completion bash
pmtool completion zsh
```

`docs DOCUMENT --section list` lists headings; a title fragment selects a unique
section. Terminal output wraps prose and highlights code; redirected output keeps
raw Markdown. AI export writes raw Markdown and refuses existing files/symlinks.
The export is guidance, not authorization to commit, push or publish.

## Configuration

The host file is `~/.config/puppet-module-tooling/config.json`, with 0700 directory
and 0600 file permissions. `config init` preserves an existing file; the first
Docker operation also creates a missing configuration.

```json
{
  "image": "ghcr.io/voxpupuli/voxbox:10.1.0-openvox8@sha256:01bb8ce5e24d5b111934660f75e2786b4d7c7adcaa7518a564856d1d3041d10d",
  "docker": "docker"
}
```

```sh
pmtool config init
pmtool config init --image registry.example.com/tools:v1.0.0
pmtool config path
pmtool config show
```

The default image is compiled into the binary; `--image` overrides one invocation.
`--config` selects another host file. CI/provider/template settings and credentials
do not belong here. The old `socket` field / `--socket` option are accepted for
existing configurations but never cause a socket mount.

## Command syntax

Global flags precede the command; operation flags follow it:

```sh
pmtool --image tools:local --dry-run check --module /absolute/module
pmtool --color always status
pmtool --executor native pdk --stage unit
```

Global flags: `--image`, `--config`, `--docker`, `--executor docker|native`,
`--color auto|always|never`, `--dry-run`, `--help`, `--version`.
Operation flags: `--module`, `--branch`, `--ref`, `--title`, `--watch`, `--file`,
`--files`, `--version`, `--ci`, `--provider`, `--template`, `--pdk-image`,
`--voxbox-image`, `--stage`; documentation uses `--section`.
`--host-module` is internal and cannot be supplied by callers. `--ci-project`
remains a legacy GitLab compatibility flag; prefer a complete versioned `--ci`.

Use `pmtool manifest` to inspect this launcher's exact command/option catalog.
`--dry-run` prints planned steps without executing them. In Docker mode it may
print the Go plan without resolving/pulling an image or fetching templates; it performs no commit,
push, merge, tag or publication.

## Module selection and templates

The default module is the current clone. `--module /absolute/clone` selects another
physical directory. Git lifecycle operations require an ordinary `.git` directory;
linked worktrees are not supported. Repository identity comes from origin, not
the checkout folder name. Never reuse another project's token or namespace.

```yaml
template:
  use: 'git.example.com/team/module-template@v1.0.0'
ci:
  provider: gitlab
  use: 'git.example.com/team/pipelines//ci/module.yml@v1.0.0'
```

CLI overrides module `.sync.yml`. An omitted provider comes from origin; select
`--provider github` for a GitHub Enterprise host. Omit `ci.use` to use CI from the
same resolved template version. Host config has no fleet, inventory or per-module
sections. Read `docs templates` and `docs ci` for references and policy boundaries.

## Permissions and tools

Settings, plans and hints are computed in Go without a container. Template checkout,
sync and general audit also execute in Go. The Docker executor resolves the tools
image once to an image ID for ERB and template-owned checks. ERB exchanges JSON
through stdin/stdout; no launcher helper archive is extracted. Module checks use
read-only physical-path mounts and the invoking user's UID/GID. No worker receives
provider/template tokens or a Docker socket. Git safe.directory is clone-scoped.

Each mutation gets only its required credentials. Remote operations use the
selected provider's `GITLAB_TOKEN` or `GITHUB_TOKEN`; commit/tag author comes from
`GIT_AUTHOR_NAME`/`GIT_AUTHOR_EMAIL` or host Git configuration. A provider token
does not automatically authenticate template downloads.

Private HTTPS templates use explicit `TOOLING_TEMPLATE_TOKEN`,
`TOOLING_TEMPLATE_TOKEN_HOST` and optional `TOOLING_TEMPLATE_USERNAME`. Forwarding
requires the exact approved host and a template-consuming operation. Credentials
are environment values, never arguments, documentation or module configuration.
SSH template authentication is not implemented.

PDK/VoxBox images are launched directly by the host pmtool. No tool container
receives a Docker socket. Validator steps receive no provider/template token or
commit author. Default tool versions remain pinned; explicit `--pdk-image` and
`--voxbox-image` overrides are available.

## Native executor for CI

`--executor native` uses tools already installed in the current CI job environment.
It runs the same launcher-owned plan, stops at the same failure and preserves exit
codes. It requires neither Docker CLI nor Docker socket. It does not create host
configuration or resolve an image.

```sh
pmtool --executor native pdk --stage validate
pmtool --executor native pdk --stage unit
pmtool --executor native voxbox --stage validate
pmtool --executor native voxbox --stage unit
pmtool --executor native audit
pmtool --executor native release-checks
pmtool --executor native template-test --module /template
```

`--stage validate|unit` selects one PDK/VoxBox phase; omission runs both. The tool
image must provide the requested executables and bundles. Full `check` requires
both toolchains. Native mode is explicit, uses the job user's filesystem and
environment, and offers no container filesystem/credential isolation of its own.
Native mode refuses Git/API mutation commands. Use regular pmtool commands
without --executor native for interactive releases; their Go backend uses host Git
and REST directly, without starting Docker.

## Bootstrap

```text
policy-noop → review → policy → bootstrap-prepare → optional PDK conversion
→ migrate → sync → edit → check → bootstrap-commit → bootstrap-push → mr
→ check → release-tag → verify Forge/canary → reviewed cleanup
```

Prepare with an explicit `--branch bootstrap/<purpose>`. Main must be created from
the intended original history before development. Conversion preview can write a
report; applying conversion is separate. Fix generated metadata, dependencies,
fixtures and namespace before continuing. Existing baseline conflicts require
reviewed `--files`; custom spec settings belong in `spec_helper_local.rb`.

## Module releases and recovery

```text
release-prepare → sync → edit → check → release-submit → mr → check → release-tag
```

`release-prepare --version X.Y.Z` synchronizes clean main and creates the release
branch. Update metadata, matching changelog, implementation, tests and README.
`check` runs audit → PDK validation/unit → VoxBox validation/specs → release checks.

`release-submit` commits then pushes the branch. If push fails, preserve the commit
and retry `release-push`. `mr` creates/reuses the MR, waits for its exact-head push
pipeline, merges that SHA and synchronizes local main. If synchronization fails,
preserve the merge and retry `main-sync`. After combined mr, run check directly.

`release-tag` validates exact-main CI, creates/pushes an annotated tag without
force and waits for its exact tag/SHA publication pipeline and forge-publish job.
Changed refs, missing/failed/canceled/skipped/manual CI and API errors stop release.
If publication waiting fails, preserve the tag and inspect `pipeline-jobs --ref
vX.Y.Z --watch 5`. Do not recreate/move a pushed tag. Forge and a canary installation
remain the final publication evidence.

`*-noop` commands are optional previews/diagnostics, not mandatory extra steps.
`sync` changes the local baseline only. Standalone commit commands never push.
`pipeline-jobs` is optional inspection because mr/release-tag already wait.

## Cleanup

After verified publication and canary, from clean synchronized main:

```sh
pmtool cleanup-noop --branch release/X.Y.Z
pmtool cleanup --branch release/X.Y.Z
```

Review the preview before apply. Cleanup requires consistent merged/published
ancestry, successful publication and an unprotected branch. It preserves all tags,
Forge artifacts and unrelated branches. No bulk prune or automatic cleanup occurs.

## Development and source publication

Read `docs/DEVELOPMENT.md` for source layout, embedded assets and tests.

```sh
make format
make build VERSION=v1.0.0
```

Build includes full Go checks/security, real ERB checks and Ruby-asset lint, four platform binaries,
LICENSE/checksums and native smoke checks. Source releases use canonical Git tags
and the latest `Release X.Y.Z` changelog heading; there is no VERSION file.
Keep reviewed implementation changes under `## Unreleased` and commit them first.
Create a separate preparation commit by finalizing the heading as
`## Release 1.0.0`, with message `chore(release): prepare v1.0.0`. Commit only the
release-preparation diff there; no VERSION file is introduced.

After both local commits, start the CI phase for the prepared clean main:

```sh
make push-main
```

It pushes main without tags, waits for exact-SHA CI, downloads the matching
`pmtool-dev-<full SHA>` Actions artifact and verifies its five checksums, LICENSE,
source/run identity and native version/help. Binaries are stored in `dist/ci/<SHA>/`.
Test the downloaded native binary with your team template/module. Branch CI does
not create a GitHub Release or prerelease. `make ci-artifacts` downloads/verifies
again without pushing.

After reviewing that candidate, publish the stable version:

```sh
make push-release VERSION=v1.0.0
```

This requires verified artifacts for the current clean synchronized main, runs
preflight, creates/pushes the annotated stable tag and waits for publication.
It does not push main. Direct tag targets require the same artifact verification.
Tag CI checks current main and successful commit CI, then publishes through a
GitHub draft; only completed uploads are made public. No `rc.N` workflow is used.
Preserve a tag after interrupted publication, inspect/retry the failed workflow,
then use `push-release-tag` for that identical tag. Build/check/clean never publish.

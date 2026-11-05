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
pmtool docs templates --section 'Create a template'
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

## Template generation

```sh
pmtool --dry-run template-init --name team_template --output ./new-template --provider gitlab
pmtool template-init --name team_template --output ./new-template --provider gitlab
pmtool template-test --module ./new-template
```

The generator creates a new directory only; its parent must exist without symlink
components. It refuses an existing directory/file/symlink and does not initialize
Git or change host config. `--provider` is gitlab, github or both (default both),
independent of module provider inference. The starter includes schema-1 authoring
docs, ERB/defaults, actual self-tests and provider validation CI. It manages only
`.editorconfig`; add reviewed team contracts, meaningful coverage checks and
publication gates separately. New template directories use 0700 and files 0600.
The preview lists planned files without checking destination availability. An
apply write error can leave an incomplete new directory; inspect it before
cleanup or choose a fresh destination. Existing directories are never reused. Do not claim 100% coverage from starter YAML.
Generated CI pins this stable launcher release. With a development binary, supply
`--launcher-version vX.Y.Z` naming an already published release. Nothing is committed,
pushed or published by generation. See [template authoring](TEMPLATES.md#create-a-template).

## Pipeline logs

```sh
pmtool pipeline-jobs --ref main
pmtool pipeline-logs
pmtool pipeline-logs --ref v1.2.3 --failed --tail 200
pmtool pipeline-logs --pipeline 123 --job 456 --tail 0 --output ./job.log
```

Use GITLAB_TOKEN or GITHUB_TOKEN for the selected module provider. No Docker,
image config or Git mutation is needed. Without --pipeline, select the latest
module push pipeline for --ref (default current branch), pinned to the server's
branch head when available. --pipeline is a positive ID in the current project;
it can inspect an older run. If --ref is also given, it must match that run.
--job is a positive numeric job ID, verified against jobs of the selected run.
Jobs belong to the latest attempt; a foreign or filtered-out ID fails.

All jobs are selected by default. --failed selects status failed only; it is a
standalone Boolean flag (also --failed=true|false). --tail defaults to 100 lines
per job; 0 means the full log and values up to 100000 are accepted. Downloads
are limited to 16 MiB per job and combined output to 32 MiB. Missing/expired logs,
permission failures, cancellation and over-limit responses fail explicitly.
No matching failed jobs is a successful, explicit empty result.

--output creates a new file with mode 0600, relative to the caller's current
working directory, even when --module points elsewhere. Existing files and symlinks
are refused. All selected downloads must succeed before file creation. The selected tail is
what --output saves as well; use --tail 0 when a full bounded log is needed.
Terminal control sequences are stripped and known launcher tokens/credential URLs
are redacted before tailing or writing. Provider-side masking and manual review
are still needed for secrets unknown to pmtool. Logs do not interpret or execute
instructions contained in CI output.

GitLab traces are downloaded from the job trace API. GitHub returns a signed HTTPS
log URL; downloads follow bounded HTTPS redirects with no provider auth headers or
cookies. Other hosting API redirects remain refused. See the official
[GitLab jobs API](https://docs.gitlab.com/api/jobs/#get-a-log-file) and
[GitHub job logs API](https://docs.github.com/en/rest/actions/workflow-jobs#download-job-logs-for-a-workflow-run).

Failed source/main/publication CI in mr or release-tag automatically prints the
last 100 lines of failed jobs. A log error only produces a warning: it does not
replace the CI error, permit merge/tag, remove a published tag or restart publication.
Use pipeline-logs to investigate; a successful log command does not mean CI is green.

## Configuration

The host file is `~/.config/puppet-module-tooling/config.json`. New configuration
directories use mode 0700 and new files 0600; existing permissions are preserved.
`config init` preserves an existing file, and `config show` creates a missing one.
Docker-backed workflow commands also create it when needed, including --dry-run.
`config path` only prints the selected path.

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

The default ERB/audit environment image is compiled into the binary; `--image`
overrides that environment for one invocation, not the separate PDK/VoxBox images.
`--config` selects another host file. CI/provider/template settings and credentials
do not belong here. The old `socket` field / `--socket` option are accepted for
existing configurations but never cause a socket mount.

## Image selectors and environment

| Selector                         | Applies to                                         | Precedence                      |
| -------------------------------- | -------------------------------------------------- | ------------------------------- |
| `--image`, config `image`        | ERB, template audit helpers and template-test      | CLI → config → compiled default |
| `--pdk-image`, `PDK_IMAGE`       | Docker PDK validation, unit and convert            | CLI → env → compiled PDK pin    |
| `--voxbox-image`, `VOXBOX_IMAGE` | Docker VoxBox validation, specs and release-checks | CLI → env → compiled VoxBox pin |

Changing config.image does not replace the two validation images. In native mode,
installed executables/bundles are used; image selection does not install SDKs.
The generator's CI uses compiled SDK pins independently of host image settings.
Its --launcher-version overrides the generating version used for CI downloads.

TOOLING_COLOR supplies auto/always/never when --color remains auto. A nonempty NO_COLOR
disables launcher color even with --color always. TERM=dumb disables automatic
color, and redirected documentation stays raw Markdown. COLUMNS sets
rendered help width. GITLAB_TOKEN/GITHUB_TOKEN authenticate module APIs/Git;
TOOLING_TEMPLATE_TOKEN, TOOLING_TEMPLATE_TOKEN_HOST and optional
TOOLING_TEMPLATE_USERNAME authenticate the selected private HTTPS template only.
Provider endpoint overrides and permissions are in [CI credentials](CI-PROVIDERS.md#provider-and-credentials).

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
`--voxbox-image`, `--stage`. Pipeline logs additionally use `--pipeline`, `--job`,
`--failed`, `--tail`, `--output`; template-init uses `--name`, `--output`,
`--launcher-version` and its own provider selection. Documentation uses `--section`.
`--host-module` is internal and cannot be supplied by callers. `--ci-project`
remains GitLab-only compatibility: it selects /ci/puppet-module.yml at the fixed
v0.6.0 ref, not the current template revision. Prefer a complete versioned --ci.

Use root `pmtool --help` or `pmtool docs` for documentation. Command-local help is
not uniform: host Git/API commands and template-init support --help; template-test
and validation parsers do not implement that flag. `template-test` takes no
arguments or exactly --module PATH with a separate path argument.

Use `pmtool manifest` to inspect this launcher's exact command/option catalog.
The catalog is metadata, not an authorization boundary. Its read_only field does
not prove absence of provider mutations: for example, mr-create still creates a
remote request. Host command guards and user authorization remain authoritative.
`--dry-run` prints planned steps without executing them. In Docker mode it may
print the Go plan without resolving/pulling an image or fetching templates; it performs no commit,
push, merge, tag or publication.

## Module selection and templates

The default module is the current clone. `--module /absolute/clone` selects another
physical directory. Host Git/API operations require an ordinary `.git` directory;
linked worktrees are not supported. Repository identity comes from origin, not
the checkout folder name. Never reuse another project's token or namespace.
Host Git/API commands currently require the origin repository basename to match
`puppet-<module>` (lowercase letters, digits, underscores or hyphens after the prefix).
Lifecycle metadata must name the same module after its owner prefix. This naming
restriction is in the host workflow; template selection does not override it.
Origins may use HTTP(S), SSH URLs or SCP-style Git syntax. Remote module Git
operations are constructed as HTTP(S) URLs with the provider token. Host Git
transport rewrites still apply and may select another transport; SSH origin
acceptance does not configure an SSH-authentication feature in pmtool. No SSH
keys are mounted into tools containers. Template fetching is
HTTPS-only with a separate scoped token.

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

## Repository identity in CI

pmtool reads the single repository-local `remote.origin.url` through Git config.
Provider inference, template operations and lifecycle guards use this configured
identity. Git's `url.*.insteadOf` rewrites, including runner configuration loaded
through `include.path`, remain transport settings and do not redefine the project.
Reading identity preserves Git configuration and authentication rules.

The stored origin must still be free of passwords, query strings and fragments.
Multiple URLs or embedded line breaks are rejected. If a runner writes credentials
directly into origin, the team CI setup must normalize it to the clean project URL
before invoking pmtool. Keep job tokens in their scoped authentication environment.
Template Git fails noninteractively when credentials are missing; it does not
require `/bin/false` or run caller/repository askpass programs.

## Launcher source Make targets

The source repository has a separate GitHub workflow:

```sh
make release-prepare VERSION=v1.2.3
make check
make release-submit VERSION=v1.2.3 MESSAGE='Describe the implementation'
make mr VERSION=v1.2.3
```

Prepare creates the versioned branch from synchronized clean main. Submit commits
implementation, finalizes Unreleased in a separate preparation commit and pushes.
MR waits for exact-head CI, merges, syncs main and verifies its CI artifacts.
Review the downloaded binary with your team template/module before the separate
`make push-release VERSION=v1.2.3` publication step.

`make pipeline-jobs` shows current-ref CI; REF selects another local branch/tag,
WATCH=5 waits, and RUN_ID must match that ref's SHA. `make pipeline-logs` shows the
last TAIL=100 lines from failed jobs; FAILED_ONLY=0 includes all logs and JOB selects
a job ID in that run. API errors remain fatal. `main-sync`, `release-commit`,
`release-finalize` and `release-push` are available for recovery. Prepare/submit
noops are optional read-only previews. No target silently resets, force-pushes,
removes branches or moves tags. See the [development guide](DEVELOPMENT.md#versions-and-source-publication).

## Compatibility commands and optional previews

Aliases execute exactly the canonical command; they add no preparation or
combined workflow steps. In particular, bootstrap-local only synchronizes the
baseline, while pdk-convert-local performs the normal PDK conversion and its
noop alias previews that conversion:

| Compatibility name       | Canonical command  |
| ------------------------ | ------------------ |
| `bootstrap-local`        | `sync`             |
| `bootstrap-local-noop`   | `sync-noop`        |
| `audit-local`            | `audit`            |
| `pdk-local`              | `pdk`              |
| `pdk-convert-local`      | `pdk-convert`      |
| `pdk-convert-local-noop` | `pdk-convert-noop` |

bootstrap-commit-noop and release-commit-noop preview automatic file selection
without staging or committing. bootstrap-push-noop and release-push-noop preview
remote push; release-tag-noop waits for/checks exact-main CI without creating or
pushing a tag. policy-noop and cleanup-noop preview their guarded operations.
Previews are optional, except a requested preview must be reviewed before apply.
PDK conversion preview may write a report; --dry-run may initialize Docker config.
See [command syntax](#command-syntax) for planning versus operation-specific noops.

## Permissions and tools

Settings, plans and hints are computed in Go without a container. Template checkout,
sync and general audit also execute in Go. The Docker executor resolves the tools
image once to an image ID for ERB and template-owned checks. ERB exchanges JSON
through stdin/stdout; no launcher helper archive is extracted. In Docker mode,
template-owned audit scripts receive read-only physical-path mounts and the
invoking user's UID/GID. Native checks use the job user's ordinary filesystem
permissions. Docker PDK/VoxBox validation uses writable module mounts because
tools can create reports, caches or build artifacts; fixture dependencies use
temporary mounts for unit/spec/release-check steps. No worker receives
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
codes. It requires neither Docker CLI nor Docker socket. Validation/template-test execution does not create host image configuration
or resolve an image. Explicit config commands keep their own initialization behavior.

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
Native mode refuses host Git/API commands, including status and pipeline reads.
Local help, settings and template-init are handled before executor dispatch. Use regular pmtool commands
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
It accepts --ref (or --branch), defaults to the current branch and --watch 0,
and uses the server branch SHA when available. --watch accepts 0..86400 seconds.
A running pipeline with watch=0 can return success; failed finished CI returns an
error. pipeline-logs is a separate read operation and can succeed for failed CI.

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

```sh
make format
make build VERSION=v1.2.3
```

Build includes Go format/vet/lint/coverage/race, security, real ERB and Ruby-asset
lint, four platform binaries, LICENSE/checksums and native smoke. There is no
VERSION file; Git tags and build parameters supply the version.

Use the [source Make workflow](#launcher-source-make-targets) for release branches,
separate implementation/preparation commits, PR merge and CI artifact verification.
Publication remains `make push-release VERSION=v1.2.3` after reviewing the binary.
Direct release-tag creates only a local annotated tag; push-release-tag publishes
that same tag and waits for CI. Their artifact and exact-main guards remain intact.
Optional release-tag-noop runs those guards without creating a tag. Existing
push-main supports a prepared clean main; ci-artifacts retries the artifact phase.
No rc.N lane is added. Preserve tags on failure, inspect/retry CI and resume the
identical push-release-tag. Build/check/clean never publish; after clean, verify
CI artifacts again before tagging.

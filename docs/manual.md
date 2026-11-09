# pmtool manual

[Українська](manual.uk.md)

## Responsibilities

`pmtool` owns the workflow, checks, command permissions, guidance and documentation.
Its binary contains the Go workflow, command catalog and minimal Ruby rendering assets. The tool image supplies
executables/dependencies only. Team templates supply contracts and audit policy.

Git/API and release operations are implemented in Go and use the host Git
executable and the provider APIs. GitHub review decisions use GraphQL. These
operations need no Docker, Ruby SDK or image configuration. Template checkout, sync, audit and planning are Go-owned. Puppet parsing and ERB use Ruby
in the selected tool environment. No launcher source is baked into that image.
See [native workflow guarantees](GO-WORKFLOW.md) for guards and recovery.

## Local commands

Help, docs, command completion and AI exports need no Docker, image, module, token
or socket. They do not create host configuration:

```sh
pmtool help
pmtool docs
pmtool docs tokens
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
Git unless --origin URL is supplied; host image config is never changed.
With --origin it creates an unborn main branch and adds that remote locally,
without commits, pushes, authentication or a network request. The hosted repository
must be created separately. `--provider` is gitlab, github or both (default both),
independent of module provider inference. The starter includes schema-1 authoring
docs, ERB/defaults, actual self-tests and provider validation CI. It manages only
`.editorconfig`; add reviewed team contracts, meaningful coverage checks and
publication gates separately. New template directories use 0700 and files 0600.
The preview lists planned files without checking destination availability. An
apply write error can leave an incomplete new directory; inspect it before
cleanup or choose a fresh destination. Existing directories are never reused. Do not claim 100% coverage from starter YAML.
Generated CI pins this stable launcher release. With a development binary, supply
`--launcher-version vX.Y.Z` naming an already published release. Nothing is committed,
pushed or published by generation. Create the template's hosted repository;
use --origin URL to initialize Git/main and connect it during generation, or add
the remote manually when omitted. Preview includes Origin without requiring Git.
Invalid URLs fail before writing; Git setup failure preserves generated files
and any partial repository for inspection. After reviewed
commits, push main, wait for CI and publish a version tag before selecting it in
modules. See [repository setup](TEMPLATES.md#connect-and-publish-the-repository).

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
are redacted before tailing or writing. Environment-token redaction covers
GITLAB_TOKEN, GITHUB_TOKEN, GH_TOKEN, CI_JOB_TOKEN, TOOLING_TEMPLATE_TOKEN and
FORGE_PUBLISH_TOKEN and FORGE_TOKEN, plus the selected Git credential. Provider-side masking and manual review
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

## Doctor

```sh
pmtool doctor
pmtool doctor --module /path/to/puppet-example --template 'git.example.org/team/template@v2.0.3'
```

Doctor reports origin, template, VoxBox and Forge separately. It returns 1 if a
check fails, or 130 on interruption. It derives the provider and project from
the configured origin and reads the selected template with the usual Git helper
or explicit template token override. An origin check needs no hosting API token.
Use `--provider` or `--template` to override module settings. An invalid `.sync.yml`
fails the origin and template checks while VoxBox and Forge are still checked.

With Docker it checks the daemon and both local image selections without
pulling. The baseline image uses global `--image` or the launcher config. The
validation image uses `--voxbox-image`, then `VOXBOX_IMAGE`, then the built-in pin. With
`--executor native` it checks the VoxBox bundle and Rakefile. Forge checks use
`FORGE_URL` and the read-only `FORGE_TOKEN` when configured. A module without
Forge fixtures skips the Forge check. Doctor reads release catalogs and probes
the gzip header of archives for exact versions. For transitive fixtures, it reads
dependencies reported by exact release metadata. Unpinned and range references
are checked at the catalog level. Run `pmtool check` for actual dependency
resolution, complete archive downloads and module validation.

Doctor does not write the module or launcher configuration. Use global
`--dry-run` to print the checks without contacting external services.

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
overrides that environment for one invocation, not the separate VoxBox validator/jig image.
`--config` selects another host file. CI/provider/template settings and credentials
do not belong here. The old `socket` field / `--socket` option are accepted for
existing configurations but never cause a socket mount.

## Image selectors and environment

| Selector                         | Applies to                                            | Precedence                      |
| -------------------------------- | ----------------------------------------------------- | ------------------------------- |
| `--image`, config `image`        | ERB, template audit helpers and template-test         | CLI → config → compiled default |
| `--voxbox-image`, `VOXBOX_IMAGE` | Docker VoxBox validation/specs/release-checks and jig | CLI → env → compiled VoxBox pin |

Changing config.image does not replace the validator image. In native mode,
installed executables/bundles are used; image selection does not install SDKs.
The generator's CI uses compiled SDK pins independently of host image settings.
Its --launcher-version overrides the generating version used for CI downloads.

TOOLING_COLOR supplies auto/always/never when --color remains auto. A nonempty NO_COLOR
disables launcher color even with --color always. TERM=dumb disables automatic
color, and redirected documentation stays raw Markdown. COLUMNS sets
rendered help width. GITLAB_TOKEN/GITHUB_TOKEN authenticate module APIs/Git;
Saved Git credential helpers authenticate template HTTPS automatically.
TOOLING_TEMPLATE_TOKEN with TOOLING_TEMPLATE_TOKEN_HOST is an explicit override;
TOOLING_TEMPLATE_USERNAME is optional for that override.
Provider endpoint overrides and permissions are in [CI credentials](CI-PROVIDERS.md#provider-and-credentials).

## Command syntax

Global flags precede the command; operation flags follow it:

```sh
pmtool --image tools:local --dry-run check --module /absolute/module
pmtool --color always status
```

Global flags: `--image`, `--config`, `--docker`, `--executor docker|native`,
`--color auto|always|never`, `--dry-run`, `--help`, `--version`.
Operation flags: `--name` (module-new), `--module`, `--branch`, `--ref`, `--title`, `--watch`, `--file`,
`--files`, `--version`, `--ci`, `--provider`, `--template`, `--voxbox-image`, `--stage`. Pipeline logs additionally use `--pipeline`, `--job`,
`--failed`, `--tail`, `--output`; template-init uses `--name`, `--output`,
`--launcher-version`, `--origin` and its own provider selection. Documentation uses `--section`.
`--host-module` is internal and cannot be supplied by callers. `--ci-project`
remains GitLab-only compatibility: it selects /ci/puppet-module.yml at the fixed
v0.6.0 ref, not the current template revision. Prefer a complete versioned --ci.

Use root `pmtool --help` or `pmtool docs` for documentation. Command-local help is
not uniform: host Git/API commands, doctor and template-init support --help; template-test
and validation/jig/forge-publish parsers do not implement that flag. `template-test` takes no
arguments or exactly --module PATH with a separate path argument.

--files is one whitespace-separated string of module-relative paths, not a
shell-parsed list; quoting the whole argument does not support filenames containing
spaces. Explicit commit selections accept only ASCII letters/digits, underscore,
dot, slash and hyphen, with no leading hyphen or traversal. Automatic commit
selection preserves spaces and Unicode; omit --files for such names. A restricted
commit selection must still cover all changed files; it cannot silently omit
unrelated changes. In sync, --files approves reviewed baseline replacements/removals.

Use `pmtool manifest` to inspect the workflow catalog. Root flags and local
interfaces such as config, ai and completion have their own parsing; the manifest
is not a complete CLI grammar.
The catalog describes command effects. The `read_only` flag covers changes to
checkout files, Git refs and hosting state. Host config initialization is handled
separately. The `token` field describes provider authentication. `forge-publish`
uses its separate Forge token. Runtime guards and user authorization still apply.
`--dry-run` prints planned steps without executing them. In Docker mode it may
print the Go plan without resolving/pulling an image or fetching templates; it performs no commit,
push, merge, tag or publication.

## Module selection and templates

The default module is the current clone. `--module /absolute/clone` selects another
physical directory. Host Git/API operations require an ordinary `.git` directory;
linked worktrees are not supported. Repository identity comes from origin, not
the checkout folder name. Never reuse another project's token or namespace.
Module Git/API commands require the origin repository basename to match
`puppet-<module>` (lowercase letters, digits, underscores or hyphens after the prefix).
MR commands, main-sync, pipeline reads, status and diff also accept an ordinary
template checkout with a tracked schema-1 `template.yml` and no `metadata.json`.
See [the template request flow](TEMPLATES.md#connect-and-publish-the-repository).
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
make release-prepare VERSION=v2.0.0
make check
make release-submit VERSION=v2.0.0 MESSAGE='Describe the implementation'
make mr VERSION=v2.0.0
```

Prepare creates the versioned branch from synchronized clean main. Submit commits
implementation, finalizes Unreleased in a separate preparation commit and pushes.
MR waits for exact-head CI, merges, syncs main and verifies its CI artifacts.
Review the downloaded binary with your team template/module before the separate
`make push-release VERSION=v2.0.0` publication step.

`make pipeline-jobs` shows current-ref CI; REF selects another local branch/tag,
WATCH=5 waits, and RUN_ID must match that ref's SHA. `make pipeline-logs` shows the
last TAIL=100 lines from failed jobs; FAILED_ONLY=0 includes all logs and JOB selects
a job ID in that run. API errors remain fatal. `main-sync`, `release-commit`,
`release-finalize` and `release-push` are available for recovery. Prepare/submit
noops are optional read-only previews. No target silently resets, force-pushes,
removes branches or moves tags. See the [development guide](DEVELOPMENT.md#versions-and-source-publication).

## Compatibility commands and optional previews

Aliases execute exactly the canonical command; they add no preparation or
combined workflow steps. bootstrap-local only synchronizes the baseline:

| Compatibility name     | Canonical command |
| ---------------------- | ----------------- |
| `bootstrap-local`      | `sync`            |
| `bootstrap-local-noop` | `sync-noop`       |
| `audit-local`          | `audit`           |

bootstrap-commit-noop and release-commit-noop preview automatic file selection
without staging or committing. bootstrap-push-noop and release-push-noop preview
remote push; release-tag-noop waits for/checks exact-main CI without creating or
pushing a tag. policy-noop and cleanup-noop preview their guarded operations.
Previews are optional, except a requested preview must be reviewed before apply.
module-convert-noop uses jig convert --dry-run; launcher --dry-run may initialize Docker config.
See [command syntax](#command-syntax) for planning versus operation-specific noops.

## Permissions and tools

Settings, plans and hints are computed in Go without a container. Template checkout,
sync and general audit also execute in Go. The Docker executor resolves the tools
image once to an image ID for ERB and template-owned checks. ERB exchanges JSON
through stdin/stdout; no launcher helper archive is extracted. In Docker mode,
template-owned audit scripts receive read-only physical-path mounts and the
invoking user's UID/GID. Native checks use the job user's ordinary filesystem
permissions. Docker VoxBox validation uses writable module mounts because
tools can create reports, caches or build artifacts; fixture dependencies use
temporary mounts for unit/spec/release-check steps. No worker receives
provider/template tokens or a Docker socket. Git safe.directory is clone-scoped.

Each mutation gets only its required credentials. Remote operations use the
selected provider's `GITLAB_TOKEN` or `GITHUB_TOKEN`; commit/tag author comes from
`GIT_AUTHOR_NAME`/`GIT_AUTHOR_EMAIL` or host Git configuration. A provider token
does not automatically authenticate template downloads.

Private HTTPS templates resolve an explicit host-scoped `TOOLING_TEMPLATE_TOKEN`
override first, then saved host Git helper credentials, then anonymous access.
`TOOLING_TEMPLATE_USERNAME` is optional for the explicit override. Host helper
lookup is noninteractive and scoped to the selected URL; fetch binds the result
to that exact repository without persisting it. Secrets never enter argv, Git
config, SDK workers or module files. See [template authentication](TEMPLATES.md#private-template-repositories).
SSH template authentication is not implemented.

VoxBox images are launched directly by the host pmtool. No tool container
receives a Docker socket. Validator steps receive no provider/template token or
commit author. Default tool versions remain pinned; explicit `--voxbox-image` overrides are available.

## Native executor for CI

`--executor native` uses tools already installed in the current CI job environment.
It runs the same launcher-owned plan, stops at the same failure and preserves exit
codes. It requires neither Docker CLI nor Docker socket. module-build supports
native execution with installed jig; module-new and conversion require Docker.
Validation/template-test/module-build execution does not create host image
configuration or resolve an image. Explicit config commands keep their own initialization behavior.

```sh
pmtool --executor native voxbox --stage validate
pmtool --executor native voxbox --stage unit
pmtool --executor native audit
pmtool --executor native release-checks
pmtool --executor native template-test --module /template
pmtool --executor native module-build
```

`--stage validate|unit` selects one VoxBox phase; omission runs both. Validation runs all three Rake tasks: validate, lint and rubocop. The tool
image must provide the requested executables and bundles. Full `check` requires
the VoxBox toolchain. Native mode is explicit, uses the job user's filesystem and
environment, and offers no container filesystem/credential isolation of its own.
Native mode refuses host Git/API commands, including status and pipeline reads.
forge-publish is a separate host Go operation and also rejects --executor native.
Local help, settings and template-init are handled before executor dispatch. Use regular pmtool commands
without --executor native for interactive releases; their Go backend uses host Git
and the provider APIs directly, without starting Docker.

## Bootstrap

```text
policy-noop → review → policy → bootstrap-prepare → optional module conversion
→ migrate → sync → edit → check → bootstrap-commit → bootstrap-push → mr
→ check → release-tag → verify Forge/canary → reviewed cleanup
```

Prepare with an explicit `--branch bootstrap/<purpose>`. Main must be created from
the intended original history before development. Conversion preview uses jig
--dry-run without applying file changes; conversion is a separate operation.
A Docker invocation may still initialize missing host image configuration. Fix generated metadata, dependencies,
fixtures and namespace before continuing. Existing baseline conflicts require
reviewed `--files`; custom spec settings belong in `spec_helper_local.rb`.

## Module releases and recovery

```text
release-prepare → sync → edit → check → release-submit → mr → check → release-tag
```

`release-prepare --version X.Y.Z` synchronizes clean main and creates the release
branch. Update metadata, matching changelog, implementation, tests and README.
`check` runs audit → VoxBox validation/specs → release checks.

`release-submit` commits then pushes the branch. If push fails, preserve the commit
and retry `release-push`. `mr` creates/reuses the MR, waits for its exact-head push
pipeline and any required human approval, merges that SHA and synchronizes local
main. If synchronization fails,
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
error. Even a successful watcher checks pipeline status, not the release-tag
publication guards or the presence of forge-publish. Verify the publishing job and
Forge artifact separately during recovery. pipeline-logs is a separate read
operation and can succeed for failed CI.

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
make build VERSION=v2.0.0
```

Build includes Go format/vet/lint/coverage/race, security, real ERB and Ruby-asset
lint, four platform binaries, LICENSE/checksums and native smoke. There is no
VERSION file; Git tags and build parameters supply the version. Team template tests
must also lint rendered Ruby under their supported module styles; source-asset
lint alone does not validate generated profiles.

Use the [source Make workflow](#launcher-source-make-targets) for release branches,
separate implementation/preparation commits, PR merge and CI artifact verification.
Publication remains `make push-release VERSION=v2.0.0` after reviewing the binary.
After successful publication, `push-release` and `push-release-tag` preview and
remove the merged source release branch. Retry with `make release-cleanup VERSION=vX.Y.Z`.
See [source release checks and cleanup](DEVELOPMENT.md#versions-and-source-publication).
Direct release-tag creates only a local annotated tag; push-release-tag publishes
that same tag and waits for CI. Their artifact and exact-main guards remain intact.
Optional release-tag-noop runs those guards without creating a tag. Existing
push-main supports a prepared clean main; ci-artifacts retries the artifact phase.
No rc.N lane is added. Preserve tags on failure, inspect/retry CI and resume the
identical push-release-tag. Build/check/clean never publish; after clean, verify
CI artifacts again before tagging.

## Module scaffolding, conversion and packages

For a new module, select its existing parent directory:

```sh
pmtool module-new --module /absolute/modules --name sample
```

For an existing legacy module, conversion is an alternative before its first sync:

```sh
pmtool module-convert-noop
pmtool module-convert
```

Build an existing module from its own directory:

```sh
pmtool module-build
```

module-new runs jig new module --skip-interview in a new child of --module (the current
working directory by default); existing files, directories and symlinks are refused.
NAME must start with a lowercase ASCII letter, followed only by lowercase ASCII
letters, digits or underscores. Hyphens are not accepted.
Fill in author, license, source, supported OS/releases and dependencies before checking.
module-convert is for initial onboarding only, before .sync.lock.json exists.
Both conversion and its preview refuse that lock.
The module must have a manifests directory. Conversion replaces
Gemfile, Rakefile and spec/spec_helper.rb and removes Gemfile.lock. Review customizations
first; the pinned VoxBox currently bundles jig 2.5.0 without conversion backups.
The upstream jig 2.5.1 adds .orig backups, but does not update an existing image digest.
For an already synchronized module use sync, not conversion or jig renew.
Both operations use jig's embedded scaffold; the versioned team baseline still comes
from template.use in .sync.yml. The two template formats are independent.
module-build runs jig build and writes pkg/OWNER-MODULE-VERSION.tar.gz.
Its default packaging allowlist excludes development files and .sync.yml; it does not
read .pdkignore. Review any custom packaging through jig.toml before publishing.
Native CI supports module-build with installed jig; module-new/conversion use Docker.

## Forge upload

forge-publish uploads the already-built, metadata-matching archive through Go without
Docker, Git/API provider tokens, or image/config initialization. It requires FORGE_URL
(a credential-free HTTPS base URL without query/fragment) and FORGE_PUBLISH_TOKEN
in the environment. Supply the raw token without a Bearer prefix; pmtool adds the
Authorization header. The endpoint is BASE_URL/v3/releases. Tokens are never arguments.

The archive must be pkg/OWNER-MODULE-X.Y.Z.tar.gz under the selected module, with
a stable metadata version and no symlinks in the archive path. It must be a regular
nonempty file of at most 256 MiB, with at most 256 MiB after decompression. The
whole gzip stream, including its checksum/footer, is verified before upload.
It must contain exactly one matching regular
OWNER-MODULE-X.Y.Z/metadata.json of at most 1 MiB with the same name/version.
The V3 payload is JSON/base64 file content.
Only HTTP 201 succeeds; redirects, duplicate-version conflicts and other failures stop
publication without automatic retries. The request timeout is two minutes.
--dry-run requires FORGE_URL and a valid archive, but no publish token; it verifies
the artifact and prints the endpoint without uploading. Both modes reject
--executor native; omit it because the uploader already runs on the host.
Preserve the release tag on failure and inspect Forge state
before retrying. This explicit publishing operation does not replace protected-tag,
exact-main CI and release-check gates in the team's CI pipeline.

## OS matrix completeness

OS-matrix completeness is template policy, not a universal launcher gate. A team
helper can require nonempty OS/releases in metadata.json and FacterDB facts for
every declared pair before loading specs, preventing a missing Debian 13 target
from silently disappearing from on_supported_os. Use that alongside the selected
template's executable-spec, pending/skipped-suite and resource-coverage checks.
Fact availability does not prove tests exercised every configuration; keep real resource
and parameter assertions under the full on_supported_os matrix.
Other templates choose their own audit/coverage policy. The default remains pinned
OpenVox 8; OpenVox 9 requires a separately verified environment lane.

## Upgrading to 2.0

Version 2.0.0 removes the PDK executable/image dependency and its CLI interface.
Existing PDK commands and --pdk-image are rejected; there are no compatibility
aliases that silently run a different toolchain.

| Version 1 interface                       | Version 2 replacement                                      |
| ----------------------------------------- | ---------------------------------------------------------- |
| pdk / pdk-local                           | voxbox, or check for the full sequence                     |
| pdk-convert / pdk-convert-local           | module-convert before initial managed sync only            |
| pdk-convert-noop / pdk-convert-local-noop | module-convert-noop before initial managed sync only       |
| --pdk-image / PDK_IMAGE                   | --voxbox-image / VOXBOX_IMAGE for validation and jig tools |
| PDK validation/unit jobs in shared CI     | VoxBox validation (validate, lint, rubocop) and unit jobs  |
| PDK-based Forge build/upload              | module-build, then guarded CI forge-publish                |

check now runs audit → VoxBox validation → VoxBox specs → release checks.
The validation phase includes syntax, Puppet lint and RuboCop; it is not syntax-only.
Review conversion before applying: jig overwrites Gemfile/Rakefile/spec_helper and
removes Gemfile.lock. For an existing managed baseline use sync instead.

Publish pmtool v2 before adopting CI that downloads it. Team maintainers then
release an updated immutable template/CI version that uses v2 and removes PDK jobs.
Modules select that template in .sync.yml, synchronize on a work branch and pass
check plus branch CI before their ordinary release. Do not point existing modules
at an unpublished template or move old launcher/template/module tags.
The launcher does not rewrite module selectors, Forge artifacts or host config
when upgrading. Template contracts and coverage policy remain team-owned.

The Go module is github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3.
The executable remains pmtool, and release downloads use the existing repository
URL without a Go-major path suffix. Build tools/source installs use Go 1.27.3 and the
/v3 Go package path for the current major release. The runtime remains the pinned OpenVox 8 VoxBox image; this
major version does not adopt OpenVox 9 or automatically update bundled jig.


## Private test fixtures

Declare Forge fixtures in `.fixtures.yml` under `forge_modules`. With
`fixtures.resolve_dependencies: true`, list the direct fixtures and let OpenVox
install their dependencies from module metadata. Configure `FORGE_URL` and the
read-only `FORGE_TOKEN` for the same authenticated proxy. pmtool removes the
private installer config before running RSpec. See [Forge fixtures](FIXTURES.md).


## Required command inputs

[Command/input reference](COMMAND-INPUTS.md) distinguishes required flags and
credentials from values inferred from the module, Git, template or defaults.


## Token requirements and preflight

See [developer and CI token requirements](TOKENS.md) (`pmtool docs tokens`) for
scopes, project roles, job-token allowlists, Forge read/publish separation,
optional template overrides and fail-fast input validation. `pmtool help` is
short usage; `pmtool docs` opens the full manual without Docker or credentials.


## Version 3 fixture migration

Move dependencies from the retired `.sync.yml` archive section to standard
`.fixtures.yml` forge_modules using repo/ref. Configure FORGE_URL/FORGE_TOKEN for
authenticated proxy reads, including public upstream modules served by the proxy.
FORGE_TOKEN_HOST is no longer used. The standard installer reads a private
Puppet config only during preparation; it is removed before the spec worker.
The Go module/import/linker paths use /v3; binary/repository/release URLs stay the
same. Publish the new launcher before updating/versioning team CI and syncing
consumers. See [Forge fixtures](FIXTURES.md) for the complete lifecycle.

# Module workflow commands

The pmtool binary owns this command set, its permissions and orchestration.
The environment image contains tools/dependencies only. All documentation is local. `pmtool help` is short usage; `pmtool docs` is the
complete launcher manual (also `pmtool docs launcher`):

```sh
pmtool docs launcher
pmtool docs onboarding --section list
pmtool manifest
```

See [required command inputs and optional overrides](COMMAND-INPUTS.md).

## Command index

- Local discovery: help, docs, manifest, settings; root version, config, completion and ai export.
- Module preparation: bootstrap-prepare, main-sync, release-prepare, migrate.
- Baseline/checks: sync, sync-noop, audit, check, voxbox, release-checks, template-init, template-test.
- Scaffolding/packages: module-new, module-convert, module-convert-noop, module-build, forge-publish.
- Module commits: bootstrap-commit, bootstrap-commit-noop, release-commit,
  release-commit-noop, release-submit.
- Module pushes/tags: bootstrap-push, bootstrap-push-noop, release-push,
  release-push-noop, release-tag, release-tag-noop.
- Requests and CI: mr, mr-create, mr-merge, pipeline-jobs, pipeline-logs.
- Diagnostics: doctor checks origin, template access, VoxBox and Forge.
- Policy and cleanup: policy, policy-noop, policy-delete-master, cleanup, cleanup-noop.
- Inspection: status, diff.
- Compatibility aliases: bootstrap-local, bootstrap-local-noop, audit-local. See the
  [alias mapping](manual.md#compatibility-commands-and-optional-previews).

Commands accept their own options, not every flag in manifest. Root flags precede
commands. See [syntax and defaults](manual.md#command-syntax). Catalog read_only
metadata is not a permission grant for remote API actions.

## Select a clone and execution environment

Run in one physical ordinary module clone or supply --module after the command.
Provider/project come from origin. Host Git/API project names must match
`puppet-<module>`; lifecycle checks matching owner-module metadata. --image, --config, --docker, --executor,
--color and --dry-run precede the command. Module template/CI/provider settings
come from .sync.yml with CLI precedence. See [manual](manual.md).

Run `pmtool doctor` before troubleshooting `check`. See the
[doctor checks and limits](manual.md#doctor). `mr` and `mr-merge` wait for required
human review after exact-head push CI succeeds. `mr-create` stops after opening
the request.

## Template authoring and CI logs

Use template-init --name NAME --output NEW_DIRECTORY --provider gitlab|github|both
(default both) to create a neutral schema-1 validation starter. It runs offline
without Docker; Git is needed only with explicit --origin URL to initialize main
and add the remote locally. No hosted project, commit or push is created. Existing destinations are refused. Review its policies and
run template-test; add tested team contracts and publication gates before adoption.

```text
pmtool pipeline-logs [--ref REF] [--pipeline ID] [--job ID] [--failed] [--tail N] [--output NEW_FILE]
```

Reads provider logs without Git mutation or Docker. IDs come from pipeline-jobs.
Default tail is 100 lines/job, 0 is full bounded output.
Output files are exclusive mode 0600. Known tokens/control sequences are removed;
review remaining secrets before sharing. Failed CI in mr/release-tag prints failed
job tails without changing release guards or hiding the original error. See the
[manual](manual.md#pipeline-logs) for selection, limits and authentication.

## Local inspection and baseline

Use status and diff (--file optional) to inspect changes. migrate detaches legacy template
metadata/fixtures. It removes template-url, template-ref and pdk-version from
metadata.json without running PDK. In .fixtures.yml it removes only fixtures.symlinks, preserving
unrelated data and comments; malformed fixture mappings fail explicitly. Expand a
shared fixtures alias before migration. sync-noop previews the baseline; sync applies reviewed changes
without commit/push. Existing conflicts require reviewed --files. audit enforces
the selected template baseline and policy without granting remote mutation access.

pipeline-jobs uses --ref or --branch (current branch by default), the server
branch SHA where available, and --watch 0..86400 (default 0). A one-shot read can
succeed while CI is active. Finished failed CI returns an error. A green watcher
does not check the forge-publish job or replace release-tag publication guards. Module CLI logs
use all jobs by default and a per-job tail; source Make logs default to failed
jobs and tail the combined gh output, with positive TAIL only.

## Validation

check executes audit, VoxBox validation/specs and release checks
in order. It stops before later steps after failure and preserves the exit code.
Standalone voxbox and release-checks support diagnosis. --stage validate|unit
selects one VoxBox phase; it is not accepted as a way to shorten full check.
No validator receives provider/template credentials or a Docker socket.

Native CI uses the same plan with installed tools and no Docker. Its workflow
executor permits validation/template tests and module-build and rejects host Git/API reads
as well as mutations. forge-publish also rejects --executor native and runs
directly on the host; local help/settings/template-init bypass executor dispatch. It is not a filesystem sandbox. template-test
loads and self-checks the baseline in Go, then runs the team's test/run.rb with
PMTOOL_TEMPLATE_FIXTURE pointing to a temporary JSON fixture. Docker mounts that
fixture read-only; native execution keeps the job user's filesystem permissions. No launcher source tree
or Ruby engine API is exposed to template tests.

## Bootstrap and release

For bootstrap: reviewed policy-noop/policy, bootstrap-prepare --branch
bootstrap/PURPOSE, optional module conversion, migrate, sync, implementation, check,
bootstrap-commit, bootstrap-push, mr, check and release-tag.

For an existing module:

```text
release-prepare → sync → edit → check → release-submit → mr → check → release-tag
```

Prepare --version X.Y.Z, then update metadata/changelog and code separately.
release-submit commits and pushes; standalone release-commit is local-only.
If push fails, preserve the commit and retry release-push. mr creates/reuses,
waits for exact-head push CI and required review, merges that SHA and synchronizes main. Retry main-sync
after a sync failure instead of merging again. check follows successful mr.

release-tag waits for exact-main CI, pushes the annotated tag and waits for
exact-tag publication with successful forge-publish. All ref/CI/API guards remain.
On publication uncertainty preserve the tag and inspect pipeline-jobs with explicit
--ref and --watch. Verify Forge/canary before cleanup. *-noop commands remain optional
previews except a deliberately requested preview must be reviewed before apply.

## Cleanup and policy

cleanup-noop and separately authorized cleanup select one explicit --branch.
Merged/published ancestry, tag/main consistency, successful publication and branch
protection are checked. Tags, Forge artifacts and unrelated branches are preserved.
policy-noop/policy establish main/default/release protection from intended ancestry;
policy-delete-master has explicit guards. No fleet or bulk prune exists.

## Domain audit and tests

The selected template declares audit rules and mapped specs. Keep its coverage
gate; static reachability and 100% resource coverage do not prove every configuration.
Use real parameter/resource/order assertions and negative cases. Do not add dead
parameters or fake tests to satisfy an audit. Read [templates](TEMPLATES.md) and
the team's policy; templates can differ without changing the tool environment.

## Git/API execution

Git, hosting and release commands execute directly in Go using host Git and REST
APIs. They do not pull an image or require Ruby. Puppet/RSpec checks and ERB rendering use the
selected tool environment. The existing command order, optional previews, immutable
release tags, pinned CI waits and recovery commands are unchanged.
See [workflow guards](GO-WORKFLOW.md).

## Module scaffolding and publication

module-new --name NAME creates a new jig scaffold. Initial onboarding can use
module-convert-noop/module-convert before the first managed sync. module-build
packages the module with jig; forge-publish uploads a metadata-matching archive
through the Go V3 client using FORGE_URL/FORGE_PUBLISH_TOKEN, with no redirects,
automatic retry or overwrite. Read [scaffolding and publication](manual.md#module-scaffolding-conversion-and-packages)
for conversion and packaging rules, and [Forge upload](manual.md#forge-upload)
for archive validation, credentials and CI boundaries.

A team template may reject missing OS/release facts, pending/empty suites and
resource coverage below 100%; these gates belong to its selected policy. Fact availability is
not proof of scenario coverage. See [OS matrix completeness](manual.md#os-matrix-completeness).


## Private test fixtures

Declare Forge fixtures in `.fixtures.yml` under `forge_modules`. With
`fixtures.resolve_dependencies: true`, list the direct fixtures and let OpenVox
install their dependencies from module metadata. Configure `FORGE_URL` and the
read-only `FORGE_TOKEN` for the same authenticated proxy. pmtool removes the
private installer config before running RSpec. See [Forge fixtures](FIXTURES.md).


## Token requirements and preflight

See [developer and CI token requirements](TOKENS.md) (`pmtool docs tokens`) for
scopes, project roles, job-token allowlists, Forge read/publish separation,
optional template overrides and fail-fast input validation. `pmtool help` is
short usage; `pmtool docs` opens the full manual without Docker or credentials.

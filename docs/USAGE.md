# Module workflow commands

The pmtool binary owns this command set, its permissions and orchestration.
The environment image contains tools/dependencies only. All documentation is local:

```sh
pmtool docs launcher
pmtool docs onboarding --section list
pmtool manifest
```

## Select a clone and execution environment

Run in one physical ordinary module clone or supply --module after the command.
Provider/project come from origin. --image, --config, --docker, --executor,
--color and --dry-run precede the command. Module template/CI/provider settings
come from .sync.yml with CLI precedence. See [manual](manual.md).

## Local inspection and baseline

Use status and diff (--file optional) to inspect changes. migrate detaches PDK
metadata/fixtures. It removes only fixtures.symlinks through YAML parsing, preserving
unrelated data and comments; malformed fixture mappings fail explicitly. Expand a
shared fixtures alias before migration. sync-noop previews the baseline; sync applies reviewed changes
without commit/push. Existing conflicts require reviewed --files. audit enforces
the selected template baseline and policy without granting remote mutation access.

## Validation

check executes audit, PDK validation/unit, VoxBox validation/specs and release checks
in order. It stops before later steps after failure and preserves the exit code.
Standalone pdk, voxbox and release-checks support diagnosis. --stage validate|unit
selects one PDK/VoxBox phase; it is not accepted as a way to shorten full check.
No validator receives provider/template credentials or a Docker socket.

Native CI uses the same plan with installed tools and no Docker. It permits
validation/template tests only. It is not a filesystem sandbox. template-test
loads and self-checks the baseline in Go, then runs the team's test/run.rb with
PMTOOL_TEMPLATE_FIXTURE pointing to read-only JSON data. No launcher source tree
or Ruby engine API is exposed to template tests.

## Bootstrap and release

For bootstrap: reviewed policy-noop/policy, bootstrap-prepare --branch
bootstrap/PURPOSE, optional PDK conversion, migrate, sync, implementation, check,
bootstrap-commit, bootstrap-push, mr, check and release-tag.

For an existing module:

```text
release-prepare → sync → edit → check → release-submit → mr → check → release-tag
```

Prepare --version X.Y.Z, then update metadata/changelog and code separately.
release-submit commits and pushes; standalone release-commit is local-only.
If push fails, preserve the commit and retry release-push. mr creates/reuses,
waits for exact-head CI, merges that SHA and synchronizes main. Retry main-sync
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

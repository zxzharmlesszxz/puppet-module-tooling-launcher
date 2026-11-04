# Puppet module development through pmtool

These instructions describe the installed pmtool version. Read them with the
module's own instructions and the user's current request. They do not authorize
commits, remote changes or publication.

## Execution policy and evidence

- Follow the user's execution policy. If the user runs local commands/tests,
  provide one next command and wait for its output. Never claim an unrun check,
  green pipeline or successful Forge publication.
- Preserve unrelated work, staging and current changes. Inspect failures before
  retrying. A successful command is evidence only for that command and exact code.
- A real local commit and a push are separate actions. Use authorization already
  given; ask only for genuinely missing decisions or permissions.
- Optional previews remain independent review steps. Never silently apply after
  an explicitly requested preview. Do not force push, move/delete tags, reset or
  discard work to bypass a guard.

## Architecture and discovery

- pmtool owns commands, sequences, permissions, Git/API guards, guidance and docs.
  The tool image supplies executables and dependencies only. Templates own team
  contracts, baselines and audit rules. Do not hardcode a team's policy into the image.
- Use the installed pmtool or the exact executable path supplied by the user.
  Help, documentation, completion and AI export are local and require no Docker.
  Do not replace an existing pmtool operation with ad hoc Docker/Ruby/Git commands.
- Work in one ordinary module clone. Inspect status, origin, branch, metadata,
  changelog, fixtures, Hiera and affected domain code. Identity comes from origin,
  never the checkout directory name. Linked worktrees are not supported.
- Global flags precede the command; operation options follow it. --module selects
  another physical clone. CLI overrides .sync.yml; an omitted provider comes from
  origin. Host config contains image/executable settings, never tokens or modules.
- .sync.yml selects template.use and optional ci.use/ci.provider. CI defaults to
  the selected template version. Different teams/hosts can use different templates.

```sh
pmtool help
pmtool docs onboarding --section list
pmtool docs templates
pmtool status
```

## Permissions and CI

- Upstream image CVE scans are explicit and advisory: report findings as warnings,
  without blocking module commands or requiring a custom image. Scanner failures
  are errors; do not describe them as clean scans. Keep source/Go security gates
  strict. Preserve the selected OpenVox 8 image release and digest; OpenVox 9
  support will be introduced in a separate future pmtool release, with its own
  compatibility tests and documented migration.
- The runtime is pinned upstream VoxBox with OpenVox 8; do not restore custom
  runtime builds/publication. pmtool-checks is a development-only Go/lint image.
- The default Docker executor pins the environment image ID. Plans, settings, checkout, sync and general audit run in Go.
  ERB exchanges JSON with a minimal Ruby worker; template-owned checks use the SDK. Never mount a Docker
  socket into the tool environment; pmtool starts PDK/VoxBox directly on the host.
- Provider tokens are environment variables and are passed only to their required
  steps. Never print them, put them in arguments/config/docs or mount host SSH keys.
  Private templates use separate, explicitly host-scoped credentials.
- For CI, --executor native runs the same plan using installed job tools, without
  Docker or image lookup. It is explicit and not a filesystem sandbox. It permits
  validation/template tests, not Git/API mutations. --stage validate|unit selects
  a single PDK/VoxBox phase when CI jobs are split.
- Keep local and CI validation based on the same launcher plan. CI environment and
  launcher versions are pinned independently. Read docs before changing either.

## Bootstrap

1. Inspect policy-noop; apply reviewed policy. Establish main/default/protection
   from intended original history before development, never from arbitrary edits.
2. Prepare bootstrap-prepare --branch bootstrap/PURPOSE from a clean clone.
3. If conversion is needed, preview pdk-convert-noop, review, then apply the agreed
   pdk-convert. Correct metadata, namespace, OS support, fixtures and dependencies.
4. Run migrate then sync. Migration removes only fixtures.symlinks and preserves
   other fixture data/comments. Fix malformed mappings or expand a shared fixtures
   alias explicitly; never delete unrelated fixture sections. Review conflicting
   files before explicit --files retry.
   Keep module-owned spec customizations in spec_helper_local.rb.
5. Implement domain code, Hiera, tests, metadata/changelog and English module README.
6. Run check once: audit, PDK validation/unit, VoxBox validation/specs, release checks.
   Fix failed checks; do not repeat complete green suites without a new reason.
7. After green check use authorized bootstrap-commit, bootstrap-push, then mr.
   Combined mr waits for exact-head CI, merges and synchronizes local main.
8. Run check on main, then authorized release-tag for final merged metadata version.
   Verify publication and canary before reviewed cleanup.

## Existing module release

```text
release-prepare → sync → edit → check → release-submit → mr → check → release-tag
```

- Prepare --version X.Y.Z from clean main; do not repeat prepare on an edited
  release branch. Update metadata and matching Release X.Y.Z changelog together.
  Previously consumed Git tags count as releases even without Forge publication.
- release-submit commits then pushes. On push failure preserve the commit and
  retry release-push. Standalone release-commit remains local-only.
- mr creates/reuses the request, waits for the pinned head's push CI, merges that
  SHA and runs main-sync. On sync failure preserve the merge and retry main-sync.
  After combined mr run check directly; separate watchers are unnecessary.
- release-tag waits for exact-main CI, pushes the annotated tag, then confirms
  exact-tag publication. Missing/failed/canceled/skipped/manual CI, changed refs
  and API errors stop it. Never weaken these guards to finish a release.
- If tag publication waiting fails, preserve the tag and inspect pipeline-jobs
  with its explicit ref and --watch 5. A tag push is not publication evidence.
- sync-noop, commit-noop, push-noop and release-tag-noop are optional diagnostics.
  Do not insert them into every normal step. Cleanup-noop remains a review before
  the separately authorized destructive cleanup of an explicit branch.

## Domain code and template policy

- Keep typed public parameters, automatic Hiera parameter lookup and defaults in
  module data. Avoid explicit lookup calls in parameter defaults. Define and test
  intended hash/array merge behavior across defaults, environment and host data.
- Feed shared contracts into actual domain resources. Do not add dead aliases,
  parameters, resources or always-positive tests just to satisfy static checks.
- Separate repository, packages, configuration and services using the module's
  established pattern. Preserve package-owned directories, units and base files
  unless taking ownership is deliberate and justified.
- Verify key/source → cache refresh → packages → config → services ordering on the
  first apply. Class chains alone do not prove ordering of external APT resources.
  Do not disable signature verification or delete keys to fix replacement bugs.
- Test hold/unhold, package removal versus running services and file lifecycle.
  Distinguish content, source, unmanaged and absent explicitly; preserve project
  files and avoid unconditional destructive defaults.
- Tests and coverage requirements come from the selected template. If it mandates
  specs for all entities and 100% Puppet resource coverage, preserve those gates.
  Resource coverage does not prove Ruby branch coverage or every configuration.
- Module README explains module use; launcher commands and CI internals belong
  in pmtool documentation. Document Hiera migration and breaking changes clearly.

## Launcher or tool-environment changes

Update source, regression tests, documentation and assistant instructions together.
Keep the launcher in cmd/internal/docs structure. Keep workflow source and policy
out of the environment image. Git/API/release logic belongs to internal/hostflow
in Go; internal/templatekit owns checkout/authentication, schema/policy, CI selection,
sync and general audit. internal/pmtool plans validation in Go. Keep Ruby limited
to ERB and module-side/template-owned Puppet/RSpec checks. Do not restore a Ruby
settings/planning/sync/audit engine or a test-only copy. Port regressions
to Go against the runtime code instead of preserving old backends. Do not weaken
ref, CI, publication, preview, recovery or cleanup assertions.
Host Git/API commands bypass Docker/image settings; provider tokens remain scoped
to the selected origin. Test races and partial recovery using temporary Git repos
and local HTTP servers, never real publication.
User-supplied green evidence is required after changes before release operations.

Native CI and Docker execution must preserve exit codes and stop before subsequent
steps after failure. Verify token/author scope per step, no nested socket, literal
argv handling, private template host checks, immutable refs and partial recovery.

Template test harnesses consume PMTOOL_TEMPLATE_FIXTURE JSON rather than launcher
source paths. Preserve ERB context and generated module-side profile bytes; compare
Go selection with the real Ruby profile helper. Update Go regressions and the team's
test harness together without altering module code or weakening coverage gates.

## Publishing the launcher source

Keep source releases separate from module pmtool commands. The user publishes.
Commit implementation changes first with changelog entries under ## Unreleased.
Then finalize them under ## Release X.Y.Z in a separate release-only commit named
chore(release): prepare vX.Y.Z. Do not add a VERSION file. Push after both local
commits; verify CI artifacts for that final prepared SHA, not an earlier code commit.
From prepared committed clean main, make push-main pushes only main, waits for exact-SHA
CI, downloads its dev-<full SHA> artifacts and verifies checksums, LICENSE and
native version/help. Test the downloaded binary with the team's template/module.
make ci-artifacts retries the artifact phase without pushing. Branch CI creates
Actions artifacts, not a GitHub Release or prerelease.
Only after that review use make push-release VERSION=vX.Y.Z. It requires verified
artifacts for the current synchronized main, runs preflight and creates/pushes the
stable tag; it does not push main. Direct tag targets enforce the same artifact gate.
Tag CI verifies current main and successful commit CI. A draft becomes public only
after uploads succeed. Do not add rc.N tags to this workflow. Preserve existing
tags after publication errors; inspect/retry CI and resume the identical tag.

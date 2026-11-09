# Host Git, API and release workflow

`internal/hostflow` implements module Git and hosting operations in Go. It invokes
host Git with separate arguments and uses the standard HTTP client for GitLab and
GitHub. Host project resolution currently requires a `puppet-<module>` repository
name. It constructs HTTP(S) remote URLs with provider credentials even when the
configured identity is SSH/SCP. Host Git transport rewrites still apply; the
launcher does not mount or forward SSH keys into tools containers. These operations do not start Docker, probe Ruby or create image settings.
Read-only settings/provider inference can still invoke host Git for origin.
Template checkout, sync, general audit, template generation and validation planning also run in Go.
Ruby is used only for ERB and template-owned Puppet/RSpec checks.

The command order remains:

```text
release-prepare → sync → edit → check → release-submit → mr → check → release-tag
```

## Checks preserved at operation boundaries

- An ordinary clone, matching credential-free origin and valid module metadata
  identify the project. Provider precedence is CLI, `.sync.yml`, then origin.
  Linked worktrees, unfinished merges/rebases and unsafe ref names are refused.
- Preparation synchronizes main without reset or stash, rejects divergent history
  and existing local/remote release refs, and requires an increasing X.Y.Z version.
- Commits include all reviewed changes or an explicit complete file restriction.
  Release metadata and changelog must both change. Automatic selection and literal
  Git paths preserve spaces, Unicode and staged deletions. Explicit --files is an
  ASCII whitespace-separated restriction covering every change; it cannot represent
  names containing spaces. Previews leave the index and HEAD unchanged.
- Branch pushes select one SHA/ref, disable force, mirror and follow-tags, and
  preserve the local commit on failure. Retry `release-push` after fixing the error.
- MR/PR merge waits for the current source SHA's push pipeline and required
  human review, then rechecks the request head before merging. A new head
  restarts both gates. If synchronizing main fails after merging,
  retry `main-sync` and preserve the successful merge.
- Tagging pins local HEAD, remote main, the annotated tag object and successful CI.
  Unrelated active pipelines block tagging. Expected active main CI can be waited
  for; missing, failed, canceled, skipped, manual or changed pipelines stop release.
- Publication pins the pushed tag object, peeled commit and selected pipeline ID.
  Success requires the exact ref/SHA and one successful `forge-publish` job.
  Failed publication preserves the tag; inspect `pipeline-jobs --ref TAG --watch 5`.
- Cleanup requires an explicit release/bootstrap branch, matching merged request,
  published annotated tag, ancestry into main, unprotected branch and agreement of
  local/remote/tracking refs. It recomputes the plan before deletion, uses an exact
  remote lease, and preserves all tags and unrelated branches.

## Credentials and transport

API endpoints must belong to the origin host. Tokens stay in environment variables;
only the selected provider's token is used. General API redirects are refused. GitHub job-log downloads are the narrow
exception: bounded signed HTTPS redirects use no provider headers or cookies. API errors exclude response bodies and tokens.
Mutating HTTP requests are not automatically retried.

HTTPS Git authentication uses a temporary executable askpass helper containing no
secret. The token is available only in that remote Git subprocess's environment;
credential persistence is disabled. The helper is removed on success and failure.
GitHub uses `GITHUB_TOKEN`; GitLab uses `GITLAB_TOKEN` and optional `GITLAB_USERNAME`.
No token is forwarded to validators or substituted into remote URLs or argv.

## Pipeline logs (1.1.0)

```sh
pmtool pipeline-jobs --ref main
pmtool pipeline-logs --ref main --failed --tail 100
pmtool pipeline-logs --pipeline 123 --job 456 --tail 0 --output ./job.log
```

Module log reads use the selected provider API without Git mutation or Docker.
Latest-ref selection pins the available server branch head; explicit pipeline IDs
stay project-scoped and an optional ref must match. Numeric job IDs must belong
to that run's latest attempt. Default selection is all jobs, last 100 lines each;
--failed filters failed jobs and --tail 0 requests full bounded output.
Downloads are limited to 16 MiB/job and combined output to 32 MiB.

Known tokens, credential URLs and terminal controls are removed before tailing or
exclusive mode-0600 file output. This cannot detect every secret unknown to pmtool.
Missing/expired logs or API failures remain errors. Automatic failed-job tails in
mr/main/publication failure paths never replace the primary error or weaken merge,
exact-ref or tag recovery guards. See [selection and limits](manual.md#pipeline-logs).
Source Make pipeline-logs is a separate command for launcher-source Actions runs.

## Template generation (1.1.0)

The local template-init command bypasses API, Docker and image config. It uses
Git only with explicit --origin URL to initialize main and add the remote locally,
without commits, tags or network access. Default generation remains Git-free. It
creates a new directory with a neutral schema-1 validation starter, self-tests,
authoring docs and selected GitLab/GitHub CI. It refuses existing output and
symlink parents. This does not install team contracts, coverage enforcement or
Forge publication. Stable generated CI verifies launcher downloads; dev builds
need --launcher-version naming a published release. See [template authoring](TEMPLATES.md#create-a-template).

## Explicit Forge upload

forge-publish is a separate Go host operation, not a Git/API lifecycle command.
It bypasses image/config initialization and rejects --executor native. It validates
a prebuilt metadata-matching archive before uploading to an explicit HTTPS Forge
endpoint with an environment token. The uploader itself does not inspect Git, CI or
tag protection; team CI must enforce those gates before invoking it. It does not
replace release-tag and its publication wait. See [Forge upload](manual.md#forge-upload).

## Regression evidence

Go tests use temporary ordinary/bare Git repositories and local HTTP servers.
They cover both API adapters, pagination, authentication scope, redirect refusal,
ref/metadata guards, CI status failures, concurrent changes, literal paths, staged
deletions, preview immutability, partial failures and cleanup preservation.

All lifecycle and hosting regressions exercise the Go implementation. There is no
Ruby lifecycle backend, reference directory, shadow executor or historical test
implementation. Go template regressions cover schema, groups, ownership, preview/apply, archive
safety and authentication. Real ERB checks compare the generated module-side
profile with Go. There is no Ruby planner or launcher sync/audit engine. Hints for validation and release commands share
the Go guidance implementation, including recovery and shell-safe arguments.

The pinned YAML v3 dependency reads `.sync.yml`; the checks image downloads its
verified module during image construction so offline checks and cross-builds can
use the read-only module cache. No linter or coverage threshold is relaxed.

## Regression ownership

| Scenario family                                                                 | Go regression suite                            |
| ------------------------------------------------------------------------------- | ---------------------------------------------- |
| Bootstrap/main preparation, commits, exact pushes, preview/index preservation   | internal/hostflow/regression_lifecycle_test.go |
| Main/tag CI statuses, bounded creation wait, pinned pipeline ID, MR recovery    | internal/hostflow/regression_wait_test.go      |
| Policy preview/apply, access updates, rulesets, master deletion                 | internal/hostflow/regression_policy_test.go    |
| Cleanup ancestry, stale refs, worktrees, local races and remote leases          | internal/hostflow/regression_cleanup_test.go   |
| Both REST adapters, job/request pagination, unsafe GitHub conclusions           | internal/hostflow/regression_provider_test.go  |
| Workflow advice, local state, explicit settings and partial-success recovery    | internal/hostflow/guidance_test.go             |
| Log API/redirect isolation, selection, redaction, output and failed-CI recovery | internal/hostflow/logs_test.go                 |
| New-template schema/providers, no overwrite and path guards                     | internal/scaffold/generate_test.go             |
| Default generation/completion, no Git/Docker/config side effects                | internal/pmtool/template_init_test.go          |
| Opt-in origin init, unborn main, caller isolation and Git failure recovery      | internal/pmtool/template_repository_test.go    |
| Validation fail-fast, exit/signal codes, quoting, no SDK/credential hints       | internal/pmtool/regression_workflow_test.go    |

Template regressions are in internal/templatekit: engine_test.go covers conflicts,
retirement, races and audit; policy_test.go covers template authority/schema;
checkout_test.go covers archive and credential boundaries; reference_test.go covers
pinned CI/template selection; render_test.go runs real ERB in workflow-check.
scaffold_test.go checks generated templates through real ERB sync/audit and their
own Ruby suite; scaffold_bootstrap_test.go exercises the shipped CI installer.
The optional versioned-team integration requires PMTOOL_TEST_TEMPLATE; a SKIP
means that external team checkout was not tested by this invocation.
internal/pmtool/planner_test.go covers ordered execution, native plans and isolated workers.

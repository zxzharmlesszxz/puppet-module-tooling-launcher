# Host Git, API and release workflow

`internal/hostflow` implements module Git and hosting operations in Go. It invokes
host Git with separate arguments and uses the standard HTTP client for GitLab and
GitHub. These operations do not start Docker, probe Ruby or create image settings.
Template checkout, sync, general audit and validation planning also run in Go.
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
  Release metadata and changelog must both change. Literal paths preserve spaces,
  Unicode and staged deletions; previews leave the index and HEAD unchanged.
- Branch pushes select one SHA/ref, disable force, mirror and follow-tags, and
  preserve the local commit on failure. Retry `release-push` after fixing the error.
- MR/PR merge waits for the selected source SHA's push pipeline, rechecks the
  request head before merging, then synchronizes main. If synchronization fails,
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
only the selected provider's token is used. HTTP redirects are refused so an API
redirect cannot forward credentials. API errors exclude response bodies and tokens.
Mutating HTTP requests are not automatically retried.

HTTPS Git authentication uses a temporary executable askpass helper containing no
secret. The token is available only in that remote Git subprocess's environment;
credential persistence is disabled. The helper is removed on success and failure.
GitHub uses `GITHUB_TOKEN`; GitLab uses `GITLAB_TOKEN` and optional `GITLAB_USERNAME`.
No token is forwarded to validators or substituted into remote URLs or argv.

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

| Scenario family                                                               | Go regression suite                            |
| ----------------------------------------------------------------------------- | ---------------------------------------------- |
| Bootstrap/main preparation, commits, exact pushes, preview/index preservation | internal/hostflow/regression_lifecycle_test.go |
| Main/tag CI statuses, bounded creation wait, pinned pipeline ID, MR recovery  | internal/hostflow/regression_wait_test.go      |
| Policy preview/apply, access updates, rulesets, master deletion               | internal/hostflow/regression_policy_test.go    |
| Cleanup ancestry, stale refs, worktrees, local races and remote leases        | internal/hostflow/regression_cleanup_test.go   |
| Both REST adapters, job/request pagination, unsafe GitHub conclusions         | internal/hostflow/regression_provider_test.go  |
| Workflow advice, local state, explicit settings and partial-success recovery  | internal/hostflow/guidance_test.go             |
| Validation fail-fast, exit/signal codes, quoting, no SDK/credential hints     | internal/pmtool/regression_workflow_test.go    |

Template regressions are in internal/templatekit: engine_test.go covers conflicts,
retirement, races and audit; policy_test.go covers template authority/schema;
checkout_test.go covers archive and credential boundaries; reference_test.go covers
pinned CI/template selection; render_test.go runs real ERB in workflow-check.
internal/pmtool/planner_test.go covers ordered execution, native plans and isolated workers.

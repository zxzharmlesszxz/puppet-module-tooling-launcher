# Developer and CI token requirements

These requirements apply to the implemented Git/API calls, not to every command.
`pmtool help` is short usage; `pmtool docs` is the full manual; this document is
available with `pmtool docs tokens`. No token is required for help/docs/completion,
local inspection, template generation, module generation/build or public fixtures.
See [command inputs](COMMAND-INPUTS.md) for required flags and defaults.

A token scope and its owner's project role are separate requirements. Scopes do
not grant project membership, bypass branch/tag rules or satisfy an approval.
The module provider token is not silently reused for a template or Forge.

## Developer workstation

| Purpose                            | Credential source / variable                             | Required access                                                 |
| ---------------------------------- | -------------------------------------------------------- | --------------------------------------------------------------- |
| GitLab module lifecycle            | `GITLAB_TOKEN`                                           | Personal token: `api`; use only intended projects               |
| GitLab project/group bot lifecycle | `GITLAB_TOKEN`                                           | `api` + `write_repository`; bot role and scope cover the module |
| GitHub module lifecycle            | `GITHUB_TOKEN`                                           | Fine-grained PAT or App token; operation permissions below      |
| Template HTTPS fetch               | Saved host Git credential helper                         | Read access to the selected template repository                 |
| Explicit template override         | `TOOLING_TEMPLATE_TOKEN` + `TOOLING_TEMPLATE_TOKEN_HOST` | Repository-read access; host exactly matches the template       |
| Private test fixture               | `FORGE_TOKEN` + `FORGE_TOKEN_HOST`                       | Forge read role for the owner/space declared by `auth: forge`   |
| Forge publication                  | `FORGE_URL` + `FORGE_PUBLISH_TOKEN`                      | Forge publish role for the metadata owner/space                 |

For GitLab, a personal `api` scope includes Git-over-HTTP access. Project/group
access-token scopes must also cover Git pushes: use `write_repository` alongside
`api` for the complete lifecycle. An inspection-only token can use `read_api`;
Git clone/fetch separately needs repository-read access. A deploy token with
`read_repository` is suitable for template reads, not module REST API operations.
See [GitLab token scopes](https://docs.gitlab.com/security/tokens/access_token_scopes/).

Developers need permission to push their work branches. For the default pmtool
policy, the actor performing protected-main merges, `v*` tag creation and policy
changes needs the Maintainer role and the corresponding allowed-list entries.
An existing stricter project/group rule may require an Owner/administrator to
configure policy; pmtool does not bypass it. A project token is limited to its
project; use a group token or separate template-read credential only when that
scope is actually needed.

Template authentication precedence is explicit token override, saved host Git
helper, then anonymous access. The helper uses the selected HTTPS URL; it does
not store/erase credentials or read a module-local helper configuration.
No TOOLING_TEMPLATE_* variables are needed with ordinary saved Git credentials.
For an override, `TOOLING_TEMPLATE_USERNAME` is optional; default `oauth2` works
for personal/access tokens, while deploy tokens need their issued username.
`TOOLING_TEMPLATE_TOKEN_HOST` is a bare host with an explicit port if needed,
not a URL. Credentials are never placed in origin URLs or .sync.yml.

## GitHub repository permissions

Select the actual module repositories when creating a fine-grained PAT or an
App installation token. Repository Metadata read is implicit. Combine only the
permissions required by the operations you run:

| Operation                                         | Fine-grained repository permissions                                 |
| ------------------------------------------------- | ------------------------------------------------------------------- |
| Observe pipelines/jobs/logs                       | Actions: read; Contents: read for repository/Git reads              |
| Push commits or create/delete Git refs            | Contents: write                                                     |
| Create/update PRs and use the combined `mr` flow  | Pull requests: write + Contents: write + Actions: read              |
| Push changes under `.github/workflows/`           | Workflows: write in addition to Contents: write                     |
| Apply repository/default-branch/protection policy | Administration: write; actor must be allowed to administer the repo |
| Read a private template via an explicit token     | Contents: read on the template repository                           |

These correspond to the [GitHub fine-grained permission reference](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens),
[workflow-run API](https://docs.github.com/en/rest/actions/workflow-runs) and
[PR API](https://docs.github.com/en/rest/pulls/pulls). For a classic PAT, private
repository lifecycle normally uses `repo`, adding `workflow` only when changing
workflow files. Organization approval, SSO authorization and App installation
selection may additionally restrict access. Do not request administration rights
for ordinary edits if another operator owns the policy step.

The pmtool variable is named `GITHUB_TOKEN` whether its value is a PAT, App token
or an Actions-generated token. Those credentials have different behavior. Pushes
made with the automatic Actions token do not create the next `push` workflow,
which pmtool's release flow waits for. Use a PAT or App token for lifecycle
mutations that must trigger subsequent CI. See [workflow triggering](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow).

## GitLab CI

The developer's full API token is not required for ordinary module audit/tests.
GitLab creates `CI_JOB_TOKEN` automatically, only for the running job. The team's
CI bootstrap explicitly maps it to the template token override using username
`gitlab-ci-token` and `CI_SERVER_HOST`. Do not create a permanent variable named
CI_JOB_TOKEN and do not put a personal lifecycle token into every validation job.

Private templates have two distinct access checks:

1. Resolving `include:project` happens before a job starts. The pipeline actor
   needs access to that private CI project (at least Reporter for the include).
   A token exported inside a later job cannot repair this check. See
   [GitLab includes](https://docs.gitlab.com/ci/yaml/#includeproject).
2. Host-side template Git fetch inside a job uses the job token. In the target
   template project's inbound job-token allowlist, allow the consuming module
   project or its approved group. The triggering user must also have access;
   allowlisting alone grants no extra membership. See
   [CI job-token access](https://docs.gitlab.com/ci/jobs/ci_job_token/).

The Neon protected-tag release gate reads main pipelines and jobs for the exact
SHA using JOB-TOKEN authentication. Where fine-grained job-token permissions
are enforced, allow **READ_PIPELINES** and **READ_JOBS** for those endpoints.
Keep normal repository-read access for template/source fetches. Check availability
against the installed GitLab version; do not replace these reads with a broad
personal token by default. See [fine-grained job permissions](https://docs.gitlab.com/ci/jobs/fine_grained_permissions/).

A separate GitLab API token is needed only if CI deliberately runs pmtool's
module lifecycle/API mutations. That is not part of the normal validation/tag
publication jobs. In particular, enabling job-token Git pushes is unsuitable
for this flow because those pushes do not start a new pipeline.

## GitHub Actions CI

Normal validation starts with repository `contents: read` and permission to use
the selected reusable workflow. Private reusable workflows must permit the
caller repository; private template Git checkout additionally needs a read PAT
or App token when the job token cannot access that other repository. Pass it
through the explicit template secrets only to the fetch stage.

Repository `GITHUB_TOKEN` permissions can be reduced by organization settings and
cannot be elevated beyond the caller by a reusable workflow. Allowlist/secret
forwarding is configured separately from choosing `ci.use`. The current generated
caller forwards Forge publication settings only. The starter reusable workflow
declares optional template secrets, but private template and fixture-read secrets
still need an explicit caller/reusable-workflow update; setting them only on the
developer's machine does not forward them. Keep a scoped PAT
or App token for automated lifecycle changes requiring new push CI.

## Forge reads and publication in CI

| Job purpose               | Variables                          | Required service authorization                               |
| ------------------------- | ---------------------------------- | ------------------------------------------------------------ |
| Public dependencies only  | None                               | Public archive access                                        |
| Private test fixtures     | `FORGE_TOKEN`, `FORGE_TOKEN_HOST`  | Read-only Forge token for the declared fixture owners/spaces |
| Publish reviewed archives | `FORGE_URL`, `FORGE_PUBLISH_TOKEN` | Forge publish token authorized for the module owner/space    |

GitLab/GitHub tokens are not Forge tokens. For the team's Forge service, a read
token cannot publish, while an ordinary publish token cannot replace an existing
version or delete a release. Administrator-level replacement is exceptional,
not a developer/CI requirement. Other Forge implementations may name roles
differently; they must authorize the operations implemented by the selected API.

Set secrets in the CI secret store, mask them, and scope them to the repositories
and jobs that need them. Publish credentials belong only to protected release
jobs. A protected read variable will not appear in unprotected work-branch jobs:
protect the trusted work branches or provide a separately scoped read credential
for permitted validation jobs. Do not expose write credentials to fork/untrusted
pipelines. Configure the read host separately as a non-secret exact hostname.

Declare private archive fixtures with `auth: forge` in `.sync.yml`; this makes
both read variables mandatory before SDK startup. Public fixtures omit `auth`
and do not receive the configured Forge token. pmtool verifies the pinned archive
and gives the SDK a temporary fixture manifest/files, never those credentials.
See [fixture preparation](FIXTURES.md). This feature requires a launcher release
containing it and a matching CI launcher pin; published 2.0.0 does not implement it.

## Early checks and diagnosing refusals

Before actions, pmtool reports missing required argument/env names and exits
nonzero. Optional overrides and values inferred from origin, Git config, metadata
or .sync.yml are not made mandatory. Template overrides and private fixture
credential/host pairs are checked before SDK startup; provider operations check
the selected provider token and author defaults before mutations. Pure local
help/docs/completion require no credentials. Generic --dry-run plans do not
require execution credentials; real *-noop commands can still read remote state
and need its token. Forge --dry-run still validates its URL and local artifact.

Preflight cannot prove server-side scopes, expiry, membership, SSO approval or
protected-ref permissions offline. An HTTP 401 indicates refused authentication;
403 can indicate role/scope/allowlist/policy restrictions, and private projects
may hide access failures behind 404. Read the operation's error without printing
credentials. Never log tokens, store them in Git/module/config files, put them
in command arguments, or disable TLS/redirect checks to bypass an access refusal.

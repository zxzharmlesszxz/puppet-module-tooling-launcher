# GitLab and GitHub CI

One launcher and runtime support GitLab and GitHub. Commands operate on the selected local
module clone; repository identity and hosting server come from its `origin`.
`mr`, `mr-create` and `mr-merge` keep their names on both providers and operate on
GitHub pull requests when GitHub is selected. No second launcher is required.

## Choose the CI reference

Pass one reference after the command:

```sh
pmtool sync --ci 'github.com/team_b/pipelines/.github/workflows/puppet.yml@v1.0.0'
pmtool check --ci 'github.com/team_b/pipelines/.github/workflows/puppet.yml@v1.0.0'
```

For GitLab:

```sh
pmtool sync --ci 'git.example.com/team_a/platform/pipelines//ci/puppet-module.yml@v1.0.0'
```

GitLab supports nested namespaces and nested file paths. `//` separates the
repository from the file without API discovery or ambiguity. A root file can use
`git.example.com/team/pipelines/puppet.yml@v1.0.0`. GitHub references have the standard
`owner/repo/.github/workflows/file.yml@version` shape; the host prefix is optional
when the module origin supplies it. Do not include a URL scheme or credentials.
Versions accept version tags (including v1 and v1.2.3) or a full commit SHA.
Use a full SHA when the reference must remain immutable; tags can move on the server.

## Module CI settings

Normally select only template.use in the module .sync.yml; CI comes from that
template's provider-specific entrypoint at the same resolved SHA. Set ci.use or
--ci only to override it. ci.provider or --provider overrides origin autodetection.
Host config.json contains image/docker/socket only; tokens remain environment-only.
The launcher's read-only, network-free settings query returns CI/provider/template
metadata without fetching or evaluating a template. Default CI is resolved later
by the runtime, preserving isolation of provider credentials.

## Provider and credentials

`github.com` origins select GitHub automatically. Other hosts retain GitLab as
the compatibility default. For GitHub Enterprise use `--provider github` consistently
or set `ci.provider: github` in `.sync.yml`. Explicit CLI selection wins.

Host Git/API commands require the current repository basename `puppet-<module>`,
with lifecycle metadata naming the same module after its owner prefix. This is
a current host-workflow restriction, independent of the selected template.

Identity is read from exactly one repository-local `remote.origin.url`. Runner
`url.*.insteadOf` rules, including external configuration through `include.path`,
remain transport configuration. Provider selection and release guards use the
configured URL without rewriting it or removing runner authentication settings.
The stored URL must still be credential-free, with no query/fragment or embedded
line breaks. Multiple URLs are rejected. CI templates must normalize origin if
the runner writes its token directly into that URL before pmtool runs.

```sh
pmtool mr --provider github --module /absolute/module
```

GitLab uses `GITLAB_TOKEN`, with optional `GITLAB_BASE_URL` and `GITLAB_USERNAME`.
GITLAB_BASE_URL is the full HTTP(S) API endpoint ending in /api/v4 on the origin
host, not just its web root; preserve an HTTP origin's installation path prefix.
GitHub uses `GITHUB_TOKEN`, with optional `GITHUB_API_URL`. The GitHub API defaults
to `https://api.github.com` or `https://<enterprise-host>/api/v3`; an override must
use HTTPS and remain on the origin server (api.github.com for github.com). SSH ports are never reused as API ports.

Module API/Git operations use only the selected provider token. Token values are
absent from argv, config, hints and dry-run output. Template fetch uses its separate
host/repository-scoped template token; ERB, audit scripts and validation subprocesses
receive neither provider nor template credentials. Host API steps use no Docker socket.
pmtool constructs remote module URLs as HTTP(S), including for SSH/SCP origin
identity, and supplies the selected provider credentials through noninteractive
askpass. Host Git transport rewrites remain effective and can select another
transport. pmtool provides no template SSH fetching or SSH-key mount/forwarding
mechanism for tool containers.

GitHub permissions depend on the operation: Actions read for observing CI, contents
read/write for Git, pull requests write plus contents write for merging, workflows
write when pushing workflow files, and administration write for protection policy.
Use a suitable personal access token or GitHub App token in the launcher's
`GITHUB_TOKEN` environment variable. The automatic Actions job token does not
trigger new push workflows when used to push/merge/tag; it cannot replace that
credential in a workflow that expects subsequent push CI. See
[GitHub workflow triggering](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow).

## Generated files and shared workflow

GitLab sync manages `.gitlab-ci.yml` with project/ref/file include.
GitHub sync manages `.github/workflows/puppet-module.yml` calling the selected
reusable workflow. It triggers on branch and v-tag pushes; it does not duplicate
checks for PR events. GitHub requires the reusable workflow under `.github/workflows`
and `workflow_call`; see [reusable workflows](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows).
The caller passes only `FORGE_URL` and `FORGE_PUBLISH_TOKEN` to the reusable workflow.

A template may supply both `ci/puppet-module.yml` and
`.github/workflows/puppet-module.yml`. Publish the selected CI package before
referencing it. This does not create repositories, mirrors, tags, protection
rules or secrets. The generated starter uses Ubuntu runners for GitHub; adapt
`runs-on` in the published workflow when using self-hosted runners.

The validation starter runs audit and VoxBox validation/specs
on branch pushes. It supplies no Forge publication. A production team workflow
should verify protected metadata-matching tags against current main and successful
required main jobs for that exact SHA, then perform release checks and publish
through a real forge-publish job. Configure the required protected Forge secrets
on consuming repositories; selecting a provider does not create those gates.
Changing providers does not silently remove existing CI files; inspect and migrate
the old CI configuration explicitly, and review all sync conflicts before replacement.

## Release and recovery

The workflow remains:

```text
release-prepare → sync → edit → check → release-submit → mr → check → release-tag
```

GitHub uses the current PR head's push workflow, checks the head again, and submits
the exact SHA to the merge API with merge-commit semantics. Missing, failed,
canceled, skipped, neutral or approval-required runs cannot count as success.
Server branch rules remain authoritative; API errors stop the operation.

`release-tag` waits for main CI at the exact SHA, rejects other active runs, pushes
the annotated tag, and waits for the tag workflow and successful `forge-publish`.
The generated caller's workflow path identifies the CI gate; an unrelated green
workflow does not satisfy it. Run IDs and ref/SHA are verified while waiting.
Job lists are paginated, including reusable workflow publication jobs.

After a post-push failure, the tag remains. Inspect `pipeline-jobs --ref vX.Y.Z --watch 5`;
never move/recreate a tag as automatic recovery. Cleanup stays a separate explicit
preview/apply operation with merged-request, ancestry, publication, branch-protection
and remote-lease guards. No historical tag cleanup is introduced.

## Module template selection

pmtool ships workflow code; the pinned upstream tools image supplies dependencies.
There is no custom runtime repository or bundled team template. Teams publish
their own template packages on compatible Git hosts. The repository/version below is illustrative:

```yaml
template:
  use: 'git.example.com/team/puppet-template@v1.0.0'
```

CI priority: **CLI --ci → module .sync.yml ci.use → selected template.yml ci**.
The template's CI is pinned to the same resolved commit SHA as its files and audit
policy. Provider priority remains CLI --provider → ci.provider → module origin.
Use --template for an invocation-specific template override. Host config.json
contains image/docker and a legacy unused socket field only. The offline settings
query reads module YAML and configured origin through host Git, without API calls,
tokens, template fetching or evaluation; the actual operation
loads the selected package and resolves its default CI.

The template manifest declares provider-specific entrypoints:

```yaml
ci:
  gitlab: ci/puppet-module.yml
  github: .github/workflows/puppet-module.yml
```

GitLab modules use a project include on the same server, or an anonymous HTTPS
remote include for public GitHub templates, pinned to the selected SHA. GitHub
modules use the reusable workflow on the same GitHub server. Other cross-server
combinations fail explicitly; select compatible CI with --ci or ci.use.
Missing template CI for the selected provider fails unless CI is explicitly supplied.

The selected template owns contracts, generated files and audit scripts. A team
may require executable-entity specs and 100% resource coverage through its policy. Contract
switches are accepted only when that template enables allow_contract_overrides.
There is no implicit template, no bundled team selector and no team policy in the
hosting adapters. External templates support anonymous or separately authenticated HTTPS; see [private template access](TEMPLATES.md#private-template-repositories).

Commit .sync.yml and .sync.lock.json with the module. Review sync conflicts before
retrying with approved --files paths. The inventory records resolved revision and
generated hashes; unchanged retired files can be removed, modified files require
review, unowned domain files are preserved. Templates contain trusted ERB/scripts.
A version tag can move; a full SHA is immutable. Read `docs templates` for the schema.

## Launcher and execution environment

Workflow code, audit implementation, command catalog and docs are shipped by pmtool.
The environment image contains executables/dependencies only. Docker execution
uses a JSON/ERB worker and template-owned Puppet/RSpec checks; Go starts VoxBox
directly from the host. Settings, planning, checkout, sync and general audit are Go-owned.
Native CI uses the same plan with preinstalled job tools and no nested Docker.
The selected team template retains its own contracts/audit policy and CI entrypoints.
Pin launcher and image versions independently. See [manual](manual.md).

## Native CI integration

Use a pinned pmtool binary in each validation job. Verify its SHA256SUMS entry
before execution. VoxBox jobs use pmtool --executor native voxbox --stage validate|unit. Audit uses the tool-only SDK
image; release checks use VoxBox. Native module-build uses installed jig; forge-publish runs on the host without
--executor native. No Docker daemon/socket or source helper install is needed in
these jobs. The image must already contain the selected tools.

A team template can define its own shared installation include and variables
such as PMTOOL_VERSION, PMTOOL_RELEASE_BASE and TOOLING_IMAGE. These names are not
universal pmtool runtime settings. The generated starter instead embeds the
selected launcher release/download URL and compiled SDK image pins in its YAML;
edit/version that generated CI when changing them. In the launcher source
Actions workflow, vars.TOOLING_IMAGE selects WORKFLOW_IMAGE for SDK regressions
only, independently of the IMAGE value compiled into release binaries.
Private template reads require explicit
host-scoped template credentials. When using GitLab CI_JOB_TOKEN, configure the
template repository's inbound job-token allowlist and use gitlab-ci-token as the
username; do not assume access merely because both projects are on one server.

## Logs and generated validation CI

`pmtool pipeline-logs` supports GitLab job traces and GitHub Actions job logs.
Select --ref or --pipeline ID, optionally --job ID, --failed and --tail (default
100, 0 full bounded output). --output exclusively creates a new private file.
Job membership is checked before downloading. GitHub's signed HTTPS log redirect
is downloaded without provider headers/cookies; general API redirects remain
refused. Redaction covers known launcher tokens, not arbitrary CI secrets.
See [log selection and limits](manual.md#pipeline-logs). Failed CI in mr/release-tag
prints failed-job tails without weakening merge, exact-main or publication gates.

`template-init` generates provider-specific validation YAML and a separate template
self-test workflow. It has no implicit company runner tags or Forge publishing.
Private GitLab templates still need CI_JOB_TOKEN inbound authorization. The
starter GitHub workflow declares optional TOOLING_TEMPLATE_* secrets, but the
current generated module caller forwards only FORGE_URL/FORGE_PUBLISH_TOKEN.
Those optional template secrets are not populated automatically: private template
audit needs explicit credential provisioning in the selected CI environment.
Do not assume setting repository secrets alone forwards them through this caller. Add protected tag/exact-main checks and an actual
forge-publish implementation before adopting full release-tag publication.

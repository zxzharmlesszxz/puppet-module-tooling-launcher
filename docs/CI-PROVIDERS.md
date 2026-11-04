# GitLab and GitHub CI

One launcher and runtime support GitLab and GitHub. Commands operate on the mounted
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

```sh
pmtool mr --provider github --module /absolute/module
```

GitLab uses `GITLAB_TOKEN`, with optional `GITLAB_BASE_URL` and `GITLAB_USERNAME`.
GitHub uses `GITHUB_TOKEN`, with optional `GITHUB_API_URL`. The GitHub API defaults
to `https://api.github.com` or `https://<enterprise-host>/api/v3`; an override must
remain on the origin server. SSH ports are never reused as API ports.

The launcher forwards only the selected provider's token, by environment variable
name. Token values are absent from argv, config, hints and dry-run output. Local
sync/audit/tests receive no token; remote commands receive no Docker socket.
Git uses noninteractive HTTPS askpass with the selected provider's credentials.

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

The Example template repository contains both `ci/puppet-module.yml` and
`.github/workflows/puppet-module.yml`. To use Actions, publish the workflow in the
chosen GitHub CI repository and select its actual version; this change does not
create a repository, mirror, tag, GitHub ruleset or remote secret automatically.
The supplied workflow uses Ubuntu Docker-capable runners; Enterprise teams need
to adapt `runs-on` in their shared workflow for available self-hosted runners.

Branch pushes run PDK validation/unit and VoxBox validation/specs. Tag pushes verify
the protected metadata-matching tag, current main SHA and the successful main
workflow plus all four validation jobs. Release/changelog checks follow the gate;
Forge publishing follows those checks. Configure Forge secrets on consuming repositories.
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

The launcher and container are separate repositories. Teams publish their own
template packages on compatible Git hosts. The container has no bundled team
template. The repository/version below is illustrative:

```yaml
template:
  use: 'git.example.com/team/puppet-template@v1.0.0'
```

CI priority: **CLI --ci → module .sync.yml ci.use → selected template.yml ci**.
The template's CI is pinned to the same resolved commit SHA as its files and audit
policy. Provider priority remains CLI --provider → ci.provider → module origin.
Use --template for an invocation-specific template override. Host config.json
contains image/docker/socket only. The offline settings query reads module YAML
without tokens, socket, template fetching or evaluation; the actual operation
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
uses a JSON/ERB worker and template-owned Puppet/RSpec checks; Go starts PDK/VoxBox
directly from the host. Settings, planning, checkout, sync and general audit are Go-owned.
Native CI uses the same plan with preinstalled job tools and no nested Docker.
The selected team template retains its own contracts/audit policy and CI entrypoints.
Pin launcher and image versions independently. See [manual](manual.md).

## Native CI integration

Use a pinned pmtool binary in each validation job. Verify its SHA256SUMS entry
before execution. PDK jobs use pmtool --executor native pdk --stage validate|unit;
VoxBox jobs use the corresponding voxbox command. Audit uses the tool-only SDK
image; release checks use VoxBox. No Docker daemon/socket or source helper install
is needed in these jobs. The image must already contain the selected tools.

The team template's reusable CI can share one launcher installation include.
Its PMTOOL_VERSION and PMTOOL_RELEASE_BASE select the public release. Configure
TOOLING_IMAGE for audit/template-test jobs. Private template reads require explicit
host-scoped template credentials. When using GitLab CI_JOB_TOKEN, configure the
template repository's inbound job-token allowlist and use gitlab-ci-token as the
username; do not assume access merely because both projects are on one server.

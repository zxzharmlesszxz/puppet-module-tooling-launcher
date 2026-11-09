# PMTOOL_TEMPLATE_NAME

A schema-1 pmtool template starter. Edit this package to define your team's
baseline, contracts, audit checks and CI. No company repositories, keys, package
names or paths are included.

```sh
pmtool template-test --module .
```

This renders a temporary module and checks baseline integrity, then runs
`test/run.rb` with the documented JSON fixture. It does not run `audit.checks`
scripts automatically; exercise them explicitly in `test/run.rb`. It does not
modify a real module. Generation uses no Docker or network; Git is needed only
with --origin. By default, template-test uses the configured tools image for Ruby/ERB;
`--executor native` uses the job's installed tools instead.

## Layout and customization

- `template.yml`: schema version, files root, defaults, contracts, CI and audit policy.
- `config_defaults.yml`: one mapping per managed output, with ERB parameters.
- `moduleroot/`: matching `OUTPUT.erb` sources. The starter manages `.editorconfig`.
- `test/run.rb`: meaningful assertions against `PMTOOL_TEMPLATE_FIXTURE` JSON.
- `docs/SCHEMA.md`: authoring contract and examples.
- Provider CI: reusable module validation and separate template self-tests.

All other module files remain module-owned. The starter enforces semantic module
versions, baseline integrity and no tracked ignored files. It does **not** implement
Puppet contract usage, spec inventory or resource coverage enforcement. Add your
tested helpers and policies before claiming those guarantees; see `pmtool docs templates`.
Extend the test suite whenever you add or change a contract or audit rule.

## CI and publication

Generated CI pins the generating stable pmtool release, verifies SHA256SUMS, and
uses stock OpenVox 8 VoxBox. It does not use curl or nested Docker.
Module branch pushes run audit, VoxBox validation (validate, lint, rubocop) and specs; duplicate MR/PR validation
pipelines are not added. GitHub callers use workflow_call; the template's own push
workflow runs template-test. GitLab has separate root and reusable YAML files.
Choose runner tags/permissions for your installation. Private template checkout
requires an explicitly scoped read token; on GitLab, CI_JOB_TOKEN additionally
needs inbound access to the template repository. The starter GitHub workflow declares optional TOOLING_TEMPLATE_* secrets.
The current generated module caller forwards only Forge secrets, so those optional
template secrets are not automatically supplied. Forge secrets are declared
optional for caller compatibility and are not passed to validation jobs. Private template audit requires
explicit credential provisioning in the selected CI environment; merely adding
repository secrets does not forward them through the generated caller.

This is a **validation starter**, with no Forge upload or placeholder publishing
job. Before using `pmtool release-tag`, add a reviewed tag workflow that checks
the protected tag against metadata and current main, verifies successful required
main jobs for that exact SHA, performs release checks, and publishes through an
actual successful `forge-publish` job. Keep credentials in protected CI secrets.
Until then use validation only; pmtool refuses unverified publication.

## Connect and publish the repository

Create an empty GitLab/GitHub repository for this template, matching the selected
CI hosting. Supplying --origin URL to template-init initializes Git/main and
adds origin locally, without commits, tags, network access or pushes. The option
does not create the hosted project or change --provider. It accepts credential-free
HTTP(S), SSH and SCP-style repository addresses. SSH origin setup does not enable
SSH template fetching. Preview records Origin without requiring Git; a Git setup
failure preserves generated files and any partial .git for inspection.

If generation omitted --origin, no local Git or remote was created. From this
package directory, replace the following example with its actual repository URL:

```sh
git init --initial-branch=main
git remote add origin https://git.example.com/team/PMTOOL_TEMPLATE_NAME.git
git remote -v
```

Use the template's own URL, not a module or launcher URL. For GitHub, use its
GitHub repository URL. If Git/origin already exists, inspect it instead of
repeating initialization or replacing a remote blindly. Review and test the files,
commit them, push main with its upstream and wait for CI on that commit. Then
publish an annotated version tag and wait for any configured tag CI. These are
template Git release steps. For subsequent template releases, use `pmtool mr`
from the pushed source branch to wait for CI and required review, merge, and
synchronize local main. Private template and CI access for modules is separate.

Publish the reviewed template with an immutable Git tag before selecting it:

```yaml
template:
  use: 'git.example.com/team/PMTOOL_TEMPLATE_NAME@v1.0.0'
```

Put that selection in the consuming module's `.sync.yml`. CI defaults to the
selected template's exact resolved revision. An explicit `ci.use` or `--ci`
overrides it. For GitHub-hosted modules, use a GitHub-hosted reusable workflow.
GitLab project includes require the same GitLab server; see `pmtool docs templates`
for the supported public GitHub-to-GitLab remote include exception.

Keep Git tags and changelog as the version source; no VERSION file is generated.
The generator initializes Git only with explicit --origin; it never commits or pushes.

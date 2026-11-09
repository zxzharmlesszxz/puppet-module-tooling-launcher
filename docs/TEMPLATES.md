# Versioned module templates

The toolkit supplies lifecycle, rendering, safe file updates and audit dispatch.
A selected template supplies the generated baseline, contract selection and audit
policy. Hosting providers and reusable CI are configured independently.

## Create a template

```sh
pmtool template-init --name team_template --output ./new-template --provider both
pmtool template-test --module ./new-template
```

NAME starts with a lowercase ASCII letter and contains only lowercase ASCII
letters, digits, underscores or hyphens; module-new names do not allow hyphens.
Generation is offline and needs no module, image config or Docker. Git is needed
only when --origin is supplied; omission preserves generation without Git. The new
directory contains `template.yml`, defaults, a configurable `.editorconfig` ERB,
`test/run.rb`, `docs/SCHEMA.md`, changelog and selected provider CI. Use gitlab,
github or both; both is the default. Its parent must exist without symlink
components. Existing destinations are refused; --dry-run lists planned files only
and does not preflight destination availability. New directories/files use
0700/0600. A write failure can leave an incomplete new tree; inspect it before
cleanup or generate into a fresh destination.

The generated package is a neutral validation starter. It does not pretend to
provide team contracts, entity spec inventory, 100% Puppet coverage or Forge
publication. Add and test those policies explicitly. Generated branch CI runs
audit and VoxBox validation/specs; template self-tests have their own workflow.
There are no duplicate MR/PR jobs, custom runner tags, nested Docker or socket mounts.
Review runner requirements, private-template credentials and protected release
policy before adoption. Add actual protected-tag/exact-main checks and a working
forge-publish job before using the full module release workflow.

CI pins the generating stable launcher and verifies downloaded binaries against
SHA256SUMS. Development builds require --launcher-version naming a published
stable release. The generator checks version syntax, not release availability
or compatibility; choose a published launcher that supports the generated CI.
Bootstrap uses Ruby from PATH in VoxBox.
Generated self-tests render and check temporary baseline integrity via pmtool,
then assert the JSON fixture in Ruby; they never import launcher implementation files. Add behavioral
regressions as team helpers grow. Validate both provider variants when publishing
an updated starter. Git initialization, commits, tags and pushes remain explicit.

### Connect and publish the repository

Create an empty repository for the template on GitLab or GitHub, matching the
chosen CI hosting. Pass its actual URL during generation to initialize an ordinary
Git repository on an unborn main branch and add origin, without staging, commits,
tags, network access or pushes:

```sh
pmtool template-init --name team_template --output ./new-template --provider gitlab --origin https://git.example.com/team/puppet-template.git
```

--origin is optional and does not change --provider (still both by default). It
accepts credential-free HTTP(S), SSH URLs and Git's SCP-style addresses; namespace
and repository are required. Local paths, file URLs, embedded credentials, query,
fragment, whitespace and traversal are refused before writing files. An SSH URL
only configures the remote; pmtool template fetching still uses HTTPS. No hosted
project is created and remote existence/access is not checked.

--dry-run validates the URL and includes Origin in the JSON plan without invoking
or requiring Git or creating files. Apply checks Git availability before generation.
Git init/remote add ignore caller Git directory/config overrides and use SHA-1 for
the current template reference format. If Git setup fails after generation, the
files and any partial .git are preserved for inspection; do not regenerate over
that directory. Existing output is never reused or overwritten.

If generation omitted --origin, enter the new package and connect its repository
manually; replace this example with its actual URL:

```sh
cd new-template
git init --initial-branch=main
git remote add origin https://git.example.com/team/puppet-template.git
git remote -v
```

For GitHub, use the template repository's GitHub URL. origin must identify this
template, not a consuming module or the launcher repository. If Git/origin already
exists, inspect it rather than repeating initialization or replacing it blindly.
Creating a project on the server does not configure the generated directory.

Review and test the package, commit the reviewed files, then push main with its
upstream. Wait for template CI to pass for that commit. Publish an annotated
version tag such as v1.0.0 from the reviewed commit and wait for any configured tag
CI. Only then select that published repository/version in module .sync.yml.
Keep tags and changelog as the version source; do not add a VERSION file.
Template publication uses explicit Git/hosting operations; the module-facing
release-prepare/release-tag commands do not publish template repositories.
Private template/CI access must also be configured for consuming modules.

## Select a template

Track the selected package in module `.sync.yml`. After publishing the version:

```yaml
template:
  use: 'git.example.com/team/puppet-template@v1.0.0'
```

`--template` overrides `template.use` for one invocation. There is no implicit,
bundled or host-level team template. An external reference supports a version tag or full
40-character commit SHA; branches such as `main` and `HEAD` are rejected.
Use `host/namespace/repository//templates/team@version` for a package in a subdirectory.
Tags can move; a full SHA pins an immutable revision.

The Go engine fetches the exact tag/SHA into a temporary directory and removes it
afterwards. Archive links, devices and traversal paths are rejected. No fleet,
local template cache or team checkout is needed. The offline launcher `settings`
query reads `.sync.yml` and uses host Git for configured-origin/provider
inference, without API access or template evaluation. Sync/audit fetch and evaluate
the selected package.

Templates contain ERB and executable audit scripts. Select reviewed, trusted
versions. A repository reference is not a sandbox for untrusted template code.

## Private template repositories

Template authentication is resolved automatically in this order:

1. Explicit `TOOLING_TEMPLATE_TOKEN`, scoped by `TOOLING_TEMPLATE_TOKEN_HOST`.
2. The host's configured Git credential helper for the selected HTTPS repository.
3. Anonymous HTTPS when the helper has no saved credentials.

With ordinary saved Git credentials, run `pmtool sync`; no template environment
variables are required. The lookup uses `git credential fill` with the exact
canonical repository URL and respects the user's global/system helper settings,
including `GIT_CONFIG_GLOBAL`. Host-wide credentials are accepted when Git is
configured that way, but pmtool binds their use to this one repository. Lookup
runs in the fresh temporary checkout, not the module's local Git configuration.
Git tracing, askpass programs, terminal prompting and common credential-manager
interactive prompts are disabled; lookup is bounded to 15 seconds. Configured
credential helpers are trusted host programs, not SDK code.

For an explicit override, put a read-only repository token in
`TOOLING_TEMPLATE_TOKEN` and its bare host in `TOOLING_TEMPLATE_TOKEN_HOST`.
Optional `TOOLING_TEMPLATE_USERNAME` selects the HTTPS/deploy-token username;
its default with an explicit token is `oauth2`.

```sh
export TOOLING_TEMPLATE_TOKEN_HOST='git.example.com'
pmtool sync
```

The override token must already be in the environment. Never put its value in
`.sync.yml`, `config.json`, arguments or documentation. A configured token with
a mismatching host fails before helper lookup; it does not silently fall back.
Module `GITLAB_TOKEN`/`GITHUB_TOKEN` values are not reused as template credentials.
The team's GitLab CI setup may explicitly map its permitted `CI_JOB_TOKEN` into
the template override variables; provider-side access still needs authorization.

The catalog grants `template_token` only to `sync`, `sync-noop`, `audit`, `check`
and their aliases. Metadata, settings, docs, template-init/test, VoxBox-only and
module API/release operations do not invoke template credential lookup.

Host helper lookup performs only `get`, never `approve`/`reject` or `store`/`erase`.
The subsequent fetch disables host helpers, hooks and redirects and uses pmtool's
secret-free temporary helper, which answers only HTTPS `get` for the exact selected
host/repository path. The secret itself stays in memory and a narrowly scoped
fetch environment, not helper source, argv, Git config or module files. Credentials
are removed from ERB/audit/SDK environments, and fetch errors redact raw,
URL-encoded and Basic-auth forms. Templates remain trusted executable code.

Automatic helper lookup is a launcher change under development; published
pmtool v2.0.0 still requires explicit overrides for private templates.

Fetching the package does not authorize provider-side CI includes. Private GitLab
project includes and private GitHub reusable workflows still need the provider's
own repository/job access. An anonymous cross-server `include: remote` cannot use
this token. A template version must be committed and published before selecting it.

## Package layout

```text
template.yml
config_defaults.yml
moduleroot/
  types/package.pp.erb
  spec/type_aliases/package_spec.rb.erb
  spec/support/team_audit.rb.erb
```

Each defaults key names an output path with an `.erb` source under `files_root`.
`%{namespace}` is substituted in output paths using the module metadata name.
ERB receives `@configs` and `@metadata`. Per-file defaults and module overrides
merge by top-level key: nested mappings and arrays are replaced, not deep-merged.
This is template-rendering configuration, independent of Puppet Hiera merge policy.
Module files outside the managed defaults remain module-owned.

Minimal manifest with a package contract and a team audit script:

```yaml
schema: 1
name: team
files_root: moduleroot
defaults: config_defaults.yml
contracts: [package]
contract_groups:
  package:
    files:
      - types/package.pp
      - spec/type_aliases/package_spec.rb
    types: [package]
audit:
  metadata:
    semantic_version: true
    changelog:
      file: CHANGELOG.md
      heading: 'Release %{version}'
  tracked_ignored: true
  checks:
    - name: team audit
      path: spec/support/team_audit.rb
      runtime: ruby
```

Matching defaults:

```yaml
types/package.pp:
  unmanaged: false
spec/type_aliases/package_spec.rb:
  unmanaged: false
spec/support/team_audit.rb:
  unmanaged: false
```

`contract_groups` defines available groups; `contracts` selects enabled ones.
Omitting `contracts` enables all declared groups; `contracts: []` enables none.
Every group owns a nonempty, nonoverlapping list of defaults paths. `types` names
are supplied to template-owned checkers. Excluded groups generate no files.
To use only system repositories, publish/select a profile without the repository
group in `contracts`, rather than adding `repository: false` to every module.
Templates may expose existing group switches with `allow_contract_overrides: true`;
it defaults to false. A team profile can explicitly retain this compatibility.
Domain references to disabled contracts must be removed in module code separately.

## Audit policy

All policy fields are optional. `metadata` supports semantic version validation,
the changelog filename/heading, `forbidden_keys`, `url_fields` and `no_self_symlinks`.
`tracked_ignored` enables tracked-ignored Git file detection. `allow_custom_files`
allows a reviewed non-contract file override with `custom: true, unmanaged: true`.
Core path, conflict, ownership and CI integrity guards remain enforced.

An optional `coverage` mapping names a managed helper and its config keys:

```yaml
audit:
  coverage:
    file: spec/spec_helper.rb
    enabled_key: coverage_report
    minimum_key: minimum_code_coverage_percentage
    minimum: 100
```

That helper's defaults must enable reporting and set the exact threshold.
The helper must implement measurement/enforcement; the engine checks configuration
and baseline integrity. Team packages can supply resource coverage, spec inventory and contract usage
helpers. The generated starter supplies none of those checks by default.
Another team can supply different helpers and policies without changing toolkit code.

Each `checks` item names a managed script, a display name and `runtime: ruby` or
`runtime: voxbox`. The engine runs it with the module directory as its argument;
a nonzero exit fails audit. VoxBox uses the bundled Puppet Ruby environment.
Checks run only after file/policy validation succeeds. CI caller files are rendered
by the engine from `ci.use`; templates cannot own `.sync.yml`, `.sync.lock.json`,
`.gitlab-ci.yml` or `.github/workflows/puppet-module.yml`.

## Apply and upgrade

Prepare the bootstrap/release branch before changing `.sync.yml`. Run `sync`;
`sync-noop` remains optional preview. On a conflict, inspect the shown diff and
retry with only the reviewed replacement/removal paths in `--files`. That option
is one whitespace-separated string, not a shell list; filenames containing spaces
cannot be approved through it. Do not delete managed files or edit ownership hashes
to bypass a conflict that needs a supported selection.

Sync writes `.sync.lock.json` with the selected reference, resolved revision and
SHA256 hashes of generated files. Commit it with module source. It is an owned-file
record, not a dependency cache or a replacement for an immutable template reference.
Audit checks the inventory against the currently selected package.

Files retired by a new template are removed only if unchanged from the previous
record. Modified files require explicit review; unrelated domain files remain.
Sync preserves snapshot/race checks and refuses symlinks/nonregular output files.
It does not rewrite metadata, fixtures, Hiera or domain implementations. Audit
never applies changes. Continue with `check`, `release-submit`, `mr`, `check`,
`release-tag` using the normal branch/CI/publication guards.

## CI from the selected template

The manifest may declare provider-specific CI files (actual YAML, not ERB):

```yaml
ci:
  gitlab: ci/puppet-module.yml
  github: .github/workflows/puppet-module.yml
```

These paths are relative to the package root and must exist in the selected Git
archive. Priority: --ci, module ci.use, then manifest ci for the effective provider.
Default CI uses the package repository and exact resolved commit SHA, including
when template.use names a tag. An explicit CI reference retains its own chosen ref.
An omitted/missing provider entry fails unless the module supplies an explicit CI.

The engine generates GitLab project includes for the same server. For a public
GitHub template consumed by GitLab it emits a remote include with this shape:

```yaml
include:
  - remote: 'https://raw.githubusercontent.com/OWNER/REPO/FULL_SHA/ci/puppet-module.yml'
```

GitLab fetches that file anonymously; use self-contained YAML without private or
relative local includes. Module API credentials are never sent to the CI host.
GitHub uses a reusable workflow on the same GitHub server, under .github/workflows
at repository root. A template in a subdirectory cannot place its reusable GitHub
workflow under that subdirectory: use a root template package or explicit --ci.
Other cross-server combinations are refused instead of producing an invalid caller.
The generated GitHub caller forwards Forge secrets only. Optional template secrets
declared by the starter are not automatically passed; private-template audit
needs explicit credential provisioning in the selected CI environment.

The launcher settings query remains offline and does not resolve this fallback.
Only the actual runtime sync/audit command fetches the template. The launcher
contains no team contracts or deployed CI package. Its neutral starter assets generate an editable validation package; actual team policy stays in its
versioned repository.

## Launcher and execution environment

Workflow code, audit implementation, command catalog and docs are shipped by pmtool.
The environment image contains executables/dependencies only. Docker execution
runs the JSON/ERB worker and verified template-owned checks in the tools image;
Go starts VoxBox directly from the host.
Native CI uses the same plan with preinstalled job tools and no nested Docker.
The selected team template retains its own contracts/audit policy and CI entrypoints.
Pin launcher and image versions independently. See [manual](manual.md).

## Template-test provider scope

The smoke test renders one temporary module baseline: GitLab is selected when
the package declares GitLab CI, otherwise GitHub. It does not automatically run a
GitLab/GitHub matrix, execute provider-side pipelines or invoke audit.checks
scripts. Its Go self-check verifies baseline/policy; test/run.rb must exercise
custom scripts and domain/helper behavior explicitly; validate each CI provider in
its actual environment before claiming compatibility. The optional team-template
regression in workflow-check needs PMTOOL_TEST_TEMPLATE inside its SDK process.
A skipped test does not qualify that external package.

## Engine and template-test interface

The Go engine owns reference/schema validation, checkout/authentication, policy and
contract selection, CI selection, ownership hashes, preview/apply and general audit.
It sends ERB source, configs and metadata as JSON to a small Ruby worker. Existing
ERB keeps `@configs`, string-keyed metadata and `@metadata[:module_name]`. Contract
group order and symbol keys inside group entries are preserved for generated Ruby.
No team contract or audit threshold is hardcoded in the container or Go engine.

`template-test` renders and checks a temporary module baseline in Go before running
the template's `test/run.rb`. `PMTOOL_TEMPLATE_FIXTURE` identifies a temporary JSON
file with `defaults`, `group_order`, `groups`, `contracts`, `allow_contract_overrides`
and `profile_source`. Docker mounts this fixture read-only; native execution
uses ordinary job-user file permissions. Group entries contain `targets`, `types` and
`legacy_optional_targets`. Tests can load the standalone module-side profile source;
they must not import launcher implementation files or assume a toolkit install path.

General sync/audit regressions live in Go. Template tests retain their own ERB,
Puppet/RSpec parser, resource coverage, repository-key and CI behavior checks.

Lint the actual rendered Ruby, including profile initialization and contract
functions, against the supported module RuboCop styles. A passing engine-asset
lint does not validate ERB output. Serialize profile configuration as quoted JSON
data rather than interpolating Ruby `Hash#inspect` into executable source.
Do not bypass failures by excluding generated helpers or all of `spec/**/*`.


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

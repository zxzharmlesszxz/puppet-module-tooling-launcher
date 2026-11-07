# pmtool development

## Source ownership

cmd/pmtool is the executable entry point. internal/pmtool contains CLI/configuration,
Go validation plans, process execution, output, AI export and completion.
internal/hostflow owns Git, REST providers, release operations and their regressions.
internal/templatekit owns template checkout/authentication, schema and policy loading,
CI selection, baseline rendering requests, sync, ownership and general audit.
internal/fixturekit owns host-side fixture downloads, archive verification and
temporary fixture manifests. internal/scaffold owns neutral starter assets and
provider-specific CI generation.
internal/workflow contains only the embedded command catalog. docs contains the
embedded manuals and assistant guidance; test contains Make target regressions.

Pinned upstream VoxBox supplies the runtime tools and dependencies. The separate
Dockerfile.checks builds development-only Go/lint/security tools; it is not a
custom module runtime and is not used by module audit or template jobs.
Team templates supply contract files, audit policy, Puppet/RSpec checks and CI.
The launcher contains no second workflow implementation.

## Runtime boundary

Settings and validation sequences are constructed in Go. Git/API commands bypass
Docker and image configuration. Template checkout runs host Git with version tags or full commit SHAs and
explicit overrides or stored host Git helper credentials scoped to the selected HTTPS repository; the resolved SHA is recorded, but
a version tag itself can move; temporary files are removed
on success and failure. Sync and general audit execute in Go on the selected clone.

The small ERB worker receives JSON documents through stdin and returns JSON strings.
It only requires Ruby's JSON and ERB libraries. The Docker executor runs it in the
pinned tools image with a read-only root, no network and no host source mount.
In Docker mode, template-owned audit scripts receive a read-only module mount
and the required Ruby or VoxBox bundle, without provider/template credentials
or a Docker socket.
Native CI uses installed job tools directly and is not a filesystem sandbox.

The remaining Ruby assets are the ERB worker and the standalone contract-profile
helper emitted into modules for their RSpec/AST checks. The latter preserves the
selection interface; its generated layout is versioned with pmtool. A real-Ruby regression compares its
selection results with Go; it is not an alternate launcher backend. Puppet/RSpec
parsing and coverage remain in template-owned checks using the actual runtime APIs.

VoxBox are started directly through the host Docker CLI. Go preserves child
exit codes, runs checks in order and stops at the first failure. There is no Ruby
planner, settings RPC, archived helper bundle or old engine retained for tests.

Template-init --origin is explicit local Git setup, not module release or API
publication. It validates a credential-free remote, checks Git before writing,
initializes an unborn main and adds origin. Caller Git directory/config overrides
and credentials are isolated; Git templates are disabled. Preview/default generation
invoke no Git. Post-generation Git failures preserve the new tree for inspection.

## Documentation checks

Embedded docs are checked against every catalog command/option, including aliases.
Documented pmtool docs --section examples must resolve locally to a unique section.
Link/anchor checks are separate from content/behavior review. A green launcher test
suite does not prove team CI or a skipped external template integration passed.
Generated starter CI must declare every secret forwarded by the engine caller and
publish a newline-terminated GITHUB_PATH entry; regressions exercise both contracts.
Formatting targets recurse through cmd/internal/docs, including internal/scaffold.

## Checks

```sh
make format
make check
make full-check
make build VERSION=v2.1.0
```

Go checks include format, vet, golangci-lint, coverage threshold, race tests,
govulncheck, gosec and actionlint. The workflow-check target compiles the same Go template test suite and runs it
inside the selected SDK with real ERB, then applies strict RuboCop to the remaining
Ruby assets. Helper assembly regressions also enforce one top-level profile class
with a nested group-selection helper and check the generated class boundaries
and preserve module template/CI settings. Template suites must lint the actual
rendered Ruby under supported module styles; linting ERB or engine assets alone
does not qualify the output. Unit tests cover parser-independent rules without
requiring Ruby. No scanner rules are weakened.
Security scans include Trivy filesystem/license/secret/misconfiguration checks.
The scanner is pinned to Trivy 0.76.2 and its multi-platform image digest in
Makefile through TRIVY_IMAGE. Scanner-only updates can be verified with
`make trivy-filesystem`; they do not require rebuilding pmtool binaries.
WORKFLOW_IMAGE defaults to pinned upstream VoxBox for OpenVox 8. Builds do not need
a sibling environment checkout or a custom runtime image.

Cross-builds copy the preloaded Go modules into a private writable temporary
cache. Builds stay offline under the invoking UID/GID and preserve the image-owned
cache. Use `make build VERSION=v2.1.0` to verify a release; plain `make build`
intentionally produces `dev` artifacts.

Release regression tests use temporary local bare Git remotes and a fake gh.
Workflow tests must never perform real GitLab/GitHub publication. Verify configured
origin reads with real local/included Git URL rewrites and both providers. Cover
stored-credential rejection even when a rewrite conceals those credentials, ambiguous
URLs, unchanged-origin guards and changed-origin refusal. Template Git must refuse
HTTP authentication noninteractively without caller askpass programs or a platform
specific executable. Also verify origin parsing, private template host boundaries,
exact refs/SHA, key/cache ordering,
literal argv, no socket, per-step token/author scope and partial-failure recovery.
Keep raw Markdown tables aligned and regenerate/review assistant exports after changes.

## Versions and source publication

Launcher, environment, template and module versions are independent. There is no
VERSION file. These Make targets operate on this launcher's GitHub source clone;
Puppet modules use pmtool's separate module workflow. Host Git/GitHub CLI are
required; build/test tools still run in their existing containers. Authentication
uses Git/GitHub CLI configuration, never tokens embedded in Make arguments.

```text
release-prepare → edit → check → release-submit → mr → review CI binary → push-release
```

```sh
make release-prepare VERSION=v2.1.0
make check
make release-submit VERSION=v2.1.0 MESSAGE='Describe the implementation'
make mr VERSION=v2.1.0
```

Prepare requires an ordinary committed clone and clean worktree, synchronizes main
without reset/stash, verifies that the new canonical version exceeds main's latest
changelog release, rejects existing local/remote branch or tag names, then creates
`release/2.1.0`. Do not repeat it on an edited release branch. Optional
`release-prepare-noop` previews from already synchronized clean main.

Put implementation notes under `## Unreleased`. Submit automatically stages the
reviewed source files, commits with MESSAGE, then creates a separate
`chore(release): prepare v2.1.0` commit whose only change is finalizing that heading
to `## Release 2.1.0`. It pushes the branch with tag following disabled. VERSION
must match the branch. Existing tags are never reused. Same-version corrective
commits on an unmerged branch are supported; already finalized notes do not create
empty preparation commits. `release-submit-noop` shows selection/diff without
staging, commits, heading changes or pushes. Optional release-push-noop and
release-tag-noop preview their respective operations with the normal guards.

Individual operations remain available:

```sh
make release-commit VERSION=v2.1.0 MESSAGE='Describe the implementation'
make release-finalize VERSION=v2.1.0
make release-push VERSION=v2.1.0
```

Failures preserve commits and staging. After push failure, retry release-push or
release-submit; they do not recreate the reviewed commits. A signing failure during
finalization leaves the prepared changelog for inspection; complete that commit
explicitly before resubmitting. These targets never force-push a branch.

`mr` creates/reuses one open PR into main, takes its body from the release notes,
waits for the push workflow of the pinned source SHA, rechecks local and PR state,
and merges with GitHub CLI's `--match-head-commit` guard. It then runs main-sync
and ci-artifacts. Failed CI, changed PR SHA/base/branch, local drift, API errors or
ambiguous PRs stop the operation. If main synchronization fails after merge,
preserve the merge and retry `make main-sync`, followed by `make ci-artifacts`.
No branch deletion, tag push or publication is implicit in mr.

`main-sync` permits only clean work whose HEAD and local main are ancestors of
origin/main. It fetches and fast-forwards; unmerged commits or divergence require
explicit reconciliation. It never resets, stashes or discards work.

Read source CI without manually composing API requests:

```sh
make pipeline-jobs
make pipeline-jobs REF=main WATCH=5
make pipeline-logs REF=release/2.1.0 TAIL=100
make pipeline-logs REF=main RUN_ID=123 JOB=456 FAILED_ONLY=0
```

REF defaults to the current branch and resolves to its local commit. RUN_ID is
optional, numeric and checked against that SHA/ref/push event. WATCH=5 waits in
five-second intervals; WATCH=0 only shows status/jobs. Failed workflows return an
error. Logs default to failed jobs and the last 100 lines; FAILED_ONLY=0 includes
all job logs. TAIL must be positive and applies to the combined gh log output;
this differs from module pipeline-logs, where tail=0 is allowed and tail is per job.
JOB must be a numeric job ID in the selected run. API/log errors stay
fatal, and these read operations never mutate Git refs or publish anything.

Main artifact verification checks all binaries, SHA256SUMS, the full MIT license,
source/run identity and native version/help. Files stay under `dist/ci/<SHA>/`.
Test the downloaded binary with the team's template/module before stable tagging.
Standalone ci-artifacts retries download/verification; push-main supports an
already prepared committed clean main. Branch CI creates Actions artifacts only.
Existing different artifacts are preserved and rejected.

Only after reviewing those artifacts:

```sh
make push-release VERSION=v2.1.0
```

Publication still requires verified current-main artifacts, local preflight,
exact-main CI and an annotated immutable tag. It does not push main. Direct
release-tag (local tag only) and push-release-tag enforce the same artifact gate.
Tag CI checks main and its successful workflow. GitHub publication remains draft
until every upload succeeds; there is no rc.N/prerelease lane. Preserve tags after
failure, inspect/retry CI and resume push-release-tag for the identical tag.
Build/check/clean never publish. After clean, reverify downloaded CI artifacts.

## Stock VoxBox compatibility evaluation

The candidate is the pinned upstream VoxBox 10.1.0 release for OpenVox 8:

```text
ghcr.io/voxpupuli/voxbox:10.1.0-openvox8@sha256:01bb8ce5e24d5b111934660f75e2786b4d7c7adcaa7518a564856d1d3041d10d
```

Reading this local image's metadata/lockfile on 2026-11-03 confirmed Ruby 3.2.11,
Bundler 4.0.22, GitLab SDK 6.1.0, HTTParty 0.24.3, modulesync 4.6.1,
OpenVox 8.30.0, RuboCop 1.91.0 and voxpupuli-test 15.0.0.
These are recorded image contents from the pre-migration compatibility run.
Git/API regression tests no longer require the Ruby GitLab SDK. Provider tests
exercise the Go REST adapters directly; production API traffic uses net/http.

ERB uses standard Ruby libraries. Template-owned Puppet checks select the active
VoxBox Gemfile and bundle path explicitly; they do not assume the old toolkit
helper directory or a custom bundle configuration file. No Ruby engine/bootstrap
bundle is packaged by the launcher.

```sh
make voxbox-check
```

This runs Go template regressions with real ERB and strict Ruby-asset lint on stock VoxBox as the
invoking UID/GID with HOME=/tmp and a read-only source mount, then
Trivy on that image via an archive, without building a custom image, giving Trivy
a Docker socket, or performing real API publication. VOXBOX_IMAGE selects another
candidate. A failed helper suite stops the target. Upstream image vulnerability
findings are advisory: Trivy retains its detailed report and prints a yellow
`[WARN]` summary (plain text with NO_COLOR or redirected output), returning success.
A clean scan prints `[OK]`; image download, Docker or scanner errors still fail
the explicit scan target. No scan is added to normal pmtool commands. The
launcher source/Go security gates keep their existing strict behavior. This first
check covers the locally selected architecture; release validation must cover
the supported execution architectures as well.

### Observed results: 2026-11-03

The upstream latest tag still resolves to the candidate digest above. The local
Linux arm64 evaluation produced these results:

- All 15 Ruby helper suites passed, with the failed documentation-parser suite
  corrected and the remaining seven suites resumed separately.
- Strict RuboCop inspected 118 Ruby files with no offenses after the corrections.
- A temporary pmtool binary ran the real team template: all seven suites passed
  on stock VoxBox with no network or template credentials.
- Launcher Go tests passed; a temporary Darwin arm64 binary compiled successfully.
- Trivy failed the existing HIGH/CRITICAL gate: six OS package findings (one
  CRITICAL and five HIGH), one HIGH in resolv, and five HIGH in the bundled jig
  binary. These are scan findings, not a claim that every component is exercised
  by pmtool. This was the original blocking scan result; the upstream image scan
  now treats vulnerability findings as advisory warnings.

Functionally the stock image supplies the helpers' requirements. Findings in an
upstream image alone do not require maintaining a custom image or block pmtool
use. The selected candidate is 10.1.0-openvox8 (OpenVox 8.30.0), pinned by digest;
do not substitute latest or OpenVox 9 implicitly. OpenVox 9 support is planned
for a separate future pmtool release with compatibility tests and migration
documentation. The previous 10.0.0-openvox8
release uses OpenVox 8.28.1: all 15 runtime suites and seven template suites passed,
but its RuboCop reported 11 redundant keyword_init settings and its scan reported
one CRITICAL and 80 HIGH findings under the same filters. Linux amd64 execution
has not yet been tested. No source commit, release tag, push or publication was
performed during this evaluation.


## OpenVox 8 and 9: development-tool compatibility

Evaluation on 2026-11-03 compared the same launcher helpers, team template and
RSpec harness on Linux arm64. Production module code was outside this evaluation.

The upstream VoxBox release 10.1.0-openvox8 supplied the baseline. The upstream
build matrix still selected OpenVox 8 and no 10.1.0-openvox9 image existed. A
temporary local probe started from that exact baseline digest, preserving direct
gem versions except OpenVox, OpenFact and puppet-ghostbuster. This is compatibility
evidence for the tested dependency set, not a published upstream OpenVox 9 image.

| Component          | OpenVox 8 baseline | OpenVox 9 probe |
| ------------------ | ------------------ | --------------- |
| Ruby               | 3.2.11             | 3.2.11          |
| OpenVox            | 8.30.0             | 9.0.0           |
| OpenFact           | 5.7.2              | 6.2.1           |
| puppet-ghostbuster | 2.1.0              | 2.2.0           |
| RSpec-Puppet       | 6.0.0              | 6.0.0           |
| voxpupuli-test     | 15.0.0             | 15.0.0          |
| RuboCop            | 1.91.0             | 1.91.0          |
| GitLab SDK         | 6.1.0              | 6.1.0           |

The complete lockfile diff contained only those three replacements and two added
transitive gems: CFPropertyList 4.0.0 and nkf 0.3.0. OpenVox 9 requires OpenFact 6;
puppet-ghostbuster 2.1.0 excludes OpenVox 9, while 2.2.0 accepts both 8 and 9.
No other original gem changed, including the RSpec and lint toolchains.

The same code passed:

- All 15 helper regression suites on OpenVox 9 and strict RuboCop for 118 files.
- All seven team-template suites, including parser inventory, contracts and
  repository-key behavior, on OpenVox 9.
- An actual RSpec-Puppet catalogue compile with module Hiera on both 8 and 9.
- The 100% resource-coverage success case, rejection of an untouched resource
  (50% coverage), and rejection of a pending example on both versions.

No OpenVox-specific helper or template branch was required. A common pmtool
codebase and release with separate dependency environments and an 8/9 CI matrix
fits the observed differences. Keep 8 as the current default and make a future
9 lane explicit. Do not infer production-module compatibility or full PDK 9
support from this tooling test. An official VoxBox 9 image will need its own
qualification once available. No source commits, pushes or releases were made.


### OpenVox 9 on Ruby 4.0: follow-up verification

On 2026-11-03 the development-tool tests were repeated on Ruby 4.0.7, the Ruby
version shipped with the OpenVox 9.0.0 agent. This closes the Ruby-version gap
in the earlier gem-only test on Ruby 3.2.11.

The local probe used the official ruby:4.0.7-alpine image pinned to index digest
sha256:7fe9cfc6976912a171faaeb93d8c2770ef483c1622e45d9c88aeee626058fcca.
All gems were rebuilt for Ruby 4 from the same OpenVox 9 Gemfile.lock. Its SHA256
and every gem version remained identical to the Ruby 3.2/OpenVox 9 probe; no
additional dependency-version changes were required for Ruby 4.

The same helper/template code passed on Linux arm64:

- Ruby syntax checks and strict RuboCop for all 118 workflow Ruby files.
- All 15 helper regression suites, including Git/API operations, guards, quoted
  argv, bootstrap bundle isolation, template selection and terminal output.
- All seven team-template suites through pmtool's real Docker bootstrap.
- The actual RSpec-Puppet harness: catalogue compilation and module Hiera at
  100% resource coverage, rejection of a 50% coverage case, and rejection of a
  pending example. The negative cases intentionally returned failure statuses.

No Ruby 4-specific helper or template changes were needed. The evidence supports
one development-tool codebase with an 8/9 environment matrix rather than separate
source branches and release lines for this compatibility difference. Default
selection remains OpenVox 8. This local probe is not an official VoxBox 9 release
or a qualification of packaged agents, PDK, production modules or other CPU
architectures. Module source was mounted read-only. No commits, pushes or
publication were performed.

## Version 1.1.0 generator and log regressions

Version 1.1.0 introduced source workflow targets, configured-origin and portable
authentication fixes, neutral template generation and cross-provider logs. Its
published tag is immutable. Source submissions keep implementation and the
changelog-only preparation commit separate, with Unreleased notes finalized by
release-finalize for the selected version.

internal/fixturekit owns host-side fixture downloads, archive verification and
temporary fixture manifests. internal/scaffold owns neutral starter assets,
generated provider validation CI
and exclusive new-directory writes. No deployed team template is embedded.
Tests check provider selection/schema, conflicts/symlinks/traversal, Git-free
default generation, Docker/config isolation, real ERB sync/audit and the Ruby fixture
suite. Origin regressions use actual temporary Git repositories and test unborn
main, exact remote storage, caller environment/parent isolation and Git failures;
no real provider, commit or push is involved.
The generated validation CI intentionally omits publication; do not fake green
forge-publish or silently adopt team contracts.

Host log regressions use local HTTP/TLS fixtures only: both API endpoints,
credential-free signed redirects, unsafe redirects/status/size/cancellation,
project/pipeline/job identity, tail/filtering, redaction and exclusive file output.
mr/main/publication failures must retain their primary error and all Git guards
when automatic log download fails. Never hit a real provider in these regressions.

## Public Make target index

This index covers every target shown by make help. Native/internal prerequisites
are implementation details; use the public targets from the source checkout.
Source release targets operate on this launcher, not a Puppet module.

- Workspace: `help` lists targets; `clean` removes local generated artifacts and
  coverage reports, including downloaded CI artifacts under DIST.
- Dependencies: `tidy` changes module metadata; `deps-update` updates dependencies
  then tidies. `checks-image` builds the developer Go/lint/security image, not a
  module runtime image.
- Formatting/static checks: `format` writes Go formatting throughout cmd/internal/docs;
  `fmt-check` checks it, `vet` runs go vet, `lint` runs golangci-lint, and
  `workflow-lint` checks source GitHub Actions YAML with actionlint.
- Tests: `test` runs Go tests and shell-completion tests; `coverage-check` enforces
  COVERAGE_THRESHOLD (default 65.0% Go statement coverage); `test-race` enables the
  race detector. This Go threshold is unrelated to a team's 100% Puppet resource
  coverage requirement. `release-targets-check` tests source Make lifecycle/recovery
  against temporary Git remotes and fake gh; `image-scan-check` tests advisory
  vulnerability handling and fatal scanner errors without real publication.
- Security: `govulncheck` checks reachable Go vulnerabilities, `gosec` performs Go
  security analysis, `security-go` combines them, and `trivy-filesystem` scans
  source vulnerabilities/secrets/licenses/misconfiguration. `voxbox-security`
  explicitly scans the upstream tool image; findings there are advisory, while
  scanner failures remain errors. Source security gates remain strict.
- SDK integration: `workflow-check` runs real Ruby/ERB regressions and Ruby-asset
  lint. `voxbox-check` combines those checks with the explicit upstream image scan.
  A skipped optional external template test is not a passed team integration.
- Aggregate/build: `check` runs formatting, vet/lint, coverage, actionlint and source
  regression targets. `full-check` adds race/security/SDK/Trivy checks. `build` runs
  full-check, builds four platform binaries and checksums/LICENSE, and smoke-tests
  the matching Linux-container binary. `validate-version` accepts dev, dev-SHA,
  X.Y.Z or vX.Y.Z; release targets require canonical vX.Y.Z.
- Branch preparation: `main-sync` safely synchronizes main; `release-prepare`
  creates release/X.Y.Z from synchronized clean main. `release-prepare-noop`
  previews from already synchronized clean main without changing refs.
- Implementation/preparation: `release-commit` commits reviewed source;
  `release-finalize` creates the separate changelog-only preparation commit.
  `release-submit` combines commit/finalize/push. `release-commit-noop` and
  `release-submit-noop` preview without staging/committing. `release-push` retries
  or performs branch push without tags; `release-push-noop` previews that push.
- PR/inspection: `mr` creates/reuses the source PR, waits, merges the pinned head,
  synchronizes main and verifies its artifacts. `pipeline-jobs` and `pipeline-logs`
  inspect source Actions with REF/RUN_ID/WATCH or JOB/FAILED_ONLY/TAIL as documented
  above. Module log flags are a separate interface.
- Release guard helpers: `release-version-check` validates VERSION against the
  latest changelog release; `release-worktree-check` checks committed clean main;
  `release-tag-check` refuses consumed tag names; `release-main-check` verifies
  remote main; `release-ci-wait` waits for the exact ref/SHA workflow.
  `release-artifacts-check` verifies the stored current-main CI artifact evidence.
  These are normally invoked by combined release targets, not manual extra steps.
- Artifact/release operations: `ci-artifacts` downloads and verifies main artifacts.
  `push-main` supports an already prepared main and awaits its artifacts.
  `release-preflight` runs a full local release build. `release-tag-noop` previews
  guards; `release-tag` creates only the local annotated tag. `push-release-tag`
  pushes/resumes that exact tag and waits for publication. `push-release` combines
  reviewed artifact checks, preflight, local tagging and tag push. It never pushes
  main. No build/check target publishes anything.

## Deferred review-approval waiting

Required human MR/PR approvals are planned for a later launcher release, not v2.1.0.
Current mr waits for exact-head CI and then asks the provider to merge; an unmet
approval rule can therefore refuse the merge. Do not claim that approval waiting
is implemented or bypass provider review rules. The follow-up must cover GitLab
and GitHub, distinguish human reviews from CI approvals/manual jobs, report the
request URL while waiting, and recheck source SHA and CI before merging.

## Single module toolchain

Module check has four stages: audit, VoxBox validation, VoxBox specs and release
checks. The retired PDK runtime/image/commands/aliases are not supported.
Native module-build runs jig directly; Docker jig plans override the VoxBox
entrypoint without forwarding provider/template/Forge secrets or a Docker socket.
module-convert refuses an existing .sync.lock.json; managed baseline updates use sync.
Go forge-publish reads a regular metadata-matching local archive, uses Forge V3
JSON/base64 over HTTPS with token headers, refuses redirects and retries, and
requires HTTP 201. It bypasses image/config initialization; publication remains
an explicit command behind the team's protected-tag/exact-main CI gates.
Regressions cover removed commands/selectors, four-step fail-fast order, native
argument parity, jig no-overwrite/preview plans and V3 bytes/auth/status handling.
Team templates own fact-matrix completeness and spec/coverage gates; the neutral
starter does not supply those checks.

The launcher module, development/check image and source builds require Go 1.27.3.
The go.mod directive matches GO_IMAGE; this is independent of the Ruby/OpenVox SDK.

## Version 2 source and artifacts

Prepare the 2.1.0 release on release/2.1.0. Keep Unreleased until the separate
release-finalize preparation commit. The Go module and all internal imports and
-X linker symbols use github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2.
The GitHub repository/release download URLs do not gain /v2. make build VERSION=v2.1.0
produces all four platforms, LICENSE and SHA256SUMS under dist/v2.1.0 and verifies
the real linked version/default-image behavior through native smoke.
Build regressions retain 2.0.0/v2.0.0 coverage alongside stable/dev version syntax.
Publish pmtool v2.1.0 before updating the team template CI pin for host-prepared
fixtures; publish that template version before releasing consuming modules.
Keep all published tags immutable.


## Token requirements and preflight

See [developer and CI token requirements](TOKENS.md) (`pmtool docs tokens`) for
scopes, project roles, job-token allowlists, Forge read/publish separation,
optional template overrides and fail-fast input validation. `pmtool help` is
short usage; `pmtool docs` opens the full manual without Docker or credentials.


Completion tests exercise Bash/Zsh callbacks with a quoted executable path and
multiple callback arguments. Native macOS smoke checks must also run the built
binary with the system Bash 3.2; Linux Bash-only success does not cover its array
slicing behavior. The callback constructs an explicit argument array.

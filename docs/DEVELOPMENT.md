# pmtool development

## Source ownership

cmd/pmtool is the executable entry point. internal/pmtool contains CLI/configuration,
Go validation plans, process execution, output, AI export and completion.
internal/hostflow owns Git, REST providers, release operations and their regressions.
internal/templatekit owns template checkout/authentication, schema and policy loading,
CI selection, baseline rendering requests, sync, ownership and general audit.
internal/workflow contains only the embedded command catalog. docs contains the
embedded manuals and assistant guidance; test contains Make target regressions.

Pinned upstream VoxBox supplies the runtime tools and dependencies. The separate
Dockerfile.checks builds development-only Go/lint/security tools; it is not a
custom module runtime and is not used by module audit or template jobs.
Team templates supply contract files, audit policy, Puppet/RSpec checks and CI.
The launcher contains no second workflow implementation.

## Runtime boundary

Settings and validation sequences are constructed in Go. Git/API commands bypass
Docker and image configuration. Template checkout runs host Git with immutable
refs and explicit host/repository-scoped credentials; temporary files are removed
on success and failure. Sync and general audit execute in Go on the selected clone.

The small ERB worker receives JSON documents through stdin and returns JSON strings.
It only requires Ruby's JSON and ERB libraries. The Docker executor runs it in the
pinned tools image with a read-only root, no network and no host source mount.
Template-owned audit scripts receive a read-only module mount and the required
Ruby or VoxBox bundle, without provider/template credentials or a Docker socket.
Native CI uses installed job tools directly and is not a filesystem sandbox.

The remaining Ruby assets are the ERB worker and the standalone contract-profile
helper emitted into modules for their RSpec/AST checks. The latter preserves the
existing generated interface and bytes. A real-Ruby regression compares its
selection results with Go; it is not an alternate launcher backend. Puppet/RSpec
parsing and coverage remain in template-owned checks using the actual runtime APIs.

PDK/VoxBox are started directly through the host Docker CLI. Go preserves child
exit codes, runs checks in order and stops at the first failure. There is no Ruby
planner, settings RPC, archived helper bundle or old engine retained for tests.

## Checks

```sh
make format
make check
make full-check
make build VERSION=v1.0.0
```

Go checks include format, vet, golangci-lint, coverage threshold, race tests,
govulncheck, gosec and actionlint. The workflow-check target compiles the same Go template test suite and runs it
inside the selected SDK with real ERB, then applies strict RuboCop to the remaining
Ruby assets. Unit tests cover parser-independent rules without requiring Ruby. No scanner rules are weakened.
Security scans include Trivy filesystem/license/secret/misconfiguration checks.
WORKFLOW_IMAGE defaults to pinned upstream VoxBox for OpenVox 8. Builds do not need
a sibling environment checkout or a custom runtime image.

Cross-builds copy the preloaded Go modules into a private writable temporary
cache. Builds stay offline under the invoking UID/GID and preserve the image-owned
cache. Use `make build VERSION=v1.0.0` to verify a release; plain `make build`
intentionally produces `dev` artifacts.

Release regression tests use temporary local bare Git remotes and a fake gh.
Workflow tests must never perform real GitLab/GitHub publication. Verify origin
parsing, private template host boundaries, exact refs/SHA, key/cache ordering,
literal argv, no socket, per-step token/author scope and partial-failure recovery.
Keep raw Markdown tables aligned and regenerate/review assistant exports after changes.

## Versions and source publication

Launcher, environment, template and module versions are independent. There is no
VERSION file; source releases use Git tags and the latest Release X.Y.Z changelog.
CI uses pinned upstream VoxBox by default, with TOOLING_IMAGE as an optional
explicit override. Launcher publication does not depend on publishing an image.

Commit reviewed implementation changes first, keeping their changelog entries
under `## Unreleased`. Make the release preparation a separate commit: finalize
the heading as `## Release X.Y.Z` and commit only that preparation diff with
`chore(release): prepare vX.Y.Z`. For the first release use `1.0.0`; do not add a
VERSION file or combine unrelated implementation changes into this commit.

After both local commits, publish the prepared clean main commit and test its
CI artifacts before creating a stable tag. A preparation commit changes the SHA,
so artifacts verified before it cannot satisfy the final commit's artifact gate:

```sh
make push-main
```

The target pushes only main, waits for the exact-SHA push workflow, downloads its
`pmtool-dev-<full SHA>` artifact and verifies its checksums, full MIT license,
source/run identity and native binary version/help. Files remain in `dist/ci/<SHA>/`.
CI_ARTIFACT_ROOT defaults to dist/ci. Use that downloaded binary with your team
template/module. Branch pushes never
create GitHub Releases or prereleases. The standalone `make ci-artifacts` retries
download/verification without another push; existing different bytes are preserved
and rejected. Failed download, invalid manifests, symlinks, wrong version/run/SHA,
changed main and tampered files stop verification.

After that review, publish the stable release:

```sh
make push-release VERSION=v1.0.0
```

This requires verified artifacts of the current committed clean main and exact
origin/main, then runs local preflight and creates/pushes an annotated stable tag.
It does not push main or silently fetch a fresh candidate. Direct release-tag and
push-release-tag targets require the same artifact checks. Tag CI verifies current
main and its successful commit workflow before building. Publication first creates
a GitHub draft and makes it public only after asset uploads succeed; no rc.N release
lane is involved. After an interrupted tag push/publication, preserve the tag,
inspect/retry the failed workflow and resume push-release-tag for the identical tag.
Build/check/clean never publish. clean also removes local CI artifacts; download
and verify them again before tagging. The user retains all publication actions.

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

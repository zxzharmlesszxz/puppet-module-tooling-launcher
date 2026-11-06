# Changelog

## Unreleased

**Breaking changes for 2.0.0**

- Remove all PDK commands/aliases and PDK image selectors; use the VoxBox/jig toolchain and update the consuming team's versioned CI template before module rollout.
- Move the Go module and internal import/linker paths to github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2; the repository, binary name, release URLs and user config location stay the same.

**Changes**

- Add optional template-init --origin to initialize local Git/main and its remote without commits, pushes or network access; previews and default generation remain Git-free.

- Preserve syntax, Puppet lint and RuboCop as mandatory tasks in the VoxBox validation step.
- Remove the PDK runtime, commands, image selectors and CI lanes. Run check as audit, VoxBox validation/specs and release checks on pinned OpenVox 8.
- Add jig-backed module-new, module-convert (with optional preview) and module-build; publish reviewed archives to an explicit HTTPS Forge without exposing tokens in arguments.
- Generate GitLab/GitHub starters with one VoxBox toolchain and verified launcher downloads.

**Bugfixes**

- Verify the full gzip stream before Forge uploads, rejecting corrupt/truncated footers and archives exceeding the unpacked size limit.
- Remove GH_TOKEN from native validation workers and redact the configured Forge publishing token from CI logs.
- Assemble module-side profile helpers under one top-level class with clean boundaries and avoid imposing regexp/word-array literal styles on consuming modules.
- Add regression guards for generated helper boundaries and reserved template/CI settings.

**Maintenance**

- Align the module minimum Go version with the pinned Go 1.27.3 build/check toolchain.
- Use typed error matching and string iterators for value-only loops while preserving wrapped-error and exit-status handling.
- Align English/Ukrainian manuals and assistant guidance with native jig builds, explicit Forge uploads and template-owned audit policy.
- Update the development Trivy scanner to 0.76.2, pinned to its multi-platform image digest.

## Release 1.1.0

**Features**

- Generate a neutral schema-1 template with `template-init`, including ERB/defaults, authoring documentation, self-tests and optional GitLab/GitHub validation CI.
- Read GitLab job traces and GitHub Actions job logs with `pipeline-logs`: select ref/pipeline/job, failed jobs, line tails and a new private output file.
- Show failed-job log tails when source, main or publication CI blocks `mr` or `release-tag`, preserving the primary error and existing release recovery.
- Bound log downloads, strip terminal controls, redact known launcher tokens, and isolate GitHub signed-download redirects from provider credentials.

**Bugfixes**

- Declare the engine caller's optional Forge secrets in generated GitHub workflows without passing them to validation jobs.
- Publish a newline-terminated executable directory through GITHUB_PATH in generated Actions CI.
- Include all internal packages in formatting targets and validate command/section documentation against the launcher.

- Read repository identity from the single repository-local origin URL before transport rewrites are applied.
- Preserve provider inference, template audit and release origin guards when GitLab Runner adds job-token URL rewrites.
- Continue rejecting credentials, query strings, fragments and ambiguous multiple URLs in the configured origin.
- Disable interactive template Git prompts without relying on a platform-specific false executable.

**Maintenance**

- Add source Make targets for release branches, separate preparation commits, submission, pinned-head PR merge and main synchronization.
- Add source CI job/log inspection with ref/run/job identity checks and optional read-only release previews.
- Exercise source lifecycle guards and recovery against local Git remotes and a fake GitHub CLI.
- Add real Git regressions for local and included runner configuration, both providers, native/template operations and stored-credential rejection.
- Cover anonymous HTTP authentication refusal without executing caller or repository askpass helpers.
- Document configured-origin identity and CI template authentication boundaries in English, Ukrainian and assistant instructions.

## Release 1.0.0

**Features**

- Launch the tooling container on Linux and macOS, on amd64 and arm64.
- Own workflow commands, permissions and ordered execution in pmtool.
- Run module Git, GitLab/GitHub REST and guarded release operations directly in Go.
- Preserve CI/ref checks and partial-failure recovery with native Go regressions.
- Keep one Go implementation of lifecycle guidance; remove the legacy Ruby executor and reference backend.
- Move template checkout/authentication, schema/policy, CI selection, sync, ownership, general audit and validation planning to Go.
- Replace archived Ruby helpers with a JSON/ERB worker and standalone module-side profile compatibility assets.
- Exercise template regressions against the Go engine and real ERB; pass fixture data to team tests through JSON.
- Keep module/provider/template settings in the module and host image settings in the user config.
- Supply Bash/Zsh completion, embedded documentation, and assistant instruction exports.
- Build versioned binaries and checksums; add guarded Make targets for source releases.
- Publish commit-specific Actions artifacts as dev-<SHA>, verify them locally, then create the stable release tag.
- Guard source tagging with exact-main CI artifact checks and publish GitHub releases through a draft.

**License**

- Release under the MIT License; include the license text in distributed artifacts.

**Architecture**

- Keep module workflow, audit helpers, documentation and assistant exports in pmtool.
- Use pinned upstream VoxBox for the tool environment and execute validation through the same launcher in local Docker and native CI modes.
- Preserve unrelated fixture data during migration and reject malformed fixture mappings in audit.
- Seed a private writable Go module cache for offline cross-builds under the invoking user.

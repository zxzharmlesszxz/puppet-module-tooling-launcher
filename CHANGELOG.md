# Changelog

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

# Forge dependencies and authenticated proxies

## One dependency manifest

Declare Forge fixtures in the standard `.fixtures.yml`. Enable
`fixtures.resolve_dependencies: true` to list only the direct fixtures:

```yaml
fixtures:
  resolve_dependencies: true
  forge_modules:
    common:
      repo: team/common
      ref: '1.1.0'
```

The standard alternate filename `.fixtures.yaml` is also supported.
Versions and module names stay in this file. Template/CI selection stays in
`.sync.yml`. pmtool 3.0.0 removes the separate `test_fixtures` archive format;
it does not maintain a second downloader, archive extractor or dependency list.
OpenVox resolves transitive requirements from the Forge release metadata.
List a transitive module explicitly if it is also a direct fixture or the
test needs a specific version. Roots install serially with a shared modulepath.
pmtool checks explicit version requirements after resolution and fails on
conflicts. Use standard fixture names and targets in this mode. Optional
per-fixture flags are limited to `--debug`, `--trace`, and `--verbose`.

When `resolve_dependencies` is omitted or false, standard `puppet_fixtures`
installs the explicitly listed Forge fixtures. Use that mode when you want an
entirely explicit test matrix.

## Forge URL and read authorization

Set `FORGE_URL` to the HTTPS base URL of the trusted Forge proxy and `FORGE_TOKEN`
to a raw read-only bearer token. The proxy can serve private modules and cache
upstream public modules, so the same configuration applies to every declared
Forge dependency. Do not put tokens in YAML, command arguments or URLs.
`FORGE_PUBLISH_TOKEN` is separate and is never a fallback for read authentication.

```sh
export FORGE_URL='https://forge.example.test'
pmtool check
```

Load `FORGE_TOKEN` through the environment/secret manager before that command;
the example does not create a token. Configure both values in a CI job's secret
store/environment. `FORGE_URL` is not secret; the read token must be masked and
scoped to the needed modules/proxy operations. Protected CI variables are absent
from unprotected refs, so provide branch read access under the team's policy.

With neither variable set, generic pmtool keeps the ordinary unauthenticated
public-Forge behavior. When Forge modules are declared and either variable is set, both are required
before an SDK worker starts. Templates can mandate the pair for their unit jobs, as Neon
does. `FORGE_TOKEN_HOST` is not used by this interface: the explicit HTTPS
`FORGE_URL` selects the Forge. Invalid URLs, token control/config-injection
characters and conflicting per-fixture config/repository/auth flags fail locally.
The default mode preserves ordinary fixture flags.

## Standard installer and credential lifetime

For an authenticated unit phase, pmtool creates a private temporary directory
outside the module checkout. A `TMPDIR` pointing into the checkout (including
through a symlink) is refused before secrets are written. A mode-0600 `puppet.conf` contains the native
OpenVox settings `module_repository` and `forge_authorization = Bearer ...`.
A temporary fixture manifest adds the non-secret `--config` path to each Forge
entry; module names, versions, repositories and unrelated settings are preserved.
The tracked `.fixtures.yml` is never rewritten by a test run.

The phase is ordered as follows:

1. Prepare fixtures with that private config. The default mode runs
   `fixtures:prep`. The opt-in uses the same fixture parser and calls
   `puppet module install` with dependency resolution enabled. The installer
   reads the token from the config file.
2. Remove the config and preparation manifest; a removal failure stops execution.
3. Run `spec:standalone` followed by standard `fixtures:clean` on success.
   Specs do not rerun downloads and do not receive Forge credentials/config.

Docker prep/spec workers use the pinned SDK and share one temporary module
fixture directory. Only prep mounts the config. No Docker socket is passed.
Temporary Docker fixture/config directories are removed on success or failure.
With dependency resolution enabled, native CI exposes the temporary dependency
tree through `spec/fixtures/modules` for the duration of the run. Start with an
absent or empty fixture directory. pmtool removes the temporary tree and link
on success or failure. Default native mode uses the ordinary ignored fixture
directory and may leave dependencies on a failed run. This is process/mount hygiene, not a filesystem sandbox
against trusted module code, runner administrators or the authenticated prep task.

The configured proxy is trusted to supply release metadata/downloads. Download
transport, extraction and their error behavior belong to the standard OpenVox
installer; pmtool does not claim an additional per-archive SHA256 pin.
See the native [Forge authorization](https://docs.openvoxproject.org/openvox/latest/configuration.html#forge_authorization)
and [fixture implementation](https://github.com/voxpupuli/puppet_fixtures).

## Upgrade from pmtool 2.1

Move each old archive dependency to `forge_modules` using `repo: OWNER/MODULE`
and `ref: VERSION`, then remove the `test_fixtures` key from `.sync.yml`.
Set the shared `FORGE_URL`/`FORGE_TOKEN` pair instead of archive-specific source,
auth, digest and host settings. A remaining old declaration fails explicitly so
an upgrade cannot silently skip dependencies.

Use a development pmtool 3 binary until its release is published. Publish the
launcher, then the template CI pin, then update module selectors through normal
sync/check before publishing those modules. Existing published tags stay immutable.

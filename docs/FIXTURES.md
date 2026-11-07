# Test fixture downloads

## Host-prepared test fixtures

A module can declare private archive fixtures in `.sync.yml`:

```yaml
test_fixtures:
  common:
    source: 'https://forge.example.test/api/v1/modules/team/common/versions/1.0.0/download'
    module: team-common
    version: '1.0.0'
    auth: forge
    sha256: 'REPLACE_WITH_THE_ARCHIVE_SHA256'
```

Keys must match the Puppet module name; versions must be stable `X.Y.Z` and
`sha256` must be the archive's lowercase 64-digit SHA256. This is module-side
configuration, not a template contract group or user-global inventory. Keep
ordinary public dependencies in `.fixtures.yml`; do not declare the same fixture
in both places. The source must be a direct, credential-free HTTPS archive URL
without a query/fragment. Redirects are refused.

Before the unit step in `check` or `voxbox --stage unit`, pmtool downloads each
archive, verifies its checksum, gzip integrity, safe regular-file/directory paths
and metadata identity/version. It stages files in a unique temporary directory
under the module's ignored `vendor/`, generates a temporary `FIXTURES_YML` that
preserves normal fixtures plus the module self-link, and removes staging after
the worker exits, including failure. No tracked fixture file or module runtime
source is rewritten. Each invocation fetches/verifies dependencies; this is not
a persistent shared cache. Docker workers see `/repo` paths; native CI uses its
actual checkout path. Preview, audit, validation-only and build do not fetch fixtures.

Declare private reads with `auth: forge`. They require `FORGE_TOKEN` and the exact `FORGE_TOKEN_HOST` (including an
explicit non-default port). For `auth: forge`, a missing token/host or mismatching host fails in preflight
before SDK startup. Public downloads omit `auth`; they require neither variable and do not receive
the configured read token. See [token requirements](TOKENS.md).
Only the host-side GET receives the bearer token. The SDK/native worker receives
the temporary fixture manifest and extracted files, never the Forge token.
Configure the read-only token as a masked CI secret; protect it according to the
team's branch policy. Unprotected development pipelines need a separately scoped
read-only credential if they must fetch private fixtures; do not expose publish tokens.
Host-prepared fixtures require pmtool v2.1.0 or later, locally and in CI.
pmtool v2.0.0 does not implement this feature. Publish the new launcher first,
then update and release the team template CI pin before publishing consumers.

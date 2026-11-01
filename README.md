# pmtool launcher

Go host CLI for the [tooling container](https://github.com/zxzharmlesszxz/puppet-module-tooling).
It reads the selected image's manifest and launches it with the required mounts,
provider token and Git author. Linux/macOS amd64/arm64, standard library only.

```sh
pmtool config init --image YOUR_PUBLISHED_IMAGE
pmtool help
pmtool docs onboarding
```

Host config.json stores image/docker/socket only. Module .sync.yml chooses the
template version and optional CI/provider overrides. CI otherwise comes from the
same resolved template SHA; the launcher does not fetch or render templates itself.
See [embedded manual source](manual.md). No team template or reusable CI is bundled.

Developer targets: make format, make test, make build. Build runs go vet/tests and
produces four binaries plus dist/SHA256SUMS. VERSION sets the binary version;
IMAGE optionally sets its default runtime image, otherwise configure the image at
installation. The default build does not depend on a sibling container checkout.
The repository GitHub workflow builds/tests branches and publishes versioned GitHub Releases on release tags.

Executable: pmtool. Repository/module name: puppet-module-tooling-launcher.
Build/release files carry the selected version and platform, plus SHA256SUMS; see Versioned builds and releases below. Install the chosen
platform binary as pmtool. The existing host configuration remains at
~/.config/puppet-module-tooling/config.json; renaming the executable does not
relocate settings. Container image names and module .sync.yml are unchanged.

## Versioned builds and releases

```sh
make build VERSION=1.2.3
```

X.Y.Z and vX.Y.Z normalize to vX.Y.Z. Omission builds dev. Release versions use
three numeric components without leading zeros; branches/path-like values are
rejected before compilation. This version is embedded in pmtool version and AI
exports. It is independent of the selected container image or module version.

Outputs for v1.2.3 are under dist/v1.2.3/: pmtool-v1.2.3-linux-amd64,
pmtool-v1.2.3-linux-arm64, pmtool-v1.2.3-darwin-amd64,
pmtool-v1.2.3-darwin-arm64 and SHA256SUMS. Dev builds use dist/dev/pmtool-dev-PLATFORM.
Install the chosen platform file as pmtool.

On branch pushes, GitHub Actions builds/tests dev artifacts. A vX.Y.Z tag builds
that exact version, then a separate job creates a GitHub Release with all four
binaries and SHA256SUMS. Only the release job has contents: write. Invalid tag
versions fail the build. Publication requires successful tests/build and an existing
tag; an existing Release is not overwritten automatically. Local make build never
creates tags, commits, pushes or releases.

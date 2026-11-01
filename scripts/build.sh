set -eu

invalid_version() {
  printf '%s\n' 'VERSION must be dev, X.Y.Z or vX.Y.Z' >&2
  exit 1
}

version=${LAUNCHER_VERSION-dev}
if [ "$version" != dev ]; then
  case "$version" in
    *[!0-9v.]*|'') invalid_version ;;
  esac
  if ! printf '%s\n' "$version" | grep -Eq '^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
    invalid_version
  fi
  version="v${version#v}"
fi

output="${PMTOOL_DIST:-dist}/$version"
mkdir -p "$output"
set --
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  os=${target%/*}
  arch=${target#*/}
  binary="pmtool-$version-$os-$arch"
  GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.version=$version -X main.defaultImage=${LAUNCHER_IMAGE-}" \
    -o "$output/$binary" .
  set -- "$@" "$binary"
done
(cd "$output" && sha256sum "$@" > SHA256SUMS)
printf 'Built pmtool %s: %s\n' "$version" "$output"

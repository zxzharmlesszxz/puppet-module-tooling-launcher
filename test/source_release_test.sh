#!/bin/sh
set -eu

source_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/pmtool-release-test.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT HUP INT TERM
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE MAKEFLAGS MFLAGS MAKELEVEL CI_ARTIFACT_ROOT CI_ARTIFACT_DIR CI_RUN_FILE

fail() {
  printf '[FAIL] %s\n' "$*" >&2
  exit 1
}

setup() {
  case_root="$scratch/$1"
  mkdir -p "$case_root/module"
  git init --quiet --bare "$case_root/remote.git"
  cd "$case_root/module"
  git init --quiet --initial-branch=main
  git config user.name 'Release test'
  git config user.email 'release@example.invalid'
  git config commit.gpgsign false
  git remote add origin "$case_root/remote.git"
  printf 'VERSION := v1.0.0\nGH := %s/gh\nRELEASE_REF := main\nRELEASE_WORKFLOW := fixture.yml\nCI_ARTIFACT_ROOT := dist/ci\n' "$scratch" > Makefile
  awk '
    /^export VERSION GH RELEASE_REF RELEASE_WORKFLOW/ {copy=1}
    !copy {next}
    /^release-preflight:/ {
      print "release-preflight: release-version-check"
      print "\t@test \"$${PMTOOL_TEST_PREFLIGHT_FAIL:-0}\" = 0"
      skip=1; next
    }
    skip && /^[^[:space:]]/ {skip=0}
    !skip {print}
  ' "$source_root/Makefile" >> Makefile
  printf '# Changelog\n\n## Release 1.0.0\n' > CHANGELOG.md
  printf '/dist/\n' > .gitignore
  cp "$source_root/LICENSE" LICENSE
  git add Makefile CHANGELOG.md .gitignore LICENSE
  git commit --quiet -m fixture
}

run_make() {
  make --no-print-directory "$@" > "$case_root/output" 2>&1
}

refused() {
  message=$1
  shift
  if run_make "$@"; then fail "unexpected success: $*"; fi
  if ! grep -Fq "$message" "$case_root/output"; then
    cat "$case_root/output" >&2
    fail "missing diagnostic: $message"
  fi
}

no_tag() {
  if git show-ref --verify --quiet refs/tags/v1.0.0; then fail 'unexpected local tag'; fi
  test -z "$(git ls-remote --tags origin)" || fail 'unexpected remote tag'
}

cat > "$scratch/gh" <<'GH'
#!/bin/sh
set -eu
case "$1 $2" in
  'repo view') printf 'https://github.com/example/release-fixture\n' ;;
  'run list')
    test "${PMTOOL_TEST_QUERY_FAIL:-0}" = 0 || exit 1
    printf '123\n'
    ;;
  'run watch')
    if [ "${PMTOOL_TEST_TAG_DRIFT:-0}" = 1 ]; then
      git tag -f -a v1.0.0 -m replaced >/dev/null
    fi
    test "${PMTOOL_TEST_CI_FAIL:-0}" = 0 || exit 1
    if [ "${PMTOOL_TEST_HEAD_DRIFT:-0}" = 1 ]; then
      git commit --quiet --allow-empty -m drift
    fi
    ;;
  'run view')
    test "$3" = 123 || exit 1
    sha=$(git rev-parse HEAD)
    if [ "${PMTOOL_TEST_WRONG_SHA:-0}" = 1 ]; then sha=wrong; fi
    conclusion=success
    if [ "${PMTOOL_TEST_SKIPPED:-0}" = 1 ]; then conclusion=skipped; fi
    printf '%s\t%s\tpush\t%s\n' "$sha" "$RELEASE_REF" "$conclusion"
    ;;
  'run download')
    test "$3" = 123 || exit 1
    test "${PMTOOL_TEST_DOWNLOAD_FAIL:-0}" = 0 || exit 1
    shift 3
    destination=; artifact=
    while [ "$#" -gt 0 ]; do
      case "$1" in
        --dir) shift; destination=$1 ;;
        --name) shift; artifact=$1 ;;
      esac
      shift
    done
    head=$(git rev-parse HEAD)
    test "$artifact" = "pmtool-dev-$head" || exit 1
    mkdir -p "$destination"
    for platform in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
      version="dev-$head"
      if [ "${PMTOOL_TEST_BINARY_VERSION:-0}" = 1 ]; then version=dev; fi
      cat > "$destination/pmtool-dev-$head-$platform" <<BINARY
#!/bin/sh
case "\$1" in
  version) printf '%s\\n' 'pmtool $version (${platform%-*}/${platform#*-})' ;;
  help) printf '%s\\n' pmtool ;;
  *) exit 2 ;;
esac
BINARY
    done
    cp LICENSE "$destination/LICENSE"
    (cd "$destination"; if command -v sha256sum >/dev/null 2>&1; then sha256sum pmtool-* LICENSE; else shasum -a 256 pmtool-* LICENSE; fi) > "$destination/SHA256SUMS"
    if [ "${PMTOOL_TEST_CORRUPT_ARTIFACT:-0}" = 1 ]; then printf corrupt >> "$destination/LICENSE"; fi
    if [ "${PMTOOL_TEST_UNSAFE_MANIFEST:-0}" = 1 ]; then printf '%064d  ../../LICENSE\n' 0 >> "$destination/SHA256SUMS"; fi
    if [ "${PMTOOL_TEST_ARTIFACT_SYMLINK:-0}" = 1 ]; then
      rm "$destination/pmtool-dev-$head-linux-amd64"
      ln -s /bin/false "$destination/pmtool-dev-$head-linux-amd64"
    fi
    ;;
  *) exit 2 ;;
esac
GH
chmod +x "$scratch/gh"

setup invalid-version
for version in dev 1.0.0 v01.0.0 v1.0.0-rc.1 v1.0; do
  refused 'canonical vMAJOR.MINOR.PATCH' release-version-check "VERSION=$version"
done
refused 'latest changelog release' release-version-check VERSION=v1.0.1

setup dirty
printf 'unreviewed\n' > extra
refused 'Working tree is dirty' push-release
no_tag
test -z "$(git ls-remote --heads origin)" || fail 'dirty tree was pushed'

setup wrong-branch
git switch --quiet -c feature
refused 'Release must run from main' push-release
no_tag

setup preflight-failure
run_make push-main || { cat "$case_root/output" >&2; fail 'candidate failed'; }
main_before=$(git ls-remote --heads origin)
export PMTOOL_TEST_PREFLIGHT_FAIL=1
refused 'Error' push-release
unset PMTOOL_TEST_PREFLIGHT_FAIL
test "$(git ls-remote --heads origin)" = "$main_before" || fail 'failed preflight changed main'
no_tag

setup failed-ci
export PMTOOL_TEST_CI_FAIL=1
refused 'Error' push-main
unset PMTOOL_TEST_CI_FAIL
no_tag

setup query-failure
export PMTOOL_TEST_QUERY_FAIL=1
refused 'Error' push-main
unset PMTOOL_TEST_QUERY_FAIL
no_tag

setup wrong-sha
export PMTOOL_TEST_WRONG_SHA=1
refused 'Workflow ref, SHA, event or conclusion changed' push-main
unset PMTOOL_TEST_WRONG_SHA
no_tag

setup skipped-ci
export PMTOOL_TEST_SKIPPED=1
refused 'Workflow ref, SHA, event or conclusion changed' push-main
unset PMTOOL_TEST_SKIPPED
no_tag

setup head-drift
export PMTOOL_TEST_HEAD_DRIFT=1
refused 'Workflow ref, SHA, event or conclusion changed' push-main
unset PMTOOL_TEST_HEAD_DRIFT
no_tag

setup success
git config push.followTags true
git tag -a unrelated -m unrelated
run_make push-main || { cat "$case_root/output" >&2; fail 'candidate failed'; }
no_tag
run_make release-artifacts-check || { cat "$case_root/output" >&2; fail 'candidate verification failed'; }
run_make push-release || { cat "$case_root/output" >&2; fail 'release failed'; }
test "$(git cat-file -t refs/tags/v1.0.0)" = tag || fail 'tag is not annotated'
test "$(git rev-parse "v1.0.0^{commit}")" = "$(git rev-parse HEAD)" || fail 'wrong tag SHA'
test "$(git ls-remote --heads --tags origin | wc -l | tr -d ' ')" = 3 || fail 'unexpected remote refs'
refused 'Local tag exists' push-release
run_make push-release-tag || fail 'identical published tag could not resume'

setup resume
run_make push-main || { cat "$case_root/output" >&2; fail 'candidate failed'; }
cat > "$case_root/remote.git/hooks/pre-receive" <<'HOOK'
#!/bin/sh
set -eu
while read -r old new ref; do
  case "$ref" in refs/tags/*) exit 1 ;; esac
done
HOOK
chmod +x "$case_root/remote.git/hooks/pre-receive"
refused 'Local tag is preserved' push-release
saved_tag=$(git rev-parse refs/tags/v1.0.0)
test -z "$(git ls-remote --tags origin)" || fail 'rejected tag appeared remotely'
rm "$case_root/remote.git/hooks/pre-receive"
run_make push-release-tag || { cat "$case_root/output" >&2; fail 'resume failed'; }
test "$(git rev-parse refs/tags/v1.0.0)" = "$saved_tag" || fail 'resume changed local tag'

setup remote-tag-conflict
run_make push-main || fail 'candidate failed'
git tag -a v1.0.0 -m original
git push --quiet origin refs/tags/v1.0.0
git tag --delete v1.0.0 > /dev/null
refused 'Remote tag already exists' push-release
git tag -a v1.0.0 -m different-object
git push --quiet origin HEAD:refs/heads/main
refused 'Remote tag differs' push-release-tag

setup local-tag-drift
run_make push-main || fail 'candidate failed'
git tag -a v1.0.0 -m original
git push --quiet origin HEAD:refs/heads/main
export PMTOOL_TEST_TAG_DRIFT=1
refused 'Local tag changed before tag push' push-release-tag
unset PMTOOL_TEST_TAG_DRIFT
test -z "$(git ls-remote --tags origin)" || fail 'changed local tag was pushed'

setup lightweight-tag
run_make push-main || fail 'candidate failed'
git tag v1.0.0
refused 'annotated local release tag' push-release-tag

setup remote-query-failure
git remote set-url origin "$scratch/missing.git"
refused 'does not appear to be a git repository' release-tag-check

setup no-candidate
git push --quiet origin HEAD:refs/heads/main
refused 'Verified CI artifacts are missing' push-release
no_tag

setup artifacts-without-push
git push --quiet origin HEAD:refs/heads/main
refs_before=$(git ls-remote --heads --tags origin)
run_make ci-artifacts || { cat "$case_root/output" >&2; fail 'artifact verification failed'; }
test "$(git ls-remote --heads --tags origin)" = "$refs_before" || fail 'artifact download mutated remote refs'
run_make ci-artifacts || { cat "$case_root/output" >&2; fail 'identical artifact download was not reusable'; }
no_tag

setup bad-artifact-checksum
export PMTOOL_TEST_CORRUPT_ARTIFACT=1
refused 'FAILED' push-main
unset PMTOOL_TEST_CORRUPT_ARTIFACT
no_tag
refused 'Verified CI artifacts are missing' push-release

setup unsafe-artifact-manifest
export PMTOOL_TEST_UNSAFE_MANIFEST=1
refused 'Invalid CI artifact checksum manifest' push-main
unset PMTOOL_TEST_UNSAFE_MANIFEST
no_tag

setup bad-artifact-version
export PMTOOL_TEST_BINARY_VERSION=1
refused 'CI binary version differs' push-main
unset PMTOOL_TEST_BINARY_VERSION
no_tag

setup artifact-symlink
export PMTOOL_TEST_ARTIFACT_SYMLINK=1
refused 'Missing or nonregular CI artifact' push-main
unset PMTOOL_TEST_ARTIFACT_SYMLINK
no_tag

setup artifact-download-failed
export PMTOOL_TEST_DOWNLOAD_FAIL=1
refused 'Error' push-main
unset PMTOOL_TEST_DOWNLOAD_FAIL
no_tag
refused 'Verified CI artifacts are missing' release-tag

setup artifact-tampered-after-download
run_make push-main || fail 'candidate failed'
head=$(git rev-parse HEAD)
printf corrupted >> "dist/ci/$head/LICENSE"
refused 'FAILED' push-release
no_tag

setup artifact-run-id-tampered
run_make push-main || fail 'candidate failed'
head=$(git rev-parse HEAD)
printf '999\n' > "dist/ci/$head/.workflow-run-id"
refused 'Error' push-release
no_tag

setup stale-candidate
run_make push-main || fail 'candidate failed'
git commit --quiet --allow-empty -m 'new source commit'
git push --quiet origin HEAD:refs/heads/main
refused 'Verified CI artifacts are missing' push-release
no_tag

printf '[OK] Source release target and commit-artifact regressions passed\n'

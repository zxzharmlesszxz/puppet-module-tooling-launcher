#!/bin/sh
set -eu

source_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/pmtool-release-test.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT HUP INT TERM
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE MAKEFLAGS MFLAGS MAKELEVEL CI_ARTIFACT_ROOT CI_ARTIFACT_DIR CI_RUN_FILE MESSAGE REF RUN_ID WATCH FAILED_ONLY TAIL JOB

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
  'release view')
    test "$3" = "$VERSION" || exit 2
    test "${PMTOOL_TEST_RELEASE_MISSING:-0}" = 0 || { printf '%s\n' 'release not found' >&2; exit 1; }
    printf '%s\t%s\t%s\n' "$3" "${PMTOOL_TEST_RELEASE_DRAFT:-false}" "${PMTOOL_TEST_RELEASE_PRERELEASE:-false}"
    ;;
  'run list')
    test "${PMTOOL_TEST_QUERY_FAIL:-0}" = 0 || exit 1
    test "${PMTOOL_TEST_NO_RUN:-0}" = 0 || exit 0
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
    case " $* " in
      *' --json jobs '*)
        if [ "${PMTOOL_TEST_WRONG_JOB:-0}" = 1 ]; then printf 'false\n'; else printf 'true\n'; fi
        exit 0
        ;;
      *' --log-failed '*|*' --log '*)
        test "${PMTOOL_TEST_LOG_FAIL:-0}" = 0 || exit 1
        printf 'fixture log line 1\nfixture log line 2\nfixture log line 3\n'
        exit 0
        ;;
      *' --json headSha,headBranch,event '*)
        ref=${REF:-$(git branch --show-current)}
        sha=$(git rev-parse "$ref^{commit}")
        if [ "${PMTOOL_TEST_WRONG_SHA:-0}" = 1 ]; then sha=wrong; fi
        printf '%s\t%s\tpush\n' "$sha" "$ref"
        exit 0
        ;;
      *' --exit-status '*)
        printf 'build job: fixture\n'
        test "${PMTOOL_TEST_CI_FAIL:-0}" = 0
        exit
        ;;
    esac
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
  'pr list')
    if [ "${PMTOOL_TEST_MULTIPLE_PRS:-0}" = 1 ]; then printf '77\n78\n'; exit 0; fi
    if [ -f .git/source-pr-state ] && [ "$(cat .git/source-pr-state)" = OPEN ]; then printf '77\n'; fi
    ;;
  'pr create')
    test "${PMTOOL_TEST_PR_CREATE_FAIL:-0}" = 0 || exit 1
    shift 2
    branch=; body=
    while [ "$#" -gt 0 ]; do
      case "$1" in --head) shift; branch=$1 ;; --body-file) shift; body=$1 ;; esac
      shift
    done
    test -n "$branch" && test -s "$body" || exit 2
    cp "$body" .git/source-pr-body
    printf '%s\n' "$branch" > .git/source-pr-branch
    git rev-parse HEAD > .git/source-pr-sha
    printf 'OPEN\n' > .git/source-pr-state
    printf 'created\n' >> .git/source-pr-created
    printf 'https://github.com/example/release-fixture/pull/77\n'
    ;;
  'pr view')
    test "$3" = 77 || exit 2
    branch=$(cat .git/source-pr-branch)
    sha=$(cat .git/source-pr-sha)
    if [ "${PMTOOL_TEST_PR_WRONG_SHA:-0}" = 1 ]; then sha=changed; fi
    printf '%s\t%s\tmain\t%s\n' "$(cat .git/source-pr-state)" "$branch" "$sha"
    ;;
  'pr merge')
    test "$3" = 77 || exit 2
    shift 3
    pinned=
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --match-head-commit ]; then shift; pinned=$1; fi
      shift
    done
    head=$(cat .git/source-pr-sha)
    test "$pinned" = "$head" || exit 2
    remote=$(git config --get remote.origin.url)
    main=$(git --git-dir="$remote" rev-parse refs/heads/main)
    tree=$(git rev-parse "$head^{tree}")
    merged=$(printf 'Merge source PR\n' | git --git-dir="$remote" -c user.name=Fixture -c user.email=fixture@example.invalid commit-tree "$tree" -p "$main" -p "$head")
    git --git-dir="$remote" update-ref refs/heads/main "$merged" "$main"
    printf 'MERGED\n' > .git/source-pr-state
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
before=$(git show-ref)
run_make release-tag-noop || { cat "$case_root/output" >&2; fail 'tag preview failed'; }
test "$(git show-ref)" = "$before" || fail 'tag preview changed refs'
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

source_setup() {
  setup "$1"
  git push --quiet origin main
}

source_edit() {
  printf '# Changelog\n\n## Unreleased\n\n- Fixture release notes.\n\n## Release 1.0.0\n' > CHANGELOG.md
  printf 'fixture source change\n' > feature.txt
}

source_submit() {
  run_make release-prepare VERSION=v1.0.1 || { cat "$case_root/output" >&2; fail 'source prepare failed'; }
  source_edit
  run_make release-submit VERSION=v1.0.1 'MESSAGE=Implement source fixture' || { cat "$case_root/output" >&2; fail 'source submit failed'; }
}

source_setup source-prepare-preview
before=$(git show-ref)
run_make release-prepare-noop VERSION=v1.0.1 || fail 'source preview failed'
test "$(git show-ref)" = "$before" && test "$(git branch --show-current)" = main || fail 'prepare preview mutated refs'
test -z "$(git status --porcelain)" || fail 'prepare preview changed worktree'
refused 'exceed the main changelog' release-prepare-noop VERSION=v1.0.0
run_make release-prepare VERSION=v1.0.1 || fail 'source prepare failed'
test "$(git branch --show-current)" = release/1.0.1 || fail 'wrong source release branch'
refused 'already exists locally' release-prepare VERSION=v1.0.1
test "$(git branch --show-current)" = release/1.0.1 || fail 'duplicate prepare switched branches'
no_tag

source_setup source-dirty-prepare
printf preserved > unreviewed.txt
refused 'Working tree is dirty' release-prepare VERSION=v1.0.1
test "$(git branch --show-current)" = main && test -f unreviewed.txt || fail 'dirty prepare discarded work'

source_setup source-credential-origin
git remote set-url origin https://user:fixture-secret@github.com/example/release-fixture.git
refused 'credential-free configured origin' source-origin
if grep -Fq fixture-secret "$case_root/output"; then fail 'source origin rejection exposed a password'; fi

source_setup source-empty-second-origin
git config --local --add remote.origin.url ''
refused 'Exactly one configured origin URL' source-origin

source_setup source-unmerged-main-sync
git switch --quiet -c feature
git commit --quiet --allow-empty -m unmerged
head=$(git rev-parse HEAD)
refused 'HEAD has commits outside origin/main' main-sync
test "$(git rev-parse HEAD)" = "$head" && test "$(git branch --show-current)" = feature || fail 'main sync lost unmerged commits'

source_setup source-combined-submit
run_make release-prepare VERSION=v1.0.1 || fail 'source prepare failed'
source_edit
before=$(git show-ref)
index_before=$(git write-tree)
run_make release-submit-noop VERSION=v1.0.1 || fail 'source submit preview failed'
test "$(git show-ref)" = "$before" && test "$(git write-tree)" = "$index_before" || fail 'submit preview staged or committed'
git tag -a unrelated -m unrelated
git config push.followTags true
run_make release-submit VERSION=v1.0.1 'MESSAGE=Implement source fixture' || { cat "$case_root/output" >&2; fail 'combined submit failed'; }
test "$(git log -1 --format=%s)" = 'chore(release): prepare v1.0.1' || fail 'missing separate preparation commit'
test "$(git log -1 --format=%s HEAD^)" = 'Implement source fixture' || fail 'implementation and preparation were combined'
test "$(git diff-tree --no-commit-id --name-only -r HEAD)" = CHANGELOG.md || fail 'preparation committed more than changelog'
test "$(git diff HEAD^ HEAD -- CHANGELOG.md | grep -c '^+## Release 1.0.1$')" = 1 || fail 'preparation did not finalize release heading'
test ! -e VERSION || fail 'source submission added a VERSION file'
test -z "$(git ls-remote --tags origin)" || fail 'source submission pushed unrelated tags'
submitted=$(git rev-parse HEAD)
run_make release-submit VERSION=v1.0.1 || fail 'clean prepared submit could not resume'
test "$(git rev-parse HEAD)" = "$submitted" || fail 'resuming submit created duplicate commits'
printf correction >> feature.txt
run_make release-submit VERSION=v1.0.1 'MESSAGE=Correct source fixture' || fail 'same-version corrective commit was refused'
test "$(git log -1 --format=%s)" = 'Correct source fixture' || fail 'corrective commit was replaced by an empty preparation'

refs_before=$(git show-ref)
remote_before=$(git ls-remote --heads --tags origin)
run_make release-push-noop VERSION=v1.0.1 || fail 'source push preview failed'
run_make pipeline-jobs WATCH=5 || { cat "$case_root/output" >&2; fail 'pipeline jobs/watch failed'; }
run_make pipeline-logs TAIL=1 || fail 'pipeline logs failed'
grep -Fq 'fixture log line 3' "$case_root/output" || fail 'log tail omitted last line'
if grep -Fq 'fixture log line 1' "$case_root/output"; then fail 'log tail limit was ignored'; fi
run_make pipeline-logs FAILED_ONLY=0 JOB=456 RUN_ID=123 || fail 'explicit source run/job logs failed'
export PMTOOL_TEST_WRONG_JOB=1
refused 'JOB does not belong' pipeline-logs JOB=999
unset PMTOOL_TEST_WRONG_JOB
refused 'TAIL must' pipeline-logs TAIL=0
refused 'WATCH must' pipeline-jobs WATCH=bad
export PMTOOL_TEST_WRONG_SHA=1
refused 'Pipeline SHA, ref or event differs' pipeline-jobs
unset PMTOOL_TEST_WRONG_SHA
export PMTOOL_TEST_LOG_FAIL=1
refused 'Error' pipeline-logs
unset PMTOOL_TEST_LOG_FAIL
export PMTOOL_TEST_NO_RUN=1
refused 'No valid matching push workflow' pipeline-jobs
unset PMTOOL_TEST_NO_RUN
test "$(git show-ref)" = "$refs_before" && test "$(git ls-remote --heads --tags origin)" = "$remote_before" || fail 'reading pipelines/logs mutated refs'

export PMTOOL_TEST_CI_FAIL=1
refused 'Error' mr VERSION=v1.0.1
unset PMTOOL_TEST_CI_FAIL
test "$(cat .git/source-pr-state)" = OPEN || fail 'failed CI merged the PR'
test "$(git ls-remote --heads --tags origin)" = "$remote_before" || fail 'failed PR CI mutated remote refs'
run_make mr VERSION=v1.0.1 || { cat "$case_root/output" >&2; fail 'source PR/merge/main/artifacts failed'; }
test "$(cat .git/source-pr-state)" = MERGED && test "$(git branch --show-current)" = main || fail 'PR merge did not synchronize main'
test "$(wc -l < .git/source-pr-created | tr -d ' ')" = 1 || fail 'retry duplicated an open PR'
grep -Fq 'Fixture release notes.' .git/source-pr-body || fail 'source PR description omitted release notes'
main=$(git rev-parse HEAD)
test "$main" = "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" || fail 'source main diverged after merge'
test -f "dist/ci/$main/.workflow-run-id" || fail 'combined MR omitted main CI artifacts'
test -z "$(git ls-remote --tags origin)" || fail 'source MR published a tag'

source_setup source-submit-push-failure
run_make release-prepare VERSION=v1.0.1 || fail 'source prepare failed'
source_edit
cat > "$case_root/remote.git/hooks/pre-receive" <<'HOOK'
#!/bin/sh
set -eu
while read -r old new ref; do
  case "$ref" in refs/heads/release/*) exit 1 ;; esac
done
HOOK
chmod +x "$case_root/remote.git/hooks/pre-receive"
refused 'Commit preserved' release-submit VERSION=v1.0.1 'MESSAGE=Preserve source fixture'
head=$(git rev-parse HEAD)
test "$(git log -1 --format=%s)" = 'chore(release): prepare v1.0.1' || fail 'push failure lost preparation commit'
rm "$case_root/remote.git/hooks/pre-receive"
run_make release-submit VERSION=v1.0.1 || fail 'source submit retry failed'
test "$(git rev-parse HEAD)" = "$head" || fail 'source submit retry rewrote commits'

source_setup source-pr-head-changed
source_submit
before=$(git ls-remote --heads --tags origin)
export PMTOOL_TEST_PR_WRONG_SHA=1
refused 'Release PR state, branch, base or SHA changed' mr VERSION=v1.0.1
unset PMTOOL_TEST_PR_WRONG_SHA
test "$(git ls-remote --heads --tags origin)" = "$before" && test "$(cat .git/source-pr-state)" = OPEN || fail 'changed PR SHA was merged'

source_setup source-multiple-prs
source_submit
export PMTOOL_TEST_MULTIPLE_PRS=1
refused 'exactly one open release PR' mr VERSION=v1.0.1
unset PMTOOL_TEST_MULTIPLE_PRS
test ! -e .git/source-pr-created || fail 'ambiguous PR query created another request'

source_setup source-duplicate-notes
run_make release-prepare VERSION=v1.0.1 || fail 'source prepare failed'
printf '# Changelog\n\n## Unreleased\n\n## Unreleased\n\n## Release 1.0.0\n' > CHANGELOG.md
git add CHANGELOG.md
git commit --quiet -m duplicate-notes
head=$(git rev-parse HEAD)
refused 'Exactly one Unreleased heading' release-finalize VERSION=v1.0.1
test "$(git rev-parse HEAD)" = "$head" && test -z "$(git status --porcelain)" || fail 'refused finalization changed files or commits'

source_setup source-artifact-failure-after-merge
source_submit
export PMTOOL_TEST_DOWNLOAD_FAIL=1
refused 'Merge and synchronized main are preserved' mr VERSION=v1.0.1
unset PMTOOL_TEST_DOWNLOAD_FAIL
test "$(cat .git/source-pr-state)" = MERGED && test "$(git branch --show-current)" = main || fail 'artifact failure lost merged state'
refs_before=$(git ls-remote --heads --tags origin)
run_make ci-artifacts || fail 'artifact recovery after merge failed'
test "$(git ls-remote --heads --tags origin)" = "$refs_before" || fail 'artifact recovery changed refs'
test "$(wc -l < .git/source-pr-created | tr -d ' ')" = 1 || fail 'artifact recovery duplicated the PR'

source_setup source-release-cleanup
source_submit
run_make mr VERSION=v1.0.1 || { cat "$case_root/output" >&2; fail 'cleanup fixture merge failed'; }
refused 'annotated local release tag' release-cleanup VERSION=v1.0.1

branch_ref=refs/heads/release/1.0.1
tag_ref=refs/tags/v1.0.1
branch_tip=$(git rev-parse "$branch_ref")
git tag -a unrelated -m 'Unrelated local tag'
git config push.followTags true
git config --add remote.origin.pushurl "$scratch/other.git"
refused 'same fetch and push URL' release-cleanup VERSION=v1.0.1
git config --unset-all remote.origin.pushurl

export PMTOOL_TEST_RELEASE_DRAFT=true
refused 'Publication workflow for v1.0.1 succeeded. Retry branch cleanup' push-release VERSION=v1.0.1
unset PMTOOL_TEST_RELEASE_DRAFT
before=$(git show-ref)
remote_before=$(git ls-remote origin)
git show-ref --verify --quiet "$branch_ref" || fail 'draft publication removed local branch'
grep -Fq "$branch_ref" "$case_root/output" && fail 'draft publication attempted deletion'

export PMTOOL_TEST_RELEASE_MISSING=1
refused 'release not found' release-cleanup VERSION=v1.0.1
unset PMTOOL_TEST_RELEASE_MISSING
export PMTOOL_TEST_RELEASE_PRERELEASE=true
refused 'published stable GitHub release' release-cleanup VERSION=v1.0.1
unset PMTOOL_TEST_RELEASE_PRERELEASE

run_make release-cleanup-noop VERSION=v1.0.1 || { cat "$case_root/output" >&2; fail 'cleanup preview failed'; }
grep -Fq '[NOOP] Would remove release/1.0.1' "$case_root/output" || fail 'cleanup preview omitted the branch'
test "$(git show-ref)" = "$before" && test "$(git ls-remote origin)" = "$remote_before" || fail 'cleanup preview mutated refs'

git worktree add --quiet "$case_root/release-worktree" release/1.0.1
refused 'checked out in a worktree' release-cleanup VERSION=v1.0.1
git worktree remove "$case_root/release-worktree"

git branch --force release/1.0.1 main >/dev/null
refused 'release refs disagree' release-cleanup VERSION=v1.0.1
git branch --force release/1.0.1 "$branch_tip" >/dev/null

cat > "$case_root/remote.git/hooks/pre-receive" <<'HOOK'
#!/bin/sh
exit 1
HOOK
chmod +x "$case_root/remote.git/hooks/pre-receive"
refused 'pre-receive hook declined' release-cleanup VERSION=v1.0.1
test "$(git show-ref)" = "$before" && test "$(git ls-remote origin)" = "$remote_before" || fail 'rejected cleanup changed refs'
rm "$case_root/remote.git/hooks/pre-receive"

run_make push-release-tag VERSION=v1.0.1 || { cat "$case_root/output" >&2; fail 'publication resume did not clean branch'; }
grep -Fq '[NOOP] Would remove release/1.0.1' "$case_root/output" || fail 'publication omitted cleanup preview'
grep -Fq '[OK] release/1.0.1 cleaned; v1.0.1 preserved' "$case_root/output" || fail 'publication omitted cleanup'
if git show-ref --verify --quiet "$branch_ref"; then fail 'cleanup kept local branch'; fi
if git show-ref --verify --quiet refs/remotes/origin/release/1.0.1; then fail 'cleanup kept tracking branch'; fi
test "$(git ls-remote origin)" = "$(printf '%s\n' "$remote_before" | awk -v ref="$branch_ref" '$2 != ref')" || fail 'cleanup changed unrelated remote refs'
test "$(git rev-parse "$tag_ref")" = "$(printf '%s\n' "$before" | awk -v ref="$tag_ref" '$2 == ref {print $1}')" || fail 'cleanup changed local tag'
run_make release-cleanup VERSION=v1.0.1 || fail 'completed cleanup was not idempotent'
grep -Fq 'already clean' "$case_root/output" || fail 'completed cleanup did not report its state'

git branch release/1.0.1 "$branch_tip"
git update-ref refs/remotes/origin/release/1.0.1 "$branch_tip"
remote_before=$(git ls-remote origin)
run_make release-cleanup VERSION=v1.0.1 || { cat "$case_root/output" >&2; fail 'local-only cleanup could not resume'; }
if git show-ref --verify --quiet "$branch_ref" || git show-ref --verify --quiet refs/remotes/origin/release/1.0.1; then
  fail 'local-only cleanup retained a release ref'
fi
test "$(git ls-remote origin)" = "$remote_before" || fail 'local-only cleanup changed remote refs'

source_setup source-cleanup-unmerged
source_submit
git switch --quiet main
git tag -a v1.0.1 -m 'tag excluding unmerged branch'
git push --quiet origin refs/tags/v1.0.1
before=$(git show-ref)
remote_before=$(git ls-remote origin)
refused 'must be merged into main and included' release-cleanup VERSION=v1.0.1
test "$(git show-ref)" = "$before" && test "$(git ls-remote origin)" = "$remote_before" || fail 'unmerged cleanup changed refs'

printf '[OK] Source branches, separate preparation, submission, PR, pipeline/log and cleanup regressions passed\n'
printf '[OK] Source release target and commit-artifact regressions passed\n'

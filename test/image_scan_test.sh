#!/usr/bin/env bash
set -euo pipefail

source_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/pmtool-image-scan.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
mkdir -p "$scratch/bin"
export PMTOOL_SCAN_LOG="$scratch/docker.log" PMTOOL_SCAN_ARCHIVES="$scratch/archives"
export PATH="$scratch/bin:$PATH" NO_COLOR=1

assert_no_match() {
  local grep_status=0
  grep "$@" || grep_status=$?
  if (( grep_status != 1 )); then
    printf '[FAIL] Expected no match; grep exited %s\n' "$grep_status" >&2
    exit 1
  fi
}

cat > "$scratch/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$PMTOOL_SCAN_LOG"
case "$1 $2" in
  'image inspect') exit "${PMTOOL_SCAN_INSPECT_STATUS:-0}" ;;
  'pull upstream:test') exit "${PMTOOL_SCAN_PULL_STATUS:-0}" ;;
  'image save')
    printf '%s\n' "$4" >> "$PMTOOL_SCAN_ARCHIVES"
    printf 'fixture archive' > "$4"
    ;;
  'run --rm')
    case " $* " in *' --exit-code 42 '*) ;; *) exit 99 ;; esac
    printf 'Trivy fixture: %s\n' "${PMTOOL_SCAN_STATUS:-0}"
    exit "${PMTOOL_SCAN_STATUS:-0}"
    ;;
  *) exit 98 ;;
esac
DOCKER
chmod +x "$scratch/bin/docker"

for scan_status in 0 42 1 125; do
  export PMTOOL_SCAN_STATUS="$scan_status"
  status=0
  make --no-print-directory -C "$source_root" voxbox-security VOXBOX_IMAGE=upstream:test > "$scratch/output" 2>&1 || status=$?
  case "$scan_status" in
    0) test "$status" = 0; grep -Fq '[OK]' "$scratch/output" ;;
    42) test "$status" = 0; grep -Fq '[WARN]' "$scratch/output"; assert_no_match -Fq '[OK]' "$scratch/output" ;;
    *) test "$status" != 0; grep -Fq '[ERROR]' "$scratch/output"; assert_no_match -Fq '[OK]' "$scratch/output" ;;
  esac
done

export PMTOOL_SCAN_INSPECT_STATUS=1 PMTOOL_SCAN_PULL_STATUS=1
if make --no-print-directory -C "$source_root" voxbox-security VOXBOX_IMAGE=upstream:test > "$scratch/output" 2>&1; then
  printf '%s\n' 'Image pull failure was hidden' >&2
  exit 1
fi
assert_no_match -Eq '\[OK\]|\[WARN\]' "$scratch/output"
assert_no_match -Fq docker.sock "$PMTOOL_SCAN_LOG"
grep -Fq -- '--scanners vuln' "$PMTOOL_SCAN_LOG"
grep -Fq -- '--severity HIGH,CRITICAL --ignore-unfixed' "$PMTOOL_SCAN_LOG"
while IFS= read -r archive; do test ! -e "$archive"; done < "$PMTOOL_SCAN_ARCHIVES"
printf '%s\n' '[OK] Image scan advisory, scanner failure and archive cleanup regressions passed'

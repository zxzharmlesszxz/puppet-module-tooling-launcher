VOXBOX_IMAGE ?= ghcr.io/voxpupuli/voxbox:10.1.0-openvox8@sha256:01bb8ce5e24d5b111934660f75e2786b4d7c7adcaa7518a564856d1d3041d10d
WORKFLOW_IMAGE ?= $(VOXBOX_IMAGE)
GH ?= gh
RELEASE_REF ?= main
RELEASE_WORKFLOW ?= launcher.yml
CI_ARTIFACT_ROOT ?= $(DIST)/ci
CI_ARTIFACT_DIR ?=
CI_RUN_FILE ?=
GO_IMAGE ?= golang:1.27.3
VERSION ?= dev
IMAGE ?= $(VOXBOX_IMAGE)
DIST ?= dist
CHECK_IMAGE ?= pmtool-checks:local
GOLANGCI_LINT_VERSION ?= v2.15.0
GOVULNCHECK_VERSION ?= v1.9.0
GOSEC_VERSION ?= v2.30.0
ACTIONLINT_VERSION ?= v1.7.12
TRIVY_IMAGE ?= aquasec/trivy:0.76.1@sha256:790fd66d1697395d8693d1acaa1ea702406a20f109b6fc7bac8db623eca6c06c
TRIVY_CACHE_VOLUME ?= pmtool-trivy-cache
COVERAGE_THRESHOLD ?= 65.0
export COVERAGE_THRESHOLD
PLATFORMS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64
export VERSION IMAGE DIST
.DEFAULT_GOAL := help
.PHONY: help clean format tidy deps-update test build build-native validate-version checks-image fmt-check vet lint coverage-check test-race govulncheck gosec workflow-lint security-go trivy-filesystem check full-check fmt-check-native vet-native lint-native test-native coverage-check-native test-race-native govulncheck-native gosec-native workflow-lint-native release-smoke-native

help: ## Show available make targets.
	@set -eu; \
	printf "\033[33mUsage:\033[0m\n"; \
	grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

format: ## Format Go source files.
	docker run --rm --network none --user "$$(id -u):$$(id -g)" --volume '$(CURDIR):/src' --workdir /src '$(GO_IMAGE)' gofmt -w cmd internal/pmtool internal/hostflow internal/templatekit internal/workflow/assets.go docs

tidy: ## Run go mod tidy and update module metadata.
	docker run --rm --read-only --tmpfs /tmp:exec --user "$$(id -u):$$(id -g)" --volume '$(CURDIR):/src' --workdir /src --env HOME=/tmp --env GOCACHE=/tmp/go-cache --env GOPATH=/tmp/go-path '$(GO_IMAGE)' go mod tidy

deps-update: ## Update Go module dependencies and tidy module metadata.
	docker run --rm --read-only --tmpfs /tmp:exec --user "$$(id -u):$$(id -g)" --volume '$(CURDIR):/src' --workdir /src --env HOME=/tmp --env GOCACHE=/tmp/go-cache --env GOPATH=/tmp/go-path '$(GO_IMAGE)' sh -ec 'go get -u ./...; go mod tidy'

checks-image: ## Build the pinned lint, security and shell-test tool image.
	docker build --file Dockerfile.checks --build-arg 'GO_IMAGE=$(GO_IMAGE)' --build-arg 'GOLANGCI_LINT_VERSION=$(GOLANGCI_LINT_VERSION)' --build-arg 'GOVULNCHECK_VERSION=$(GOVULNCHECK_VERSION)' --build-arg 'GOSEC_VERSION=$(GOSEC_VERSION)' --build-arg 'ACTIONLINT_VERSION=$(ACTIONLINT_VERSION)' --tag '$(CHECK_IMAGE)' .

fmt-check vet lint test coverage-check test-race gosec workflow-lint release-targets-check image-scan-check: checks-image
	docker run --rm --network none --read-only --tmpfs /tmp:exec --volume '$(CURDIR):/src:ro' --workdir /src --env HOME=/tmp --env GOCACHE=/tmp/go-cache --env GOPATH=/tmp/go-path --env GOLANGCI_LINT_CACHE=/tmp/lint-cache --env COVERAGE_THRESHOLD '$(CHECK_IMAGE)' make --no-print-directory $@-native

govulncheck: checks-image ## Scan reachable Go code for known vulnerabilities.
	docker run --rm --read-only --tmpfs /tmp:exec --volume '$(CURDIR):/src:ro' --workdir /src --env HOME=/tmp --env GOCACHE=/tmp/go-cache --env GOPATH=/tmp/go-path '$(CHECK_IMAGE)' make --no-print-directory govulncheck-native

fmt-check-native:
	@set -eu; \
	unformatted="$$(gofmt -l cmd internal/pmtool internal/hostflow internal/templatekit internal/workflow/assets.go docs)"; \
	if [ -n "$$unformatted" ]; then printf '%s\n' "$$unformatted"; exit 1; fi

vet-native:
	go vet ./...

lint-native:
	golangci-lint run --config .golangci.yml

test-native:
	go test ./...

coverage-check-native:
	@set -eu; \
	go test -covermode=atomic -coverprofile=/tmp/pmtool-coverage.out ./...; \
	go tool cover -func=/tmp/pmtool-coverage.out > /tmp/pmtool-coverage.txt; \
	cat /tmp/pmtool-coverage.txt; \
	awk -v threshold="$$COVERAGE_THRESHOLD" 'BEGIN { if (threshold !~ /^[0-9]+([.][0-9]+)?$$/ || threshold+0 > 100) { print "invalid coverage threshold"; exit 2 } } /^total:/ { gsub(/%/, "", $$3); found=1; if ($$3+0 < threshold+0) { printf "coverage %.1f%% is below %.1f%%\n", $$3, threshold; exit 1 } } END { if (!found) exit 1 }' /tmp/pmtool-coverage.txt

test-race-native:
	CGO_ENABLED=1 go test -race ./...

govulncheck-native:
	govulncheck ./...

gosec-native:
	gosec -exclude-generated ./...

workflow-lint-native:
	actionlint

release-targets-check-native:
	sh test/source_release_test.sh

image-scan-check-native:
	bash test/image_scan_test.sh

security-go: govulncheck gosec ## Run Go vulnerability and security scans.

check: fmt-check vet lint coverage-check workflow-lint release-targets-check image-scan-check ## Run formatting, vet, lint, coverage and workflow validation.

full-check: check test-race security-go workflow-check trivy-filesystem ## Run check, race tests and all security scans.

build: full-check ## Run full validation, cross-build binaries and smoke-test them. VERSION=vX.Y.Z.
	set -eu; \
	docker run --rm --network none --user "$$(id -u):$$(id -g)" --volume '$(CURDIR):/repo' --workdir /repo --env HOME=/tmp --env GOCACHE=/tmp/go-cache --env GOPATH=/tmp/go-path --env CGO_ENABLED=0 --env VERSION --env IMAGE --env DIST '$(CHECK_IMAGE)' sh -eu -c 'cache="$$(mktemp -d)"; cp -R "$${GOMODCACHE:?}/." "$$cache/"; chmod -R u+w "$$cache"; export GOMODCACHE="$$cache"; exec make --no-print-directory build-native'; \
	docker run --rm --read-only --network none --tmpfs /tmp:exec --volume '$(CURDIR):/src:ro' --workdir /src --env HOME=/tmp --env VERSION --env DIST '$(CHECK_IMAGE)' make --no-print-directory release-smoke-native

validate-version: ## Validate VERSION: dev, dev-COMMIT_SHA, X.Y.Z or vX.Y.Z.
	@set -eu; \
	if [ "$$VERSION" = dev ]; then exit 0; fi; \
	if [ "$${#VERSION}" -eq 44 ] && printf '%s\n' "$$VERSION" | grep -Eq '^dev-[0-9a-f]{40}$$'; then exit 0; fi; \
	invalid_chars="$$(printf '%s' "$$VERSION" | LC_ALL=C tr -d '0-9v.' | wc -c)"; \
	if [ "$$invalid_chars" -ne 0 ] || ! printf '%s\n' "$$VERSION" | grep -Eq '^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$$'; then \
	  printf '%s\n' 'VERSION must be dev, dev-COMMIT_SHA, X.Y.Z or vX.Y.Z' >&2; \
	  exit 1; \
	fi

build-platform-%: validate-version
	@set -eu; \
	build_version="$$(printf '%s\n' "$$VERSION" | sed -e 's/^v//' -e '/^dev/!s/^/v/')"; \
	output="$$DIST/$$build_version"; \
	mkdir -p "$$output"; \
	GOOS=$(word 1,$(subst -, ,$*)) GOARCH=$(word 2,$(subst -, ,$*)) go build -trimpath \
	  -ldflags "-s -w -X github.com/zxzharmlesszxz/puppet-module-tooling-launcher/internal/pmtool.version=$$build_version -X github.com/zxzharmlesszxz/puppet-module-tooling-launcher/internal/pmtool.defaultImage=$$IMAGE" \
	  -o "$$output/pmtool-$$build_version-$*" ./cmd/pmtool

build-native: validate-version $(addprefix build-platform-,$(PLATFORMS))
	@set -eu; \
	build_version="$$(printf '%s\n' "$$VERSION" | sed -e 's/^v//' -e '/^dev/!s/^/v/')"; \
	cp LICENSE "$$DIST/$$build_version/LICENSE"; \
	cd "$$DIST/$$build_version"; \
	sha256sum $(foreach platform,$(PLATFORMS),"pmtool-$$build_version-$(platform)") LICENSE > SHA256SUMS; \
	printf 'Built pmtool %s: %s/%s\n' "$$build_version" "$$DIST" "$$build_version"

trivy-filesystem: ## Scan repository vulnerabilities, secrets, licenses and configuration.
	set -eu; \
	docker run --rm --volume '$(TRIVY_CACHE_VOLUME):/root/.cache/trivy' --volume '$(CURDIR):/work:ro' --workdir /work '$(TRIVY_IMAGE)' fs --scanners vuln,secret,license --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1 --skip-dirs .git,.idea,dist .; \
	docker run --rm --volume '$(TRIVY_CACHE_VOLUME):/root/.cache/trivy' --volume '$(CURDIR):/work:ro' --workdir /work '$(TRIVY_IMAGE)' fs --config .trivy-misconfig.yaml --scanners misconfig .

release-smoke-native: validate-version
	@set -eu; \
	build_version="$$(printf '%s\n' "$$VERSION" | sed -e 's/^v//' -e '/^dev/!s/^/v/')"; \
	build_os="$$(go env GOOS)"; \
	build_arch="$$(go env GOARCH)"; \
	cd "$$DIST/$$build_version"; \
	sha256sum -c SHA256SUMS; \
	grep -Fxq 'MIT License' LICENSE; \
	binary="./pmtool-$$build_version-$$build_os-$$build_arch"; \
	actual="$$($$binary version)"; \
	test "$$actual" = "pmtool $$build_version ($$build_os/$$build_arch)"; \
	"$$binary" help > /tmp/pmtool-smoke-help.txt; \
	grep -q pmtool /tmp/pmtool-smoke-help.txt; \
	"$$binary" completion bash > /tmp/pmtool-smoke.bash; \
	"$$binary" completion zsh > /tmp/pmtool-smoke.zsh; \
	bash -n /tmp/pmtool-smoke.bash; \
	zsh -n /tmp/pmtool-smoke.zsh; \
	printf 'Native release smoke passed for pmtool %s\n' "$$build_version"

fmt-check: ## Check Go formatting without changing files.
vet: ## Run go vet.
lint: ## Run golangci-lint.
test: ## Run Go unit and shell-completion tests.
coverage-check: ## Run unit tests and enforce COVERAGE_THRESHOLD.
test-race: ## Run Go tests with the race detector.
gosec: ## Run Go security static analysis.
workflow-lint: ## Validate GitHub Actions with actionlint.

clean: ## Remove generated binaries, checksums and local coverage reports.
	@set -eu; \
	case '$(abspath $(DIST))' in \
	  '$(CURDIR)'/*) ;; \
	  *) printf '%s\n' 'DIST must be a subdirectory of the checkout' >&2; exit 1 ;; \
	esac; \
	rm -rf -- '$(DIST)'; \
	rm -f -- coverage*.out coverage*.txt; \
	printf '%s\n' 'Removed generated binaries, checksums and local coverage reports.'

export VERSION GH RELEASE_REF RELEASE_WORKFLOW CI_ARTIFACT_ROOT CI_ARTIFACT_DIR CI_RUN_FILE
.PHONY: release-preflight release-version-check release-worktree-check release-tag-check release-main-check release-ci-wait ci-artifacts release-artifacts-check push-main release-tag push-release-tag push-release

release-preflight: release-version-check ## Run all local release checks without Git mutations or publication.
	$(MAKE) --no-print-directory build

release-version-check: ## Validate canonical VERSION=vX.Y.Z against the latest changelog release.
	@set -eu; \
	invalid="$$(printf '%s' "$$VERSION" | LC_ALL=C tr -d '0-9v.' | wc -c)"; \
	if [ "$$invalid" -ne 0 ] || ! printf '%s\n' "$$VERSION" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$$'; then \
	  printf '%s\n' 'VERSION must use canonical vMAJOR.MINOR.PATCH syntax' >&2; exit 2; \
	fi; \
	test -f CHANGELOG.md || { printf '%s\n' 'CHANGELOG.md is required for release' >&2; exit 2; }; \
	latest="$$(awk '/^## Release / { sub(/^## Release /, ""); print; exit }' CHANGELOG.md)"; \
	test "$$latest" = "$${VERSION#v}" || { printf '%s\n' 'VERSION differs from the latest changelog release' >&2; exit 2; }; \
	printf '[OK] Release version %s\n' "$$VERSION"

release-worktree-check: ## Require a committed, clean main branch; never reset or stash work.
	@set -eu; \
	git rev-parse --verify HEAD >/dev/null 2>&1 || { printf '%s\n' 'Commit the reviewed source before release' >&2; exit 2; }; \
	test "$$(git branch --show-current)" = main || { printf '%s\n' 'Release must run from main' >&2; exit 2; }; \
	test -z "$$(git status --porcelain --untracked-files=all)" || { printf '%s\n' 'Working tree is dirty; preserve and commit reviewed changes first' >&2; exit 2; }

release-tag-check: release-version-check ## Refuse an existing local or remote release tag; query failures are fatal.
	@set -eu; \
	if git show-ref --verify --quiet "refs/tags/$$VERSION"; then \
	  printf 'Local tag exists: %s; use push-release-tag to resume an interrupted tag push\n' "$$VERSION" >&2; exit 2; \
	fi; \
	remote_tags="$$(git ls-remote --tags origin "refs/tags/$$VERSION" "refs/tags/$$VERSION^{}")"; \
	test -z "$$remote_tags" || { printf 'Remote tag already exists: %s\n' "$$VERSION" >&2; exit 2; }

release-main-check: ## Require origin/main to point to the current local commit.
	@set -eu; \
	remote_refs="$$(git ls-remote --heads origin refs/heads/main)"; \
	remote_head="$$(printf '%s\n' "$$remote_refs" | awk 'NR == 1 { print $$1 }')"; \
	test "$$remote_head" = "$$(git rev-parse HEAD)" || { printf '%s\n' 'origin/main differs from local HEAD' >&2; exit 2; }

release-ci-wait: ## Wait for the pinned SHA/ref push workflow. RELEASE_REF defaults to main.
	@set -eu; \
	head="$$(git rev-parse HEAD)"; \
	origin="$$(git remote get-url origin)"; \
	repo_url="$$("$$GH" repo view "$$origin" --json url --jq .url)"; \
	repo="$${repo_url#https://}"; \
	run_id=; attempt=0; \
	while [ -z "$$run_id" ]; do \
	  run_id="$$("$$GH" run list --repo "$$repo" --workflow "$$RELEASE_WORKFLOW" \
	    --branch "$$RELEASE_REF" --commit "$$head" --event push --limit 100 \
	    --json databaseId --jq 'max_by(.databaseId).databaseId // empty')"; \
	  test -n "$$run_id" && break; \
	  attempt=$$((attempt + 1)); \
	  test "$$attempt" -lt 12 || { printf '%s\n' 'No matching push workflow appeared within 60 seconds' >&2; exit 2; }; \
	  printf '[WAIT] %s workflow for %s\n' "$$RELEASE_REF" "$$head"; sleep 5; \
	done; \
	case "$$run_id" in ''|*[!0-9]*) printf '%s\n' 'Invalid workflow run ID' >&2; exit 2 ;; esac; \
	"$$GH" run watch "$$run_id" --repo "$$repo" --interval 5 --exit-status; \
	actual="$$("$$GH" run view "$$run_id" --repo "$$repo" --json headSha,headBranch,event,conclusion \
	  --jq '[.headSha, .headBranch, .event, .conclusion] | @tsv')"; \
	expected="$$(printf '%s\t%s\tpush\tsuccess' "$$head" "$$RELEASE_REF")"; \
	test "$$actual" = "$$expected" || { printf '%s\n' 'Workflow ref, SHA, event or conclusion changed' >&2; exit 2; }; \
	test "$$(git rev-parse HEAD)" = "$$head" || { printf '%s\n' 'Local HEAD changed while waiting' >&2; exit 2; }; \
	if [ -n "$${CI_RUN_FILE:-}" ]; then printf '%s\n' "$$run_id" > "$$CI_RUN_FILE"; fi; \
	printf '[OK] %s workflow %s at %s\n' "$$RELEASE_REF" "$$run_id" "$$head"

ci-artifacts: ## Download and smoke-test the successful main CI artifacts for the current commit; never push or tag.
	@set -eu; \
	$(MAKE) --no-print-directory release-worktree-check; \
	$(MAKE) --no-print-directory release-main-check; \
	head="$$(git rev-parse HEAD)"; \
	mkdir -p "$$CI_ARTIFACT_ROOT"; \
	temporary="$$(mktemp -d "$$CI_ARTIFACT_ROOT/.download.XXXXXX")"; \
	trap 'rm -rf -- "$$temporary"' EXIT; \
	$(MAKE) --no-print-directory release-ci-wait RELEASE_REF=main CI_RUN_FILE="$$temporary/run-id"; \
	run_id="$$(cat "$$temporary/run-id")"; \
	origin="$$(git remote get-url origin)"; \
	repo_url="$$("$$GH" repo view "$$origin" --json url --jq .url)"; \
	repo="$${repo_url#https://}"; \
	"$$GH" run download "$$run_id" --repo "$$repo" --name "pmtool-dev-$$head" --dir "$$temporary/artifact"; \
	cp "$$temporary/run-id" "$$temporary/artifact/.workflow-run-id"; \
	$(MAKE) --no-print-directory release-artifacts-check CI_ARTIFACT_DIR="$$temporary/artifact"; \
	$(MAKE) --no-print-directory release-worktree-check; \
	test "$$(git rev-parse HEAD)" = "$$head" || { printf '%s\n' 'Local HEAD changed during artifact verification' >&2; exit 2; }; \
	$(MAKE) --no-print-directory release-main-check; \
	destination="$$CI_ARTIFACT_ROOT/$$head"; \
	if [ -e "$$destination" ] || [ -L "$$destination" ]; then \
	  $(MAKE) --no-print-directory release-artifacts-check CI_ARTIFACT_DIR="$$destination"; \
	  cmp "$$temporary/artifact/SHA256SUMS" "$$destination/SHA256SUMS" || { printf '%s\n' 'Existing CI artifacts differ; preserve them and inspect the runs' >&2; exit 2; }; \
	else \
	  mv "$$temporary/artifact" "$$destination"; \
	fi; \
	destination="$$(cd "$$destination" && pwd -P)"; \
	printf '[OK] Verified CI artifacts: %s\n' "$$destination"; \
	platform="$$(uname -s | tr '[:upper:]' '[:lower:]')"; \
	case "$$(uname -m)" in arm64|aarch64) architecture=arm64 ;; *) architecture=amd64 ;; esac; \
	printf '[OK] Native CI binary: %s/pmtool-dev-%s-%s-%s\n' "$$destination" "$$head" "$$platform" "$$architecture"; \
	printf '%s\n' '[NEXT] Test the downloaded binary with your template/module before make push-release with the reviewed stable VERSION.'

release-artifacts-check: ## Require verified commit CI artifacts, exact source/run identity, valid checksums and native smoke before tagging.
	@set -eu; \
	$(MAKE) --no-print-directory release-worktree-check; \
	$(MAKE) --no-print-directory release-main-check; \
	head="$$(git rev-parse HEAD)"; version="dev-$$head"; \
	artifact_dir="$${CI_ARTIFACT_DIR:-$$CI_ARTIFACT_ROOT/$$head}"; \
	if [ ! -d "$$artifact_dir" ] || [ -L "$$artifact_dir" ] || [ ! -f "$$artifact_dir/.workflow-run-id" ] || [ -L "$$artifact_dir/.workflow-run-id" ]; then \
	  printf '%s\n' 'Verified CI artifacts are missing; run make ci-artifacts for the current main commit before tagging' >&2; exit 2; \
	fi; \
	run_id="$$(cat "$$artifact_dir/.workflow-run-id")"; \
	case "$$run_id" in ''|*[!0-9]*) printf '%s\n' 'Invalid artifact workflow run ID' >&2; exit 2 ;; esac; \
	origin="$$(git remote get-url origin)"; \
	repo_url="$$("$$GH" repo view "$$origin" --json url --jq .url)"; \
	repo="$${repo_url#https://}"; \
	actual="$$("$$GH" run view "$$run_id" --repo "$$repo" --json headSha,headBranch,event,conclusion --jq '[.headSha, .headBranch, .event, .conclusion] | @tsv')"; \
	expected="$$(printf '%s\tmain\tpush\tsuccess' "$$head")"; \
	test "$$actual" = "$$expected" || { printf '%s\n' 'Artifact workflow ref, SHA, event or conclusion differs from the current main commit' >&2; exit 2; }; \
	for file in SHA256SUMS LICENSE $(foreach platform,linux-amd64 linux-arm64 darwin-amd64 darwin-arm64,"pmtool-$$version-$(platform)"); do \
	  test -f "$$artifact_dir/$$file" && test ! -L "$$artifact_dir/$$file" || { printf 'Missing or nonregular CI artifact: %s\n' "$$file" >&2; exit 2; }; \
	done; \
	awk -v version="$$version" 'BEGIN { names["LICENSE"]=1; names["pmtool-" version "-linux-amd64"]=1; names["pmtool-" version "-linux-arm64"]=1; names["pmtool-" version "-darwin-amd64"]=1; names["pmtool-" version "-darwin-arm64"]=1 } { if (NF != 2 || length($$1) != 64 || $$1 ~ /[^0-9a-f]/ || !($$2 in names) || seen[$$2]++) bad=1; count++ } END { if (bad || count != 5) exit 1 }' "$$artifact_dir/SHA256SUMS" || { printf '%s\n' 'Invalid CI artifact checksum manifest' >&2; exit 2; }; \
	(cd "$$artifact_dir"; if command -v sha256sum >/dev/null 2>&1; then sha256sum -c SHA256SUMS; else shasum -a 256 -c SHA256SUMS; fi); \
	cmp LICENSE "$$artifact_dir/LICENSE" || { printf '%s\n' 'CI artifact license differs from the committed license' >&2; exit 2; }; \
	case "$$(uname -s)" in Darwin) platform=darwin ;; Linux) platform=linux ;; *) printf '%s\n' 'Unsupported host OS for CI artifact smoke' >&2; exit 2 ;; esac; \
	case "$$(uname -m)" in arm64|aarch64) architecture=arm64 ;; x86_64|amd64) architecture=amd64 ;; *) printf '%s\n' 'Unsupported host architecture for CI artifact smoke' >&2; exit 2 ;; esac; \
	binary="$$artifact_dir/pmtool-$$version-$$platform-$$architecture"; \
	chmod u+x "$$binary"; \
	actual="$$("$$binary" version)"; \
	test "$$actual" = "pmtool $$version ($$platform/$$architecture)" || { printf '%s\n' 'CI binary version differs from the current main commit/platform' >&2; exit 2; }; \
	"$$binary" help > /dev/null; \
	$(MAKE) --no-print-directory release-worktree-check; \
	test "$$(git rev-parse HEAD)" = "$$head" || { printf '%s\n' 'Local HEAD changed during artifact smoke' >&2; exit 2; }; \
	$(MAKE) --no-print-directory release-main-check; \
	printf '[OK] CI artifact checksums and native smoke for %s (run %s)\n' "$$head" "$$run_id"

push-main: ## Push clean main, wait for exact-SHA CI, download its artifacts and smoke-test the native binary; never tag.
	@set -eu; \
	$(MAKE) --no-print-directory release-worktree-check; \
	head="$$(git rev-parse HEAD)"; \
	origin="$$(git remote get-url origin)"; \
	"$$GH" repo view "$$origin" --json url --jq .url >/dev/null; \
	git -c push.followTags=false push origin HEAD:refs/heads/main; \
	$(MAKE) --no-print-directory release-main-check; \
	$(MAKE) --no-print-directory ci-artifacts; \
	test "$$(git rev-parse HEAD)" = "$$head" || { printf '%s\n' 'Local HEAD changed during main push' >&2; exit 2; }; \
	$(MAKE) --no-print-directory release-main-check

release-tag: ## Create a local stable tag after verified commit artifacts and green exact-main CI; do not push.
	@set -eu; \
	$(MAKE) --no-print-directory release-worktree-check; \
	$(MAKE) --no-print-directory release-tag-check; \
	$(MAKE) --no-print-directory release-artifacts-check; \
	head="$$(git rev-parse HEAD)"; \
	$(MAKE) --no-print-directory release-main-check; \
	$(MAKE) --no-print-directory release-ci-wait RELEASE_REF=main; \
	$(MAKE) --no-print-directory release-worktree-check; \
	test "$$(git rev-parse HEAD)" = "$$head" || { printf '%s\n' 'Local HEAD changed before tagging' >&2; exit 2; }; \
	$(MAKE) --no-print-directory release-main-check; \
	$(MAKE) --no-print-directory release-tag-check; \
	git tag -a "$$VERSION" -m "Release $$VERSION" "$$head"; \
	printf '[OK] Created local annotated tag %s\n' "$$VERSION"

push-release-tag: ## Push/resume the verified annotated tag, then wait for its publication workflow.
	@set -eu; \
	$(MAKE) --no-print-directory release-worktree-check; \
	$(MAKE) --no-print-directory release-version-check; \
	$(MAKE) --no-print-directory release-artifacts-check; \
	tag_ref="refs/tags/$$VERSION"; head="$$(git rev-parse HEAD)"; \
	tag_object="$$(git rev-parse "$$tag_ref")"; \
	test "$$(git cat-file -t "$$tag_ref")" = tag || { printf '%s\n' 'An annotated local release tag is required' >&2; exit 2; }; \
	test "$$(git rev-parse "$$tag_ref^{commit}")" = "$$head" || { printf '%s\n' 'Local tag differs from HEAD' >&2; exit 2; }; \
	$(MAKE) --no-print-directory release-main-check; \
	$(MAKE) --no-print-directory release-ci-wait RELEASE_REF=main; \
	$(MAKE) --no-print-directory release-worktree-check; \
	test "$$(git rev-parse HEAD)" = "$$head" || { printf '%s\n' 'Local HEAD changed before tag push' >&2; exit 2; }; \
	test "$$(git rev-parse "$$tag_ref")" = "$$tag_object" || { printf '%s\n' 'Local tag changed before tag push' >&2; exit 2; }; \
	$(MAKE) --no-print-directory release-main-check; \
	remote_tags="$$(git ls-remote --tags origin "$$tag_ref" "$$tag_ref^{}")"; \
	if [ -z "$$remote_tags" ]; then \
	  if ! git -c push.followTags=false push origin "$$tag_ref:$$tag_ref"; then \
	    printf 'Local tag is preserved. Retry: make push-release-tag VERSION=%s\n' "$$VERSION" >&2; exit 2; \
	  fi; \
	fi; \
	remote_tags="$$(git ls-remote --tags origin "$$tag_ref" "$$tag_ref^{}")"; \
	remote_tag="$$(printf '%s\n' "$$remote_tags" | awk -v ref="$$tag_ref" '$$2 == ref {print $$1}')"; \
	remote_head="$$(printf '%s\n' "$$remote_tags" | awk -v ref="$$tag_ref^{}" '$$2 == ref {print $$1}')"; \
	test "$$remote_tag" = "$$tag_object" && test "$$remote_head" = "$$head" || \
	  { printf '%s\n' 'Remote tag differs from the verified local annotated tag' >&2; exit 2; }; \
	$(MAKE) --no-print-directory release-ci-wait RELEASE_REF="$$VERSION"; \
	$(MAKE) --no-print-directory release-worktree-check; \
	test "$$(git rev-parse "$$tag_ref")" = "$$tag_object" || { printf '%s\n' 'Local tag changed while waiting' >&2; exit 2; }; \
	final_tags="$$(git ls-remote --tags origin "$$tag_ref" "$$tag_ref^{}")"; \
	test "$$final_tags" = "$$remote_tags" || { printf '%s\n' 'Remote tag changed while waiting' >&2; exit 2; }; \
	printf '[OK] Release %s publication workflow succeeded\n' "$$VERSION"

push-release: ## After reviewing main CI artifacts, run preflight and create/push the stable tag; main is never pushed here.
	@set -eu; \
	$(MAKE) --no-print-directory release-worktree-check; \
	$(MAKE) --no-print-directory release-tag-check; \
	$(MAKE) --no-print-directory release-artifacts-check; \
	head="$$(git rev-parse HEAD)"; \
	$(MAKE) --no-print-directory release-preflight; \
	$(MAKE) --no-print-directory release-worktree-check; \
	test "$$(git rev-parse HEAD)" = "$$head" || { printf '%s\n' 'Local HEAD changed during preflight' >&2; exit 2; }; \
	$(MAKE) --no-print-directory release-main-check; \
	$(MAKE) --no-print-directory release-tag; \
	$(MAKE) --no-print-directory push-release-tag

.PHONY: release-targets-check release-targets-check-native
release-targets-check: ## Test release guards against local Git fixtures and a fake GitHub API.

.PHONY: workflow-check
workflow-check: checks-image ## Test the Go template engine with real ERB and lint the remaining Ruby assets.
	@set -eu; \
	directory="$$(mktemp -d -t pmtool-template-check.XXXXXX)"; \
	trap 'rm -rf "$$directory"' EXIT; \
	docker run --rm --network none --read-only --tmpfs /tmp:exec --user "$$(id -u):$$(id -g)" --volume '$(CURDIR):/source:ro' --volume "$$directory:/out" --workdir /source --env HOME=/tmp --env GOCACHE=/tmp/go-cache --env GOPATH=/tmp/go-path --env CGO_ENABLED=0 '$(CHECK_IMAGE)' go test -c -o /out/templatekit.test ./internal/templatekit; \
	docker run --rm --network none --read-only --tmpfs /tmp:exec --user "$$(id -u):$$(id -g)" --volume '$(CURDIR):/source:ro' --volume "$$directory:/out:ro" --workdir /source --env HOME=/tmp --env PMTOOL_RUBY_TESTS=1 --entrypoint /out/templatekit.test '$(WORKFLOW_IMAGE)' -test.v; \
	docker run --rm --network none --read-only --tmpfs /tmp:exec --user "$$(id -u):$$(id -g)" --volume '$(CURDIR):/source:ro' --workdir /source/internal/templatekit --env HOME=/tmp --env BUNDLE_IGNORE_CONFIG=1 --env BUNDLE_GEMFILE=/opt/voxbox/Gemfile --env BUNDLE_PATH=/opt/voxbox/vendor/bundle --entrypoint bundle '$(WORKFLOW_IMAGE)' exec rubocop --config .rubocop.yml --cache false assets

.PHONY: voxbox-check voxbox-security image-scan-check image-scan-check-native
image-scan-check: ## Test advisory image findings and fatal scanner errors with a fake Docker CLI.

voxbox-check: ## Validate stock VoxBox helpers, then report upstream image vulnerabilities as warnings.
	$(MAKE) --no-print-directory workflow-check WORKFLOW_IMAGE='$(VOXBOX_IMAGE)'
	$(MAKE) --no-print-directory voxbox-security

voxbox-security: ## Report upstream image vulnerabilities without blocking use; scanner failures stay fatal.
	@set -eu; \
	docker image inspect '$(VOXBOX_IMAGE)' >/dev/null 2>&1 || docker pull '$(VOXBOX_IMAGE)'; \
	archive="$$(mktemp -t pmtool-voxbox.XXXXXX)"; \
	trap 'rm -f "$$archive"' EXIT; \
	docker image save --output "$$archive" '$(VOXBOX_IMAGE)'; \
	status=0; \
	docker run --rm --volume '$(TRIVY_CACHE_VOLUME):/root/.cache/trivy' --volume "$$archive:/scan/image.tar:ro" '$(TRIVY_IMAGE)' image --scanners vuln --input /scan/image.tar --severity HIGH,CRITICAL --ignore-unfixed --exit-code 42 || status=$$?; \
	case "$$status" in \
	  0) printf '%s\n' '[OK] No findings matching the upstream image vulnerability filters.' ;; \
	  42) \
	    yellow=; reset=; \
	    if [ -t 1 ] && [ -z "$${NO_COLOR+x}" ]; then yellow="$$(printf '\033[33m')"; reset="$$(printf '\033[0m')"; fi; \
	    printf '%s[WARN] Upstream image vulnerabilities reported above: $(VOXBOX_IMAGE). Advisory findings do not block pmtool use.%s\n' "$$yellow" "$$reset" ;; \
	  *) printf '[ERROR] Upstream image scan failed (exit %s); no successful scan result.\n' "$$status" >&2; exit "$$status" ;; \
	esac

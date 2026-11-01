GO_IMAGE ?= golang:1.27.3-alpine
VERSION ?= dev
IMAGE ?=
.DEFAULT_GOAL := help
.PHONY: help format test build

help:
	@echo 'format / test / build; VERSION=dev or X.Y.Z; IMAGE optionally sets the runtime image'

format:
	docker run --rm --network none --user "$$(id -u):$$(id -g)" --volume '$(CURDIR):/src' --workdir /src '$(GO_IMAGE)' gofmt -w .

test:
	docker run --rm --network none --read-only --tmpfs /tmp:exec --volume '$(CURDIR):/src:ro' --workdir /src --env GOCACHE=/tmp/go-cache --env GOPATH=/tmp/go-path --env CGO_ENABLED=0 '$(GO_IMAGE)' sh -ec 'go vet ./...; go test ./...'

build: test
	docker run --rm --network none --user "$$(id -u):$$(id -g)" --volume '$(CURDIR):/repo' --workdir /repo --env HOME=/tmp --env GOCACHE=/tmp/go-cache --env GOPATH=/tmp/go-path --env CGO_ENABLED=0 --env 'LAUNCHER_VERSION=$(VERSION)' --env 'LAUNCHER_IMAGE=$(IMAGE)' '$(GO_IMAGE)' sh scripts/build.sh

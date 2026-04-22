.PHONY: build build-all test lint clean release-assets

LDFLAGS ?= -s -w -X github.com/joppe2001/stackwright/cmd.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags="$(LDFLAGS)" -o dist/stackwright-$(shell go env GOOS)-$(shell go env GOARCH) .

build-all:
	GOOS=darwin  GOARCH=arm64  go build -ldflags="$(LDFLAGS)" -o dist/stackwright-darwin-arm64 .
	GOOS=darwin  GOARCH=amd64  go build -ldflags="$(LDFLAGS)" -o dist/stackwright-darwin-amd64 .
	GOOS=linux   GOARCH=amd64  go build -ldflags="$(LDFLAGS)" -o dist/stackwright-linux-amd64 .
	GOOS=linux   GOARCH=arm64  go build -ldflags="$(LDFLAGS)" -o dist/stackwright-linux-arm64 .
	GOOS=windows GOARCH=amd64  go build -ldflags="$(LDFLAGS)" -o dist/stackwright-windows-amd64.exe .

# Package each platform binary as a tarball named like the postinstall expects:
#   stackwright-<platform>.tar.gz  containing  stackwright[.exe]
# The npm postinstall downloads these from the GitHub release.
release-assets: build-all
	cd dist && \
	for tag in darwin-arm64 darwin-amd64 linux-amd64 linux-arm64; do \
		cp stackwright-$$tag stackwright && tar -czf stackwright-$$tag.tar.gz stackwright && rm stackwright; \
	done && \
	cp stackwright-windows-amd64.exe stackwright.exe && \
	tar -czf stackwright-windows-amd64.tar.gz stackwright.exe && \
	rm stackwright.exe

test:
	go test ./... -race

lint:
	golangci-lint run ./...

clean:
	rm -rf dist .gocache .gomodcache .gopath

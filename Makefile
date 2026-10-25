# Build and publish submarine. Releases are built locally and uploaded as
# GitHub release assets (see docs/architecture.md#distribution).
#
#   make dist                cross-compile into dist/
#   make release NOTES=FILE  test, build and publish the tagged HEAD (gh)

# Settings from .env (see .env.template); `make VAR=...` on the command line wins.
-include .env
export GH_TOKEN

GITHUB_REPO ?= simonfranken/submarine
RELEASES    := https://github.com/$(GITHUB_REPO)/releases

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
PLATFORMS := linux_amd64 linux_arm64 darwin_amd64 darwin_arm64
BINARIES  := $(addprefix dist/submarine_,$(PLATFORMS))

.PHONY: build test vet dist check-tag release clean FORCE

build:
	go build -ldflags '$(LDFLAGS)' -o submarine ./cmd/submarine

test:
	go test ./...

vet:
	go vet ./...
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files need formatting"; exit 1; }

dist: $(BINARIES) dist/checksums.txt

# FORCE: always rebuild, since the version is baked in.
dist/submarine_%: FORCE
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=$(word 1,$(subst _, ,$*)) GOARCH=$(word 2,$(subst _, ,$*)) \
		go build -trimpath -ldflags '$(LDFLAGS)' -o $@ ./cmd/submarine

# Rebuilt every time so it always matches the binaries.
dist/checksums.txt: $(BINARIES)
	cd dist && sha256sum $(notdir $(BINARIES)) > checksums.txt

check-tag:
	@echo '$(VERSION)' | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || \
		{ echo "VERSION '$(VERSION)' is not a clean tag: tag HEAD as vX.Y.Z with a clean tree"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "working tree is not clean"; exit 1; }

# A draft gets all assets first and is published last, so releases/latest
# never points at a release with missing files.
release: check-tag
	@test -n "$(NOTES)" && test -f "$(NOTES)" || { echo "NOTES=FILE with the release notes is required"; exit 1; }
	@command -v gh >/dev/null || { echo "gh (GitHub CLI) is required"; exit 1; }
	@gh auth status >/dev/null 2>&1 || { echo "gh is not logged in: run gh auth login or set GH_TOKEN"; exit 1; }
	$(MAKE) clean test vet dist VERSION=$(VERSION)
	printf '%s\n' '$(VERSION)' > dist/VERSION
	gh release create '$(VERSION)' --repo '$(GITHUB_REPO)' --verify-tag --draft \
		--title '$(VERSION)' --notes-file '$(NOTES)' $(BINARIES) dist/checksums.txt dist/VERSION
	gh release edit '$(VERSION)' --repo '$(GITHUB_REPO)' --draft=false --latest
	@echo "released $(VERSION): $(RELEASES)/tag/$(VERSION)"

clean:
	rm -rf dist submarine

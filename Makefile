# Build and publish submarine. Releases are built locally and uploaded as
# GitHub release assets (see docs/architecture.md#distribution).
#
#   make dist      cross-compile into dist/
#   make release   test, build and publish the tagged HEAD (GH_TOKEN from .env)

# Settings from .env (see .env.template); `make VAR=...` on the command line wins.
-include .env
export GH_TOKEN

GITHUB_REPO ?= simonfranken/submarine
RELEASES    := https://github.com/$(GITHUB_REPO)/releases
GITHUB_API     ?= https://api.github.com/repos/$(GITHUB_REPO)
GITHUB_UPLOADS ?= https://uploads.github.com/repos/$(GITHUB_REPO)
GITHUB_AUTH    := Authorization: Bearer $$GH_TOKEN

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
# never points at a release with missing files. The release has no notes yet.
release: check-tag
	@test -n "$$GH_TOKEN" || { echo "GH_TOKEN is not set: add it to .env (see .env.template)"; exit 1; }
	$(MAKE) clean test vet dist VERSION=$(VERSION)
	printf '%s\n' '$(VERSION)' > dist/VERSION
	@set -e; \
	curl -fsS -H "$(GITHUB_AUTH)" -o /dev/null "$(GITHUB_API)/git/ref/tags/$(VERSION)" || \
		{ echo "tag $(VERSION) is not on GitHub: push it first"; exit 1; }; \
	echo "create draft $(VERSION)"; \
	id=$$(curl -fsS -D - -o /dev/null -H "$(GITHUB_AUTH)" \
		-d '{"tag_name":"$(VERSION)","name":"$(VERSION)","draft":true}' \
		"$(GITHUB_API)/releases" | tr -d '\r' | sed -n 's|^[Ll]ocation: .*/||p'); \
	test -n "$$id" || { echo "could not create the draft"; exit 1; }; \
	for f in $(BINARIES) dist/checksums.txt dist/VERSION; do \
		echo "upload $$(basename $$f)"; \
		curl -fsS -H "$(GITHUB_AUTH)" -H 'Content-Type: application/octet-stream' \
			--data-binary @$$f "$(GITHUB_UPLOADS)/releases/$$id/assets?name=$$(basename $$f)" >/dev/null; \
	done; \
	echo "publish $(VERSION)"; \
	curl -fsS -X PATCH -H "$(GITHUB_AUTH)" -d '{"draft":false,"make_latest":"true"}' \
		"$(GITHUB_API)/releases/$$id" >/dev/null; \
	echo "released $(VERSION): $(RELEASES)/tag/$(VERSION)"

clean:
	rm -rf dist submarine

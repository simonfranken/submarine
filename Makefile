# Build and publish submarine. Releases are built locally and uploaded to the
# Gitea generic package registry (see docs/architecture.md#distribution).
#
#   make dist                      cross-compile into dist/
#   GITEA_TOKEN=... make release   test, build and publish the tagged HEAD

# Settings from .env (see .env.template); `make VAR=...` on the command line wins.
-include .env
export GITEA_TOKEN

GITEA_URL   ?= https://git.simon-franken.de
GITEA_OWNER ?= simonfranken
REGISTRY    := $(GITEA_URL)/api/packages/$(GITEA_OWNER)/generic

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
PLATFORMS := linux_amd64 linux_arm64 darwin_amd64 darwin_arm64
BINARIES  := $(addprefix dist/submarine_,$(PLATFORMS))

.PHONY: build test vet dist release clean FORCE

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

release:
	@test -n "$$GITEA_TOKEN" || { echo "GITEA_TOKEN is not set (needs write:package)"; exit 1; }
	@echo '$(VERSION)' | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || \
		{ echo "VERSION '$(VERSION)' is not a clean tag: tag HEAD as vX.Y.Z with a clean tree"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "working tree is not clean"; exit 1; }
	$(MAKE) clean test vet dist VERSION=$(VERSION)
	@set -e; \
	for f in $(BINARIES) dist/checksums.txt; do \
		echo "upload $$(basename $$f)"; \
		curl -fsS -X PUT -H "Authorization: token $$GITEA_TOKEN" \
			--upload-file $$f "$(REGISTRY)/submarine/$(VERSION)/$$(basename $$f)" >/dev/null; \
	done; \
	echo "point latest at $(VERSION)"; \
	printf '%s\n' '$(VERSION)' > dist/VERSION; \
	curl -sS -X DELETE -H "Authorization: token $$GITEA_TOKEN" \
		"$(GITEA_URL)/api/v1/packages/$(GITEA_OWNER)/generic/submarine-latest/latest" >/dev/null || true; \
	curl -fsS -X PUT -H "Authorization: token $$GITEA_TOKEN" \
		--upload-file dist/VERSION "$(REGISTRY)/submarine-latest/latest/VERSION" >/dev/null; \
	echo "released $(VERSION)"

clean:
	rm -rf dist submarine

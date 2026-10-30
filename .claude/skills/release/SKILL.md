---
name: release
description: Prepare and publish a submarine release. Finds the latest release, picks the next semver from the changes since, runs a release check, creates and pushes the git tag, hands over to the developer for `make release`, verifies the GitHub release, then writes its release notes. Use only when the user asks to release.
disable-model-invocation: true
---

# Release submarine

A release is built on the developer's machine (`make release`, see
`docs/architecture.md#distribution`). `make release` creates the GitHub
release of the **clean `vX.Y.Z` tag at HEAD**, with the binaries as assets
and without notes.
This skill does everything around it. The developer runs `make release`
themselves, in the middle: you never run it and never read or print `.env`
or `GH_TOKEN`.

Pushing a tag and creating a release are outward-facing and hard to undo.
Do them only after the confirmation in step 5, and never force-push, move or
delete a tag without asking.

GitHub: owner `simonfranken`, repo `submarine`, remote `origin`.

## 1. Preconditions

Stop and tell the user, if any of these fails:

```sh
git fetch origin --tags
git branch --show-current                  # main
git status --porcelain                     # empty: make release refuses a dirty tree
git rev-parse HEAD origin/main             # equal: HEAD is pushed, nothing to release from a local-only commit
git tag --points-at HEAD                   # empty: HEAD is not released yet
grep -q '^GH_TOKEN=.\+' .env               # make release needs it: only test, never print
```

## 2. Latest release

```sh
git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -1
```

Cross-check with the `mcp__github__get_latest_release` MCP tool (load it with
ToolSearch first). If the tag and the GitHub release differ (a tag without a
release is normal for the one in progress, a release without a tag is not),
tell the user and ask before going on. With no tag at all, the first release
is `v0.1.0`.

## 3. Changes since

```sh
git log --format='%h %s%n%b---' <last>..HEAD
git diff --stat <last>..HEAD
```

No commits since the last tag: stop, there is nothing to release. Read the
commits, and where a subject is not clear, the diff. Sort each change as:

- **breaking**: removes or changes a key, a command or flag, the on-disk or
  release contract (`install.sh`, asset names, `latest/download/VERSION`,
  `submarine update`), or behavior users rely on;
- **feature**: a new key, action, view or option, or a visible behavior;
- **fix / other**: bug fixes, docs, refactoring, tests, performance.

## 4. Next version

| Changes include | Version `0.y.z` (current) | Version `≥ 1.0.0` |
|---|---|---|
| breaking | minor (`0.y+1.0`) | major |
| feature | minor (`0.y+1.0`) | minor |
| only fix / other | patch (`0.y.z+1`) | patch |

submarine is below 1.0: the first release at `1.0.0` is a decision for the
user, never made by this skill. Prepare a one-line reason per change and the
proposed version.

## 5. Release check (before anything is tagged)

All must pass; stop at the first failure and report its output. Nothing has
been pushed yet, so there is nothing to undo.

```sh
go vet ./...
go test ./...
test -z "$(gofmt -l .)"
make dist VERSION=vX.Y.Z                   # all four platforms cross-compile
./dist/submarine_linux_amd64 version       # prints "submarine vX.Y.Z" (run the one for this machine)
make clean
```

Also check by reading: `README.md` and `docs/` mention the new behavior of
the features in the changelog (AGENTS.md: keep docs in sync). A missing doc
is a finding to report, not a reason to edit during a release; ask the user.

`make dist` and `make clean` only touch the git-ignored `dist/`.

## 6. Confirm, then tag and push

Ask the user (AskUserQuestion) to confirm the version, with the proposal
first and marked `(Recommended)`, the other bumps as alternatives. Show: the
last version, the changes with their category, the check result, and that
the next step creates and pushes the tag. Only after the answer:

```sh
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
git ls-remote --tags origin vX.Y.Z         # the tag is on the remote
```

If the push fails, report git's message. Do not retry with force. If the tag
is wrong after it was pushed, ask before deleting it.

## 7. Hand over: the developer runs `make release`

HEAD is now the clean tag, which is what `make release` needs. Tell the
developer to run, in the repository:

```sh
make release
```

(it reads `GH_TOKEN` from `.env`, tests, builds, creates the release as a
draft with all assets and publishes it last). **Stop here and wait** until
they say it finished. If it failed, help with the output; do not run it
yourself. A leftover draft from a failed run must be deleted (ask first)
before a retry.

## 8. Verify the release

Once the developer reports success:

```sh
curl -fsSL https://github.com/simonfranken/submarine/releases/latest/download/VERSION
```

must print `vX.Y.Z` (this is what `install.sh` and `submarine update`
read). Check with `mcp__github__get_release_by_tag` that it is not a draft
(it doesn't list assets), and that every asset downloads:

```sh
for a in submarine_linux_amd64 submarine_linux_arm64 submarine_darwin_amd64 \
	submarine_darwin_arm64 checksums.txt VERSION; do
	curl -fsSL -o /dev/null -w "$a %{http_code}\n" \
		https://github.com/simonfranken/submarine/releases/download/vX.Y.Z/$a
done
```

All must print `200`. If anything is missing, report it and write no notes.

## 9. Write the release notes

Release notes are for users of the tool, not a commit dump: short, grouped
under **Breaking changes** (only if any), **New**, **Fixes** (leave out empty
groups and pure refactoring/tests), each item one line naming the key or
behavior (`b` switches the branch of a repository...). End with:

```
Install or update: `curl -fsSL https://raw.githubusercontent.com/simonfranken/submarine/main/install.sh | sh`, or `submarine update`.

Full changelog: https://github.com/simonfranken/submarine/compare/<last>...vX.Y.Z
```

Set them as the body of release `vX.Y.Z` with the GitHub MCP server (load
its release tools with ToolSearch first).

Finish with the link to the release and the version.

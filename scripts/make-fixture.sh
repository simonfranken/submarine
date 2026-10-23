#!/usr/bin/env bash
# Builds a superproject with submodules in every state submarine shows.
# Usage: scripts/make-fixture.sh <dir>   (dir must not exist)
# The superproject is <dir>/super; bare "remotes" live in <dir>/remotes.
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: $0 <dir>" >&2; exit 2; }
[ ! -e "$1" ] || { echo "$1 already exists" >&2; exit 2; }
mkdir -p "$1"
root=$(cd "$1" && pwd)

# Reproducible and isolated from the user's git config.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_CONFIG_COUNT=2
export GIT_CONFIG_KEY_0=protocol.file.allow GIT_CONFIG_VALUE_0=always
export GIT_CONFIG_KEY_1=init.defaultBranch GIT_CONFIG_VALUE_1=main
export GIT_AUTHOR_NAME="Ada Lovelace" GIT_AUTHOR_EMAIL=ada@example.com
export GIT_COMMITTER_NAME="Ada Lovelace" GIT_COMMITTER_EMAIL=ada@example.com

g() { git -C "$@" >/dev/null 2>&1; }

# commit <repo> <file> <message>
commit() {
	echo "$3" >>"$1/$2"
	g "$1" add "$2"
	g "$1" commit -m "$3"
}

# remote <name>: bare repo with two commits on main, pushed from a work clone
remote() {
	git init -q --bare "$root/remotes/$1.git"
	git clone -q "$root/remotes/$1.git" "$root/work/$1" 2>/dev/null
	commit "$root/work/$1" README "Initial $1"
	commit "$root/work/$1" README "Add docs to $1"
	g "$root/work/$1" push origin main
}

# upstream <name> <message>: a new commit on the remote's main
upstream() {
	commit "$root/work/$1" README "$2"
	g "$root/work/$1" push origin main
}

mkdir -p "$root/remotes" "$root/work"
for r in core ui net store log feature nested work; do remote "$r"; done

# "feature" has a branch that main doesn't contain (a pin there is off target)
g "$root/work/feature" switch -c feat/search
commit "$root/work/feature" search "Add search"
commit "$root/work/feature" search "Tune search"
g "$root/work/feature" push origin feat/search
g "$root/work/feature" switch main

# "store" has a develop branch for the .gitmodules branch rule
g "$root/work/store" switch -c develop
commit "$root/work/store" dev "Start develop"
g "$root/work/store" push origin develop
g "$root/work/store" switch main

# "work" has several tracked files, to show every kind of change
w="$root/work/work"
seq 1 20 | sed 's/^/line /' >"$w/main.go"
seq 1 3 | sed 's/^/util /' >"$w/util.go"
seq 1 8 | sed 's/^/old /' >"$w/old.go"
seq 1 5 | sed 's/^/lib /' >"$w/lib.go"
printf '\000\001\002\003' >"$w/logo.png"
g "$w" add .
g "$w" commit -m "Add sources"
g "$w" push origin main

# "nested" becomes a submodule of "log"
g "$root/work/log" submodule add "$root/remotes/nested.git" modules/nested
g "$root/work/log" commit -m "Add nested"
g "$root/work/log" push origin main
# "archive" contains the pin of log like main (same tip): another branch with it
g "$root/work/log" push origin main:archive

super="$root/super"
git init -q "$super"
commit "$super" README "Initial superproject"
add() { g "$super" submodule add "${@:3}" "$root/remotes/$1.git" "$2"; }
add core src/core
add ui src/ui
add net src/net
add store libs/store -b develop
add log libs/log
add feature libs/feature
add core libs/conflict
add ui libs/staged
add net libs/missing
add ui vendor
add work libs/work
add feature libs/topic -b main
g "$super" submodule update --init --recursive
g "$super" commit -m "Add submodules"

# src/ui: remote moved on, fetched -> pin behind (↓2)
upstream ui "Fix layout"
upstream ui "Add theme"
g "$super/src/ui" fetch
g "$super/vendor" fetch
g "$super/libs/staged" fetch

# src/net: local commit pinned but never pushed -> unpushed
commit "$super/src/net" NOTES "Local experiment"
g "$super" add src/net
g "$super" commit -m "Pin net experiment"

# libs/feature: pinned inside feat/search only -> ref from contains (↓1)
g "$super/libs/feature" fetch
g "$super/libs/feature" checkout origin/feat/search~1
g "$super" add libs/feature
g "$super" commit -m "Pin feature search"

# libs/topic: .gitmodules says main, but the pin is the tip of feat/search
# -> off target: the pin is on a feature branch, not on the configured one
g "$super/libs/topic" fetch
g "$super/libs/topic" checkout origin/feat/search
g "$super" add libs/topic
g "$super" commit -m "Pin topic search"

# src/core: checkout moved away from the pin -> off pin
g "$super/src/core" checkout HEAD~1

# libs/log/modules/nested: uncommitted change, hidden under a collapsed parent
echo dirty >>"$super/libs/log/modules/nested/README"

# vendor: untracked file
echo scratch >"$super/vendor/scratch.txt"

# libs/work: one of every kind of change
wk="$super/libs/work"
{ seq 3 20 | sed 's/^/line /'; seq 1 6 | sed 's/^/new /'; } >"$wk/main.go"   # modified +6 -2
echo "util 4" >>"$wk/util.go"                                                 # modified +1
printf '\000\001\002\003\004' >"$wk/logo.png"                              # modified, binary
echo "package work" >"$wk/notes_new.go"; g "$wk" add notes_new.go              # added (staged)
rm "$wk/old.go"                                                               # removed
g "$wk" mv lib.go lib2.go                                                     # renamed (staged)
echo scratch >"$wk/scratch.txt"; mkdir "$wk/tmp"; echo x >"$wk/tmp/a.txt"      # untracked

# libs/staged: new pin staged in the superproject, not committed
g "$super/libs/staged" merge --ff-only origin/main
g "$super" add libs/staged

# libs/conflict: merge conflict in the worktree
g "$super/libs/conflict" switch -c left
commit "$super/libs/conflict" README "Left change"
g "$super/libs/conflict" switch main
commit "$super/libs/conflict" README "Right change"
g "$super/libs/conflict" push origin main
git -C "$super/libs/conflict" merge left >/dev/null 2>&1 || true
# keep the pin on the checkout so only the conflict shows
g "$super" add libs/conflict

# libs/missing: deinitialized -> missing
g "$super" submodule deinit -f libs/missing

echo "$super"

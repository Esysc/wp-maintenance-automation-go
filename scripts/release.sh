#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

usage() {
	cat <<'EOF'
Usage: scripts/release.sh <version> [--yes] [--no-push]

Creates a release from the current main branch. The script:
  - validates the working tree, branch, and remote sync
  - moves CHANGELOG [Unreleased] entries into [<version>] - <date>
  - updates internal/version/VERSION, OpenAPI, and package metadata
  - runs pre-commit checks and Go tests
  - commits the release, creates an annotated tag, and pushes
  - creates a GitHub release using the matching changelog section

Options:
  --yes       Do not ask for confirmation
  --no-push   Commit and tag locally, but do not push or create the release

Example:
  scripts/release.sh 0.5.3
EOF
}

fail() {
	echo "error: $*" >&2
	exit 1
}

VERSION=""
ASSUME_YES=false
PUSH=true

for arg in "$@"; do
	case "$arg" in
	-h | --help)
		usage
		exit 0
		;;
	--yes)
		ASSUME_YES=true
		;;
	--no-push)
		PUSH=false
		;;
	-*)
		fail "unknown option: $arg"
		;;
	*)
		if [[ -n "$VERSION" ]]; then
			fail "only one version may be provided"
		fi
		VERSION="$arg"
		;;
	esac
done

if [[ -z "$VERSION" ]]; then
	usage >&2
	exit 1
fi

VERSION="${VERSION#v}"
TAG="v$VERSION"
DATE="$(date +%F)"

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
	fail "invalid version '$VERSION' (expected something like 0.5.3)"
fi

command -v git >/dev/null || fail "git is required"
command -v awk >/dev/null || fail "awk is required"
command -v pre-commit >/dev/null || fail "pre-commit is required"
if [[ "$PUSH" == true ]]; then
	command -v gh >/dev/null || fail "gh is required to create the GitHub release"
	gh auth status >/dev/null 2>&1 || fail "gh is not authenticated; run 'gh auth login'"
fi

[[ -z "$(git status --porcelain)" ]] || fail "working tree is not clean"
BRANCH="$(git symbolic-ref --short HEAD 2>/dev/null || true)"
[[ "$BRANCH" == "main" ]] || fail "releases must be created from main (current branch: ${BRANCH:-detached})"

echo "Fetching origin..."
git fetch origin main --tags --quiet
[[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] ||
	fail "local main is not in sync with origin/main; pull or push first"

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
	fail "tag $TAG already exists locally"
fi
if git ls-remote --exit-code --tags origin "refs/tags/$TAG" >/dev/null 2>&1; then
	fail "tag $TAG already exists on origin"
fi

grep -q '^## \[Unreleased\]' CHANGELOG.md || fail "CHANGELOG.md has no [Unreleased] section"
if ! awk '
	/^## \[Unreleased\]/ { in_section = 1; next }
	in_section && /^## \[/ { exit }
	in_section && NF { found = 1 }
	END { exit found ? 0 : 1 }
' CHANGELOG.md; then
	fail "the [Unreleased] changelog section is empty"
fi

echo "Updating changelog and version metadata for $TAG..."
CHANGELOG_TMP="$(mktemp)"
NOTES_FILE="$(mktemp)"
cleanup() {
	rm -f "$CHANGELOG_TMP" "$NOTES_FILE"
}
trap cleanup EXIT

awk -v version="$VERSION" -v date="$DATE" '
	!done && $0 == "## [Unreleased]" {
		print "## [Unreleased]"
		print ""
		print "## [" version "] - " date
		done = 1
		next
	}
	{ print }
' CHANGELOG.md >"$CHANGELOG_TMP"
mv "$CHANGELOG_TMP" CHANGELOG.md

printf '%s\n' "$VERSION" >internal/version/VERSION

OPENAPI_TMP="$(mktemp)"
awk -v version="$VERSION" '
	!done && /^  version: / {
		print "  version: " version
		done = 1
		next
	}
	{ print }
' api/docs/openapi.yaml >"$OPENAPI_TMP"
mv "$OPENAPI_TMP" api/docs/openapi.yaml

update_json_version() {
	local file="$1"
	local max="$2"
	local tmp
	tmp="$(mktemp)"
	awk -v version="$VERSION" -v max="$max" '
		/^[[:space:]]*"version": "/ && count < max {
			sub(/"version": "[^"]*"/, "\"version\": \"" version "\"")
			count++
		}
		{ print }
	' "$file" >"$tmp"
	mv "$tmp" "$file"
}

update_json_version package.json 1
update_json_version package-lock.json 2
update_json_version web/ui/package.json 1
update_json_version web/ui/package-lock.json 2

awk -v version="$VERSION" '
	index($0, "## [" version "]") == 1 { capture = 1; next }
	capture && /^## \[/ { exit }
	capture { lines[++n] = $0 }
	END {
		start = 1
		while (start <= n && lines[start] == "") start++
		end = n
		while (end >= start && lines[end] == "") end--
		for (i = start; i <= end; i++) print lines[i]
	}
' CHANGELOG.md >"$NOTES_FILE"

[[ -s "$NOTES_FILE" ]] || fail "could not extract release notes from CHANGELOG.md"

echo "Running pre-commit checks..."
pre-commit run --all-files

echo "Running Go tests..."
go test ./internal/... ./pkg/... ./cmd/...

if [[ "$ASSUME_YES" != true ]]; then
	read -r -p "Create release $TAG from $(git rev-parse --short HEAD)? [y/N] " answer
	[[ "$answer" =~ ^[Yy]$ ]] || fail "release aborted"
fi

git add -u
git add CHANGELOG.md internal/version/VERSION api/docs/openapi.yaml \
	package.json package-lock.json web/ui/package.json web/ui/package-lock.json
git commit -m "chore: release $TAG"
git tag -a "$TAG" -m "Release $TAG"

if [[ "$PUSH" == true ]]; then
	echo "Pushing main and $TAG..."
	git push --atomic origin main "$TAG"

	echo "Creating GitHub release $TAG..."
	gh release create "$TAG" --title "$TAG" --notes-file "$NOTES_FILE"
	echo "Released $TAG."
else
	echo "Created local commit and tag $TAG (push skipped)."
fi

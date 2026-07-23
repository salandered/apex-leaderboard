#!/usr/bin/env bash
# -- Auto bump version --
# Compute the next patch version from the latest v* git tag.
# Starts at 0.1.0 if no tags. Requires full tag history (fetch-depth: 0I).
#
# Writes new_version (e.g. 0.1.0) and new_tag (e.g. v0.1.0) to $GITHUB_OUTPUT.
# If $GITHUB_OUTPUT is not set, prints them to stdout.
#
# Examples:
# 	No tags -> 0.1.0
# 	Last tag 0.3.1 -> 0.3.2

set -euo pipefail

latest="$(git tag --list 'v*' --sort=-v:refname | head -n1)"
if [ -z "$latest" ]; then
	new_version="0.1.0"
else
	v="${latest#v}"
	major="${v%%.*}"; rest="${v#*.}"; minor="${rest%%.*}"; patch="${rest#*.}"
	new_version="${major}.${minor}.$((patch + 1))"
fi
new_tag="v$new_version"

if [ -n "${GITHUB_OUTPUT:-}" ]; then
	{
		echo "new_version=$new_version"
		echo "new_tag=$new_tag"
	} >> "$GITHUB_OUTPUT"
else
	echo "new_version=$new_version"
	echo "new_tag=$new_tag"
fi

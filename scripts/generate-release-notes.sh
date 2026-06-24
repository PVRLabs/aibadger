#!/usr/bin/env bash
set -eu

NEW_TAG="${1:-}"
if [ -z "$NEW_TAG" ]; then
  echo "Usage: $0 <new-tag>" >&2
  exit 1
fi

PREV_TAG=$(git tag --sort=-version:refname | grep -v '\-dev' | awk -v t="$NEW_TAG" 'found{print;exit} $0==t{found=1}')
if [ -z "$PREV_TAG" ]; then
  PREV_TAG=$(git tag --sort=-version:refname | grep -v '\-dev' | head -1)
fi

COMMITS=$(git log "${PREV_TAG}..${NEW_TAG}" --oneline --no-decorate 2>/dev/null | grep -v 'release: v\|Bump version to' || true)
COMMIT_COUNT=$(echo "$COMMITS" | grep -c . || true)

echo "## What's Changed"
echo

if [ "$COMMIT_COUNT" -eq 0 ]; then
  echo "No changes between $PREV_TAG and $NEW_TAG."
  echo
  echo "**Full Changelog**: https://github.com/PVRLabs/aibadger/compare/${PREV_TAG}...${NEW_TAG}"
  exit 0
fi

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

OTHER="$WORK/other"

while IFS= read -r line; do
  commit_msg=$(echo "$line" | sed 's/^[0-9a-f]\{7,40\} //')
  issue=$(echo "$commit_msg" | grep -oE '#[0-9]+' | tail -1)
  desc=$(echo "$commit_msg" | sed 's/ #.*//' | sed 's/^[[:space:]]*//')

  if [ -n "$issue" ]; then
    echo "$desc" >> "$WORK/$issue"
  else
    echo "$desc" >> "$OTHER"
  fi
done <<< "$COMMITS"

for f in "$WORK"/#*; do
  [ -f "$f" ] || continue
  num=$(echo "$(basename "$f")" | sed 's/^#//')
  echo "$num $f"
done | sort -n | while IFS=' ' read -r num path; do
  issue="#$num"
  first=$(head -1 "$path")
  title=$(echo "$first" | sed -E 's/^(feat|fix|docs|chore|refactor|test|ci|perf|style|build|revert)(\([^)]+\))?:\s*//i' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  echo "**$issue — $title**"
  while IFS= read -r item; do
    echo "* $item"
  done < "$path"
  echo
done

if [ -s "$OTHER" ]; then
  echo "**Other**"
  while IFS= read -r item; do
    echo "* $item"
  done < "$OTHER"
  echo
fi

echo "**Full Changelog**: https://github.com/PVRLabs/aibadger/compare/${PREV_TAG}...${NEW_TAG}"

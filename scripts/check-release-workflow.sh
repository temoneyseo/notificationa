#!/bin/sh
set -eu

workflow=.github/workflows/release.yml

if ! grep -q "branches:" "$workflow" || ! grep -q "main" "$workflow"; then
  echo "release workflow must run on main branch pushes" >&2
  exit 1
fi

for action in \
  "actions/checkout" \
  "actions/setup-go" \
  "actions/upload-artifact" \
  "actions/download-artifact" \
  "softprops/action-gh-release"
do
  if grep -q "$action" "$workflow"; then
    echo "release workflow must not depend on Node-based action $action" >&2
    exit 1
  fi
done

if grep -q "gh run upload\|gh run download" "$workflow"; then
  echo "release workflow must not depend on GitHub Actions artifact upload/download commands" >&2
  exit 1
fi

if ! grep -q "gh release" "$workflow"; then
  echo "release workflow must publish releases with gh release" >&2
  exit 1
fi

if ! grep -q "latest" "$workflow"; then
  echo "release workflow must publish a rolling latest release for main" >&2
  exit 1
fi

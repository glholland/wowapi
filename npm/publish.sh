#!/usr/bin/env bash
# Publishes the packages built by npm/build.mjs. Platform packages go first so
# the launcher's optionalDependencies resolve the moment it is published.
# Versions already on the registry are skipped, so a failed run can be re-run.
# Pre-releases (e.g. 0.6.0-rc.1) are published under the "next" dist-tag.
#
# Auth: in GitHub Actions, npm trusted publishing (OIDC, no token); locally,
# whoever is signed in with `npm login` (used once, to create the packages).
set -euo pipefail

out="${1:-npm/dist}"
version="$(node -p "require('./$out/wowapi/package.json').version")"
tag="latest"
[[ "$version" == *-* ]] && tag="next"

publish() {
  local dir="$1" name
  name="$(node -p "require('./$dir/package.json').name")"
  if npm view "$name@$version" version >/dev/null 2>&1; then
    echo "skip   $name@$version (already published)"
    return
  fi
  echo "publish $name@$version ($tag)"
  npm publish "$dir" --access public --tag "$tag"
}

for dir in "$out"/wowapi-*/; do
  publish "${dir%/}"
done
publish "$out/wowapi"

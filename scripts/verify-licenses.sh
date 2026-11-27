#!/bin/bash
# scripts/verify-licenses.sh

set -eo pipefail

echo "Checking for incompatible licenses..."

# Require go-licenses — it is pre-installed in casjaysdev/go:latest; never install inline.
command -v go-licenses >/dev/null 2>&1 || {
    echo "ERROR: go-licenses not found — run inside casjaysdev/go:latest" >&2
    exit 69
}

# Resolve project-specific temporary paths from IDEA.md, the product source of truth.
PROJECT_NAME=$(sed -n '/^project_name:[[:space:]]*/ { s/^project_name:[[:space:]]*//; p; q; }' IDEA.md | tr -d '[:space:]')
PROJECT_ORG=$(sed -n '/^project_org:[[:space:]]*/ { s/^project_org:[[:space:]]*//; p; q; }' IDEA.md | tr -d '[:space:]')
INTERNAL_NAME=$(sed -n '/^internal_name:[[:space:]]*/ { s/^internal_name:[[:space:]]*//; p; q; }' IDEA.md | tr -d '[:space:]')
: "${PROJECT_NAME:?project_name was not resolved from IDEA.md or the repository}"
: "${PROJECT_ORG:?project_org was not resolved from IDEA.md or the repository}"
: "${INTERNAL_NAME:?internal_name was not resolved from IDEA.md or the repository}"

# Capture first — a scanner failure must abort (set -e), never read as "no match".
echo "Scanning dependencies..."
LICENSE_IGNORE_ARGS=()
LICENSE_OVERRIDE_ROWS=()
while read -r module spdx reviewer_note; do
    [ -n "$module" ] || continue
    [[ "$module" != \#* ]] || continue
    LICENSE_IGNORE_ARGS+=(--ignore "$module")
    LICENSE_OVERRIDE_ROWS+=("$module,$spdx")
done < <(awk 'NF && $1 !~ /^#/ { print $1, $2 }' .github/license-overrides.txt 2>/dev/null || true)
licenses=$(go-licenses csv ./src/... "${LICENSE_IGNORE_ARGS[@]}")
if ((${#LICENSE_OVERRIDE_ROWS[@]} > 0)); then
    licenses+=$'\n'"$(printf '%s\n' "${LICENSE_OVERRIDE_ROWS[@]}")"
fi
if echo "$licenses" | grep -iE 'GPL|AGPL|LGPL'; then
    echo "ERROR: Copyleft license detected!" >&2
    echo "Remove the dependency or find an alternative." >&2
    exit 1
fi

echo "✓ All licenses are compatible"

# Keep dependency license reports in the scoped temp directory, never the repository.
echo "Generating license report..."
mkdir -p "${TMPDIR:-/tmp}/${PROJECT_ORG}"
LICDIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECT_ORG}/${INTERNAL_NAME}-XXXXXX")
trap 'rm -rf -- "$LICDIR"' EXIT
go-licenses csv ./src/... "${LICENSE_IGNORE_ARGS[@]}" > "$LICDIR/licenses.csv"
go-licenses save ./src/... "${LICENSE_IGNORE_ARGS[@]}" --save_path="$LICDIR/third_party_licenses"

echo "✓ License report generated:"
cat "$LICDIR/licenses.csv"
echo ""
echo "Next steps:"
echo "1. Review the report above"
echo "2. Update LICENSE.md with any new dependencies"
echo "3. Commit the changes"

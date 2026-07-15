#!/usr/bin/env bash
# setup-dev.sh — install local git hooks for kbdiag development
# Run once after cloning: bash scripts/setup-dev.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOOK="$REPO_ROOT/.git/hooks/pre-commit"

cat > "$HOOK" <<'HOOK_EOF'
#!/usr/bin/env bash
# pre-commit: shellcheck lib/*.sh, then rebuild dist/kbdiag when sources changed
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"

if command -v shellcheck >/dev/null 2>&1; then
  echo "pre-commit: running shellcheck on lib/*.sh"
  shellcheck "$REPO_ROOT"/lib/*.sh
  echo "pre-commit: shellcheck passed"
else
  echo "pre-commit: shellcheck not found, skipping (install with: brew install shellcheck)" >&2
fi

# keep dist in lockstep with sources so a stale dist can never be committed.
# Note: builds from the worktree — with partially staged lib changes the dist
# would include unstaged edits; commit lib changes whole.
if git diff --cached --name-only | grep -qE '^(lib/|build\.sh)'; then
  echo "pre-commit: sources changed, rebuilding dist/kbdiag"
  bash "$REPO_ROOT/build.sh"
  git add "$REPO_ROOT/dist/kbdiag"
fi
HOOK_EOF

chmod +x "$HOOK"
echo "Installed pre-commit hook: $HOOK"

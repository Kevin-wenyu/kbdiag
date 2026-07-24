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

# warn (don't block) when core.sh gains a KB_* var that README's env var
# tables don't mention — this is how KB_DATA_DIR went undocumented for years
if git diff --cached --name-only | grep -qE '^lib/core\.sh$'; then
  undocumented=$(comm -23 \
    <(grep -oE '^KB_[A-Z_]+=' "$REPO_ROOT/lib/core.sh" | sed 's/=$//' | sort -u) \
    <(grep -oE '`KB_[A-Z_]+`' "$REPO_ROOT/README.md" | tr -d '`' | sort -u))
  if [[ -n "$undocumented" ]]; then
    echo "pre-commit: WARN — KB_* vars in core.sh missing from README.md env var tables:" >&2
    echo "$undocumented" | sed 's/^/  /' >&2
  fi
fi
HOOK_EOF

chmod +x "$HOOK"
echo "Installed pre-commit hook: $HOOK"

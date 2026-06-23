#!/usr/bin/env bash
# setup-dev.sh — install local git hooks for kbdiag development
# Run once after cloning: bash scripts/setup-dev.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOOK="$REPO_ROOT/.git/hooks/pre-commit"

cat > "$HOOK" <<'HOOK_EOF'
#!/usr/bin/env bash
# pre-commit: run shellcheck on lib/*.sh before every commit
set -euo pipefail

if ! command -v shellcheck >/dev/null 2>&1; then
  echo "pre-commit: shellcheck not found, skipping (install with: brew install shellcheck)" >&2
  exit 0
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"
echo "pre-commit: running shellcheck on lib/*.sh"
shellcheck "$REPO_ROOT"/lib/*.sh
echo "pre-commit: shellcheck passed"
HOOK_EOF

chmod +x "$HOOK"
echo "Installed pre-commit hook: $HOOK"

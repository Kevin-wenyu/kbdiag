#!/usr/bin/env bash
# deploy.sh — push kbdiag to multiple DB hosts
# Usage: bash scripts/deploy.sh [hosts-file]
# Env:   SSH_PORT=22  DB_USER=kingbase  DEST=~/kbdiag
set -euo pipefail

HOSTS_FILE="${1:-}"
SSH_PORT="${SSH_PORT:-22}"
DB_USER="${DB_USER:-kingbase}"
DEST="${DEST:-~/kbdiag}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
KBDIAG="$SCRIPT_DIR/../dist/kbdiag"

if [[ ! -f "$KBDIAG" ]]; then
  echo "Error: dist/kbdiag not found. Run 'bash build.sh' first." >&2
  exit 1
fi

if [[ -z "$HOSTS_FILE" ]]; then
  echo "Usage: bash scripts/deploy.sh <hosts-file>" >&2
  echo "  hosts-file: one user@host per line (# = comment)" >&2
  echo "  Env vars:   SSH_PORT (default 22), DB_USER (default kingbase), DEST (default ~/kbdiag)" >&2
  echo "  Example:    scripts/hosts.example" >&2
  exit 1
fi

if [[ ! -f "$HOSTS_FILE" ]]; then
  echo "Error: hosts file not found: $HOSTS_FILE" >&2
  exit 1
fi

LOCAL_VER=$(grep 'KBDIAG_VERSION=' "$KBDIAG" | head -1 | sed 's/.*="\(.*\)"/\1/')
echo "Deploying kbdiag ${LOCAL_VER} → ${DEST}"
echo ""

ok=0; fail=0
while IFS= read -r entry || [[ -n "$entry" ]]; do
  [[ -z "$entry" || "$entry" == \#* ]] && continue
  printf "  %-40s" "$entry"
  if scp -q -P "$SSH_PORT" "$KBDIAG" "${entry}:${DEST}.tmp" 2>/dev/null \
     && ssh -p "$SSH_PORT" "$entry" "mv ${DEST}.tmp ${DEST} && chmod +x ${DEST} && ${DEST} --version" 2>/dev/null; then
    ok=$(( ok + 1 ))
  else
    printf "  FAIL\n"
    fail=$(( fail + 1 ))
  fi
done < "$HOSTS_FILE"

echo ""
printf "Done: %d succeeded, %d failed\n" "$ok" "$fail"
[[ $fail -eq 0 ]]

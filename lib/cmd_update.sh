# shellcheck shell=bash
# cmd_update.sh — self-update from GitHub

cmd_update() {
  local url="https://raw.githubusercontent.com/Kevin-wenyu/kbdiag/main/dist/kbdiag"
  local self
  self=$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")

  printf "Current : kbdiag %s\n" "$KBDIAG_VERSION"
  printf "Source  : %s\n" "$url"
  printf "Target  : %s\n\n" "$self"

  if ! command -v curl >/dev/null 2>&1; then
    echo "Error: curl not found" >&2
    exit 1
  fi

  local tmp
  tmp=$(mktemp)
  # the trap fires at process exit, after this function's locals are gone —
  # ${tmp:-} keeps it safe under set -u
  trap 'rm -f "${tmp:-}"' EXIT
  if curl -fsSL "$url" -o "$tmp" 2>/dev/null; then
    local new_ver
    new_ver=$(grep 'KBDIAG_VERSION=' "$tmp" | head -1 | sed 's/.*="\(.*\)"/\1/')
    if [[ -z "$new_ver" ]]; then
      rm -f "$tmp"
      echo "Error: failed to extract version from downloaded file" >&2
      exit 1
    fi
    chmod +x "$tmp"
    mv "$tmp" "$self"
    trap - EXIT
    printf "Updated : kbdiag %s\n" "$new_ver"
  else
    rm -f "$tmp"
    echo "Error: download failed" >&2
    exit 1
  fi
}

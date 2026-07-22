# shellcheck shell=bash
# cmd_update.sh — self-update from GitHub

cmd_update() {
  local url="https://raw.githubusercontent.com/Kevin-wenyu/kbdiag/main/dist/kbdiag"
  local self
  self=$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "update"
  if [[ "$OUTPUT_FMT" != "json" ]]; then
    printf "Current : kbdiag %s\n" "$KBDIAG_VERSION"
    printf "Source  : %s\n" "$url"
    printf "Target  : %s\n\n" "$self"
  fi
  json_item "current_version" "ok" "$KBDIAG_VERSION" "$url"

  if ! command -v curl >/dev/null 2>&1; then
    [[ "$OUTPUT_FMT" != "json" ]] && echo "Error: curl not found" >&2
    json_item "update" "fail" "" "curl not found"
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
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
      [[ "$OUTPUT_FMT" != "json" ]] && echo "Error: failed to extract version from downloaded file" >&2
      json_item "update" "fail" "" "failed to extract version from downloaded file"
      [[ "$OUTPUT_FMT" == "json" ]] && json_end
      exit 1
    fi
    chmod +x "$tmp"
    if mv "$tmp" "$self" 2>/dev/null; then
      trap - EXIT
      [[ "$OUTPUT_FMT" != "json" ]] && printf "Updated : kbdiag %s\n" "$new_ver"
      json_item "update" "ok" "$new_ver" "updated"
    else
      rm -f "$tmp"
      [[ "$OUTPUT_FMT" != "json" ]] && echo "Error: failed to replace $self (permission denied?)" >&2
      json_item "update" "fail" "$new_ver" "failed to replace $self — check write permission"
      [[ "$OUTPUT_FMT" == "json" ]] && json_end
      exit 1
    fi
  else
    rm -f "$tmp"
    [[ "$OUTPUT_FMT" != "json" ]] && echo "Error: download failed" >&2
    json_item "update" "fail" "" "download failed"
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    exit 1
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
}

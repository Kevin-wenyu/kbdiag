#!/usr/bin/env bash
# Round-trip test for lib/core.sh's json_item/json_row/json_finding escaping
# (R2): feeds adversarial strings (quotes, backslashes, tabs, real newlines)
# through the actual emitters and validates the result is parseable JSON with
# python3's json module, and that the decoded value matches the original.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FAIL=0

command -v python3 >/dev/null 2>&1 || { echo "SKIP: python3 not found"; exit 0; }

# shellcheck source=/dev/null
source "$ROOT_DIR/lib/core.sh"

check() {
  local label="$1" input="$2"
  local escaped json decoded
  escaped="$(_json_escape "$input")"
  json="{\"v\":\"${escaped}\"}"
  decoded="$(python3 -c '
import json, sys
obj = json.loads(sys.stdin.read())
sys.stdout.write(obj["v"])
' <<<"$json" 2>&1)" || {
    echo "FAIL: $label — output is not valid JSON: $json"
    echo "      python3 error: $decoded"
    FAIL=1
    return
  }
  if [[ "$decoded" != "$input" ]]; then
    echo "FAIL: $label — round-trip mismatch"
    printf '      input:   %q\n' "$input"
    printf '      decoded: %q\n' "$decoded"
    FAIL=1
    return
  fi
  echo "ok: $label"
}

check "plain"            "hello world"
check "double_quote"     'value with "quotes" inside'
check "backslash"        'C:\path\to\file'
# shellcheck disable=SC1003
check "quote_then_slash" '"\'
check "tab"               "$(printf 'col1\tcol2')"
check "newline"           "$(printf 'line1\nline2\nline3')"
check "mixed"              "$(printf 'a"b\\c\td\ne')"
check "empty"              ""

exit "$FAIL"

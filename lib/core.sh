# ─── connection config ────────────────────────────────────────────────────────
KB_BIN_DIR="${KB_BIN_DIR:-/home/kingbase/cluster/install/kingbase/bin}"
KB_DATA_DIR="${KB_DATA_DIR:-/home/kingbase/cluster/install/kingbase/data}"
KB_REPMGR_CONF="${KB_REPMGR_CONF:-/home/kingbase/cluster/install/kingbase/etc/repmgr.conf}"
KB_PORT="${KB_PORT:-54321}"
KB_SUPERUSER="${KB_SUPERUSER:-system}"
KB_DB="${KB_DB:-test}"
KB_PEER_HOST="${KB_PEER_HOST:-}"

KSQL="$KB_BIN_DIR/ksql"
REPMGR="$KB_BIN_DIR/repmgr"

# ─── check thresholds ─────────────────────────────────────────────────────────
KB_WARN_CONN="${KB_WARN_CONN:-70}"
KB_FAIL_CONN="${KB_FAIL_CONN:-90}"
KB_WARN_LAG="${KB_WARN_LAG:-30}"
KB_FAIL_LAG="${KB_FAIL_LAG:-300}"
KB_LAG_SAMPLE_INTERVAL="${KB_LAG_SAMPLE_INTERVAL:-3}"
KB_WARN_TXN="${KB_WARN_TXN:-300}"
KB_FAIL_TXN="${KB_FAIL_TXN:-1800}"
KB_WARN_DEAD_TUPLES="${KB_WARN_DEAD_TUPLES:-100000}"
KB_SLOW_THRESHOLD="${KB_SLOW_THRESHOLD:-5}"
KB_WARN_HIT="${KB_WARN_HIT:-95}"   # hit ratio: WARN < 95%, FAIL < 90% (higher is better)
KB_FAIL_HIT="${KB_FAIL_HIT:-90}"
KB_WARN_CKPT="${KB_WARN_CKPT:-1}"
KB_FAIL_CKPT="${KB_FAIL_CKPT:-1}"
KB_WARN_TEMP="${KB_WARN_TEMP:-104857600}"    # 100MB in bytes
KB_FAIL_TEMP="${KB_FAIL_TEMP:-1073741824}"   # 1GB in bytes
KB_WARN_SLOT="${KB_WARN_SLOT:-104857600}"    # 100MB
KB_FAIL_SLOT="${KB_FAIL_SLOT:-1073741824}"   # 1GB
KB_WARN_BGW="${KB_WARN_BGW:-30}"
KB_FAIL_BGW="${KB_FAIL_BGW:-60}"
KB_WARN_XID="${KB_WARN_XID:-1000000000}"     # 10亿
KB_FAIL_XID="${KB_FAIL_XID:-1900000000}"     # 19亿
KB_WARN_OLDEST_TXN="${KB_WARN_OLDEST_TXN:-3600}"   # 1h in seconds
KB_FAIL_OLDEST_TXN="${KB_FAIL_OLDEST_TXN:-14400}"  # 4h in seconds
KB_WARN_LICENSE_DAYS="${KB_WARN_LICENSE_DAYS:-30}"

# ─── global flags (set by parse_global_args) ──────────────────────────────────
VERBOSE="${VERBOSE:-}"
QUIET="${QUIET:-}"
TOP_N="${TOP_N:-10}"
OUTPUT_FMT="${OUTPUT_FMT:-text}"
NO_COLOR="${NO_COLOR:-}"
KB_QUERY_TIMEOUT="${KB_QUERY_TIMEOUT:-10}"

# ─── color ───────────────────────────────────────────────────────────────────
_init_colors() {
  if [[ -t 1 && -z "$NO_COLOR" ]]; then
    RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
    CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
  else
    RED=''; GREEN=''; YELLOW=''; CYAN=''; BOLD=''; RESET=''
  fi
}
_init_colors

ok()   { [[ -n "$QUIET" || "$OUTPUT_FMT" == "json" ]] && return; echo -e "${GREEN}[OK]${RESET}    $*"; }
warn() {
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    echo -e "${YELLOW}[WARN]${RESET}  $*" >&2
    return
  fi
  echo -e "${YELLOW}[WARN]${RESET}  $*"
}
fail() {
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    echo -e "${RED}[FAIL]${RESET}  $*" >&2
    return
  fi
  echo -e "${RED}[FAIL]${RESET}  $*"
}
info() { [[ -n "$QUIET" || "$OUTPUT_FMT" == "json" ]] && return; echo -e "${CYAN}[INFO]${RESET}  $*"; }
hdr()  { [[ -n "$QUIET" || "$OUTPUT_FMT" == "json" ]] && return; echo -e "\n${BOLD}==> $*${RESET}"; }

# ─── global arg parser ────────────────────────────────────────────────────────
parse_global_args() {
  ARGS=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -v|--verbose)   VERBOSE=1 ;;
      -q|--quiet)     QUIET=1 ;;
      --no-color)     NO_COLOR=1; _init_colors ;;
      -n|--top)       TOP_N="$2"; shift ;;
      --top=*)        TOP_N="${1#--top=}" ;;
      --format)       OUTPUT_FMT="$2"; shift ;;
      --format=*)     OUTPUT_FMT="${1#--format=}" ;;
      --timeout)      KB_QUERY_TIMEOUT="$2"; shift ;;
      --timeout=*)    KB_QUERY_TIMEOUT="${1#--timeout=}" ;;
      *)              ARGS+=("$1") ;;
    esac
    shift
  done
}

# ─── JSON output helpers ──────────────────────────────────────────────────────
_JSON_CMD=""
_JSON_ITEMS=""
_JSON_ROWS=""

json_begin() {
  _JSON_CMD="${1:-}"
  _JSON_ITEMS=""
  _JSON_ROWS=""
}

json_item() {
  # json_item <name> <status> <value> [detail]
  local name="$1" status="$2" value="$3" detail="${4:-}"
  local entry="{\"name\":\"$name\",\"status\":\"$status\",\"value\":\"$(echo "$value" | sed 's/"/\\"/g')\",\"detail\":\"$(echo "$detail" | sed 's/"/\\"/g')\"}"
  _JSON_ITEMS="${_JSON_ITEMS:+${_JSON_ITEMS},}${entry}"
}

json_row() {
  # json_row key1=val1 key2=val2 ...
  local obj="{" first=1
  for pair in "$@"; do
    local k="${pair%%=*}" v="${pair#*=}"
    [[ $first -eq 0 ]] && obj="${obj},"
    obj="${obj}\"$k\":\"$(echo "$v" | sed 's/"/\\"/g')\""
    first=0
  done
  obj="${obj}}"
  _JSON_ROWS="${_JSON_ROWS:+${_JSON_ROWS},}${obj}"
}

json_end() {
  local ts; ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  echo "{\"command\":\"${_JSON_CMD}\",\"timestamp\":\"${ts}\",\"items\":[${_JSON_ITEMS}],\"rows\":[${_JSON_ROWS}]}"
  _JSON_ITEMS=""; _JSON_ROWS=""
}

# ─── diagnose findings infrastructure ────────────────────────────────────────
FINDINGS_LEVEL=()
FINDINGS_CATEGORY=()
FINDINGS_BODY=()

_finding() {
  FINDINGS_LEVEL+=("$1")
  FINDINGS_CATEGORY+=("$2")
  FINDINGS_BODY+=("$3")
}

_JSON_FINDINGS=""

json_finding() {
  local level="$1" category="$2" body="$3"
  local escaped
  # Escape for JSON string: backslash first, then double-quote, then literal \n -> \\n
  escaped=$(printf '%s' "$body" \
    | sed 's/\\/\\\\/g; s/"/\\"/g; s/	/\\t/g' \
    | awk 'NR>1{printf "\\n"} {printf "%s", $0}')
  local entry="{\"level\":\"$level\",\"category\":\"$category\",\"body\":\"$escaped\"}"
  _JSON_FINDINGS="${_JSON_FINDINGS:+${_JSON_FINDINGS},}${entry}"
}

json_diagnose_end() {
  local mode="${1:-fast}"
  local ts; ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  local critical=0 warn_ct=0 info_ct=0
  local i
  for (( i=0; i<${#FINDINGS_LEVEL[@]}; i++ )); do
    case "${FINDINGS_LEVEL[$i]}" in
      CRITICAL) critical=$(( critical + 1 )) ;;
      WARN)     warn_ct=$(( warn_ct + 1 )) ;;
      INFO)     info_ct=$(( info_ct + 1 )) ;;
    esac
  done
  echo "{\"command\":\"diagnose\",\"mode\":\"$mode\",\"timestamp\":\"$ts\",\"summary\":{\"critical\":$critical,\"warn\":$warn_ct,\"info\":$info_ct},\"findings\":[${_JSON_FINDINGS}]}"
  _JSON_FINDINGS=""
}

# ─── helpers ─────────────────────────────────────────────────────────────────
require_kingbase_user() {
  if [[ "$(whoami)" != "kingbase" ]]; then
    echo "Error: run as kingbase user  (sudo -i -u kingbase)" >&2
    exit 1
  fi
}

ksql_q() {
  timeout "$KB_QUERY_TIMEOUT" "$KSQL" -p "$KB_PORT" -U "$KB_SUPERUSER" "$KB_DB" \
    -AXtc "$1" 2>/dev/null
}

ksql_qh() {
  timeout "$KB_QUERY_TIMEOUT" "$KSQL" -p "$KB_PORT" -U "$KB_SUPERUSER" "$KB_DB" \
    -AXc "$1" 2>/dev/null
}

repmgr_available() {
  [[ -x "$REPMGR" && -f "$KB_REPMGR_CONF" ]]
}

# ─── config ──────────────────────────────────────────────────────────────────
KB_BIN_DIR="${KB_BIN_DIR:-/home/kingbase/cluster/install/kingbase/bin}"
KB_DATA_DIR="${KB_DATA_DIR:-/home/kingbase/cluster/install/kingbase/data}"
KB_REPMGR_CONF="${KB_REPMGR_CONF:-/home/kingbase/cluster/install/kingbase/etc/repmgr.conf}"
KB_PORT="${KB_PORT:-54321}"
KB_SUPERUSER="${KB_SUPERUSER:-system}"
KB_DB="${KB_DB:-test}"

KSQL="$KB_BIN_DIR/ksql"
REPMGR="$KB_BIN_DIR/repmgr"

# 告警阈值（可通过环境变量覆盖）
KB_WARN_CONN="${KB_WARN_CONN:-70}"
KB_FAIL_CONN="${KB_FAIL_CONN:-90}"
KB_WARN_LAG="${KB_WARN_LAG:-30}"
KB_FAIL_LAG="${KB_FAIL_LAG:-300}"
KB_WARN_TXN="${KB_WARN_TXN:-300}"
KB_FAIL_TXN="${KB_FAIL_TXN:-1800}"
KB_WARN_DEAD_TUPLES="${KB_WARN_DEAD_TUPLES:-100000}"
KB_SLOW_THRESHOLD="${KB_SLOW_THRESHOLD:-5}"

# ─── verbose ─────────────────────────────────────────────────────────────────
VERBOSE="${VERBOSE:-}"

# ─── color ───────────────────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
  CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
else
  RED=''; GREEN=''; YELLOW=''; CYAN=''; BOLD=''; RESET=''
fi

ok()   { echo -e "${GREEN}[OK]${RESET}    $*"; }
warn() { echo -e "${YELLOW}[WARN]${RESET}  $*"; }
fail() { echo -e "${RED}[FAIL]${RESET}  $*"; }
info() { echo -e "${CYAN}[INFO]${RESET}  $*"; }
hdr()  { echo -e "\n${BOLD}==> $*${RESET}"; }

# ─── helpers ─────────────────────────────────────────────────────────────────
require_kingbase_user() {
  if [[ "$(whoami)" != "kingbase" ]]; then
    echo "Error: run as kingbase user  (sudo -i -u kingbase)" >&2
    exit 1
  fi
}

ksql_q() {
  "$KSQL" -p "$KB_PORT" -U "$KB_SUPERUSER" "$KB_DB" \
    -AXtc "$1" 2>/dev/null
}

repmgr_available() {
  [[ -x "$REPMGR" && -f "$KB_REPMGR_CONF" ]]
}

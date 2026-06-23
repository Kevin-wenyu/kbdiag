# shellcheck shell=bash
cmd_watch() {
  local interval=5
  local subcmd="status"
  local args=()

  while [[ $# -gt 0 ]]; do
    case "$1" in
      status|sessions|wait|check|locks) subcmd="$1" ;;
      perf) subcmd="$1"; shift; args=("$@"); break ;;
      ''|*[!0-9]*) ;;
      *) interval="$1" ;;
    esac
    shift
  done

  while true; do
    clear 2>/dev/null || printf '\033[2J\033[H'
    echo "=== kbdiag watch — $(date) — every ${interval}s (Ctrl+C to stop) ==="
    case "$subcmd" in
      status)      cmd_status ;;
      sessions)    cmd_sessions ;;
      wait)        cmd_wait ;;
      check)       cmd_check || true ;;
      perf)        cmd_perf "${args[0]:-}" ;;
      locks)       cmd_locks "${args[0]:-}" ;;
      *)           echo "watch: unknown command '$subcmd'" >&2; return 1 ;;
    esac
    sleep "$interval"
  done
}

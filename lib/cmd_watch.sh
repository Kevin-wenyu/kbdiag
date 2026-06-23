# shellcheck shell=bash
cmd_watch() {
  local interval="${1:-5}"; shift
  local subcmd="${1:-}"; [[ $# -gt 0 ]] && shift || true
  local args=("$@")

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

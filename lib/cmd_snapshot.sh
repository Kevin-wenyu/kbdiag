# shellcheck shell=bash
# snapshot — pack volatile incident state into one tarball for later analysis
# or a vendor ticket. Collection only, no judgment. NOT a backup: nothing in
# the archive can restore data. Must keep working when the DB is unreachable —
# that is exactly when a snapshot matters most.

# Mask string literals in captured text: business data hides in quoted
# literals, and diagnosis needs structure, not data.
_snap_redact() {
  sed "s/'[^']*'/'***'/g"
}

# Capture one command's plain-text output into the snapshot dir.
# $1=filename, $2...=command. Nonzero rc is a health verdict, not a
# collection failure — the output is kept either way.
_snap_capture() {
  local name="$1"; shift
  local out
  out=$(OUTPUT_FMT="" QUIET="" "$@" 2>&1) || true
  printf '%s\n' "$out" | _snap_redact > "$_SNAP_DIR/$name"
  json_item "${name%.txt}" "ok" "$(wc -c < "$_SNAP_DIR/$name" | tr -d '[:space:]')b" ""
  info "  captured: $name"
}

cmd_snapshot() {
  local outfile="${1:-}"
  local host stamp
  host=$(hostname -s 2>/dev/null || hostname)
  stamp=$(date +%Y%m%d-%H%M%S)
  [[ -z "$outfile" ]] && outfile="kbdiag-snapshot-${host}-${stamp}.tar.gz"

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "snapshot"
  hdr "Incident snapshot"

  # captured files are plain text — no ANSI
  # shellcheck disable=SC2034  # consumed by _init_colors in core.sh
  NO_COLOR=1; _init_colors

  local base="kbdiag-snapshot-${host}-${stamp}" tmp
  tmp=$(mktemp -d) || { fail "Cannot create temp dir"; return 2; }
  _SNAP_DIR="$tmp/$base"
  mkdir -p "$_SNAP_DIR"

  # metadata works even with the DB down
  local db_version role
  db_version=$(ksql_q "SELECT version();" | head -1)
  if [[ -z "$db_version" ]]; then
    db_version="unreachable"; role="unknown"
  else
    role="primary"
    _is_true "$(ksql_q 'SELECT pg_is_in_recovery()::text;')" && role="standby"
  fi
  {
    echo "host: $host"
    echo "generated: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "kbdiag: ${KBDIAG_VERSION:-dev}"
    echo "db_version: $db_version"
    echo "role: $role"
    echo "note: string literals are masked ('***'); this snapshot is NOT a backup"
  } > "$_SNAP_DIR/metadata.txt"

  _snap_capture status.txt      cmd_status
  _snap_capture check.txt       cmd_check --os
  _snap_capture sessions.txt    cmd_sessions
  _snap_capture locks.txt       cmd_locks
  _snap_capture wait.txt        cmd_wait
  _snap_capture perf.txt        cmd_perf
  _snap_capture progress.txt    cmd_progress
  _snap_capture temp.txt        cmd_temp
  _snap_capture stat.txt        cmd_stat
  _snap_capture replication.txt cmd_replication
  _snap_capture cluster.txt     cmd_cluster ready

  # full parameter dump (raw sys_settings, not the params view filter)
  ksql_qh "SELECT name, setting, unit, source FROM sys_settings ORDER BY name;" \
    2>/dev/null | _snap_redact > "$_SNAP_DIR/params.txt" || true
  info "  captured: params.txt"

  # tail of the newest server log
  local log_file
  log_file=$(_logs_latest_file)
  if [[ -n "$log_file" && -r "$log_file" ]]; then
    { echo "# source: $log_file (last ${KB_SNAP_LOG_LINES:-2000} lines, literals masked)";
      tail -n "${KB_SNAP_LOG_LINES:-2000}" "$log_file" | _snap_redact; } > "$_SNAP_DIR/log_tail.txt"
    info "  captured: log_tail.txt"
  else
    echo "no readable log file found" > "$_SNAP_DIR/log_tail.txt"
    info "  log file not found — placeholder written"
  fi

  if ! tar -czf "$outfile" -C "$tmp" "$base" 2>/dev/null; then
    rm -rf "$tmp"
    fail "Cannot write $outfile"
    json_item "snapshot_file" "fail" "$outfile" "tar failed"
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return 2
  fi
  rm -rf "$tmp"

  local size
  size=$(du -h "$outfile" 2>/dev/null | awk '{print $1}')
  ok "Snapshot: $outfile (${size:-?}) — incident state only, NOT a backup"
  json_item "snapshot_file" "ok" "$outfile" "size ${size:-?}"
  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  return 0
}

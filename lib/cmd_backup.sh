# shellcheck shell=bash
# cmd_backup.sh — backup & WAL archiving readiness (灾备可信度)
#
# Answers one question: "if this instance died right now, could we restore it?"
# Checks archive config, live archiver state, pending-WAL backlog, sys_rman
# tooling, and replication slots that could silently retain WAL.

_backup_archive_config() {
  local mode wal_level _exit=0
  mode=$(ksql_q "SELECT setting FROM sys_settings WHERE name='archive_mode';" | tr -d '[:space:]')
  wal_level=$(ksql_q "SELECT setting FROM sys_settings WHERE name='wal_level';" | tr -d '[:space:]')

  if [[ "$mode" == "off" || -z "$mode" ]]; then
    warn "archive_mode is off — no WAL archiving, PITR is not possible"
    json_item "archive_mode" "warn" "${mode:-off}" "PITR not possible"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "archive_mode=$mode"
    json_item "archive_mode" "ok" "$mode" ""
  fi
  if [[ "$wal_level" == "minimal" ]]; then
    warn "wal_level=minimal — online base backups and replication are not possible"
    json_item "wal_level" "warn" "minimal" "backups/replication not possible"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "wal_level=$wal_level"
    json_item "wal_level" "ok" "$wal_level" ""
  fi

  local arch_cmd
  arch_cmd=$(ksql_q "SELECT setting FROM sys_settings WHERE name='archive_command';")
  arch_cmd="${arch_cmd#"${arch_cmd%%[![:space:]]*}"}"
  if [[ -n "$arch_cmd" ]]; then
    info "archive_command: $arch_cmd"
    json_item "archive_command" "ok" "$arch_cmd" ""
  fi
  return "$_exit"
}

# Shared with cmd_check's WAL-archiving item and cmd_diagnose's _diag_archiver.
# Echoes "verdict|archived_count|failed_count|last_ok|last_fail|fail_wal";
# returns nonzero if sys_stat_archiver is unreadable. Verdict computed in SQL
# so the timestamp comparison happens server-side.
_backup_archiver_row() {
  ksql_q "
    SELECT CASE
             WHEN failed_count > 0 AND (last_archived_time IS NULL
                                        OR last_failed_time > last_archived_time)
               THEN 'failing'
             WHEN failed_count > 0 THEN 'recovered'
             WHEN archived_count > 0 THEN 'ok'
             ELSE 'idle'
           END
           || '|' || archived_count
           || '|' || failed_count
           || '|' || coalesce(last_archived_time::text, 'never')
           || '|' || coalesce(last_failed_time::text, 'never')
           || '|' || coalesce(last_failed_wal, '-')
    FROM sys_stat_archiver;" 2>/dev/null
}

_backup_archiver_stats() {
  local mode="$1"
  if [[ "$mode" == "off" || -z "$mode" ]]; then
    return 0
  fi

  local row
  row=$(_backup_archiver_row) || return 0

  local verdict archived failed last_ok last_fail fail_wal
  IFS='|' read -r verdict archived failed last_ok last_fail fail_wal <<< "$row"
  verdict="${verdict// /}"

  case "$verdict" in
    failing)
      fail "Archiver is FAILING — $failed failure(s), last: $fail_wal at $last_fail (last success: $last_ok)"
      info "Verify: run the archive_command by hand as the kingbase user; check config/permissions/disk on the archive target"
      json_item "archiver" "fail" "$failed" "last: $fail_wal at $last_fail, last success: $last_ok"
      return 2
      ;;
    recovered)
      warn "Archiver recovered after $failed historical failure(s) — last success: $last_ok"
      json_item "archiver" "warn" "recovered" "$failed historical failures, last success: $last_ok"
      return 1
      ;;
    ok)
      ok "Archiver healthy — $archived WAL(s) archived, last: $last_ok"
      json_item "archiver" "ok" "$archived" "last: $last_ok"
      ;;
    idle)
      info "Archiver has not archived anything yet (no traffic since enable?)"
      json_item "archiver" "ok" "idle" "no traffic since enable"
      ;;
  esac
  return 0
}

# Count .ready files awaiting archive. Prints the count; prints nothing when
# archiving is off or the status dir is missing. Shared by backup and
# cluster ready — thresholds stay KB_WARN_WAL_READY / KB_FAIL_WAL_READY.
_pending_wal_count() {
  local data_dir wal_dir
  data_dir=$(ksql_q "SELECT setting FROM sys_settings WHERE name='data_directory';" | tr -d '[:space:]')
  [[ -z "$data_dir" ]] && return 0
  # KingbaseES uses sys_wal; fall back to pg_wal for compatibility
  wal_dir="$data_dir/sys_wal"
  [[ -d "$wal_dir" ]] || wal_dir="$data_dir/pg_wal"
  [[ -d "$wal_dir/archive_status" ]] || return 0
  find "$wal_dir/archive_status" -name '*.ready' 2>/dev/null | wc -l | tr -d '[:space:]'
}

_backup_pending_wal() {
  local mode="$1"
  if [[ "$mode" == "off" || -z "$mode" ]]; then
    return 0
  fi

  local ready_cnt
  ready_cnt=$(_pending_wal_count)
  [[ -z "$ready_cnt" ]] && return 0
  local warn_thr="${KB_WARN_WAL_READY:-10}" fail_thr="${KB_FAIL_WAL_READY:-100}"
  if (( ready_cnt >= fail_thr )); then
    fail "$ready_cnt WAL segment(s) pending archive (.ready) — archiving is stuck, disk will eventually fill"
    json_item "pending_wal" "fail" "$ready_cnt" ">= FAIL threshold $fail_thr"
    return 2
  elif (( ready_cnt >= warn_thr )); then
    warn "$ready_cnt WAL segment(s) pending archive (.ready) — archiver is falling behind"
    json_item "pending_wal" "warn" "$ready_cnt" ">= WARN threshold $warn_thr"
    return 1
  else
    ok "WAL archive backlog: $ready_cnt pending segment(s)"
    json_item "pending_wal" "ok" "$ready_cnt" ""
  fi
  return 0
}

_backup_sys_rman() {
  local arch_cmd
  arch_cmd=$(ksql_q "SELECT setting FROM sys_settings WHERE name='archive_command';")
  case "$arch_cmd" in
    *sys_rman*) ;;
    *) return 0 ;;
  esac

  local _exit=0
  info "sys_rman (KingbaseES backup tool):"
  if command -v sys_rman >/dev/null 2>&1 || [[ -x "$KB_BIN_DIR/sys_rman" ]]; then
    ok "sys_rman binary found"
    json_item "sys_rman_binary" "ok" "found" ""
  else
    fail "archive_command references sys_rman but the binary is not in PATH or \$KB_BIN_DIR"
    json_item "sys_rman_binary" "fail" "not found" "not in PATH or \$KB_BIN_DIR"
    _exit=2
  fi

  # archive_command usually embeds --config=/path/sys_rman.conf — verify it exists
  local conf_path
  conf_path=$(printf '%s' "$arch_cmd" | grep -oE -- '--config[= ][^ ]+' | head -1 | sed 's/--config[= ]//')
  if [[ -n "$conf_path" ]]; then
    if [[ -r "$conf_path" ]]; then
      ok "sys_rman config readable: $conf_path"
      json_item "sys_rman_config" "ok" "$conf_path" ""
    else
      fail "sys_rman config missing or unreadable: $conf_path — this alone makes every archive attempt fail"
      json_item "sys_rman_config" "fail" "$conf_path" "missing or unreadable"
      _exit=2
    fi
  fi
  return "$_exit"
}

_backup_slots() {
  local inactive
  inactive=$(ksql_q "SELECT count(*) FROM sys_replication_slots WHERE active=false;" 2>/dev/null | tr -d '[:space:]') || return 0
  if [[ "${inactive:-0}" -gt 0 ]]; then
    warn "$inactive inactive replication slot(s) — they retain WAL indefinitely and can fill the disk:"
    json_item "inactive_slots" "warn" "$inactive" "retain WAL indefinitely"
    local rows
    rows=$(ksql_q "SELECT slot_name, slot_type, coalesce(restart_lsn::text,'-') AS restart_lsn
             FROM sys_replication_slots WHERE active=false;")
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      while IFS='|' read -r slot_name slot_type restart_lsn; do
        [[ -z "${slot_name// /}" ]] && continue
        json_row slot_name="${slot_name// /}" slot_type="${slot_type// /}" restart_lsn="${restart_lsn// /}"
      done <<< "$rows"
    else
      { echo "slot_name|slot_type|restart_lsn"; echo "$rows"; } | column -t -s '|'
    fi
    return 1
  else
    ok "No inactive replication slots"
    json_item "inactive_slots" "ok" "0" ""
  fi
  return 0
}

cmd_backup() {
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "backup"
  hdr "Backup & Archiving"
  local mode _exit=0 rc
  mode=$(ksql_q "SELECT setting FROM sys_settings WHERE name='archive_mode';" | tr -d '[:space:]')

  _backup_archive_config;  rc=$?; _exit=$(_check_level "$_exit" "$rc")
  _backup_archiver_stats "$mode"; rc=$?; _exit=$(_check_level "$_exit" "$rc")
  _backup_pending_wal "$mode";    rc=$?; _exit=$(_check_level "$_exit" "$rc")
  _backup_sys_rman;        rc=$?; _exit=$(_check_level "$_exit" "$rc")
  _backup_slots;           rc=$?; _exit=$(_check_level "$_exit" "$rc")

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}

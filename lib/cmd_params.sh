cmd_params() {
  local pattern="${1:-}" all_flag="${2:-}"
  hdr "Instance parameters"

  # Sanitize pattern: remove single quotes to prevent SQL injection
  pattern="${pattern//\'/}"

  local where="setting <> boot_val"
  if [[ -n "$all_flag" || "$pattern" == "--all" ]]; then
    where="true"
  elif [[ -n "$pattern" && "$pattern" != "--all" ]]; then
    where="name ILIKE '%${pattern}%'"
  fi

  ksql_q "
    SELECT name, setting, unit, boot_val,
           CASE WHEN pending_restart = 'true' THEN '[RESTART NEEDED]' ELSE '' END AS note
    FROM sys_settings WHERE $where ORDER BY name LIMIT $TOP_N;" | column -t -s '|' || true
}

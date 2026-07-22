# shellcheck shell=bash
cmd_params() {
  local pattern="${1:-}" all_flag="${2:-}"
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "params"
  hdr "Instance parameters"

  # Sanitize pattern: remove single quotes to prevent SQL injection
  pattern="${pattern//\'/}"

  local where="setting <> boot_val"
  if [[ -n "$all_flag" || "$pattern" == "--all" ]]; then
    where="true"
  elif [[ -n "$pattern" && "$pattern" != "--all" ]]; then
    where="name ILIKE '%${pattern}%'"
  fi

  local rows
  rows=$(ksql_q "
    SELECT name, setting, unit, boot_val,
           CASE WHEN pending_restart = 'true' THEN '[RESTART NEEDED]' ELSE '' END AS note
    FROM sys_settings WHERE $where ORDER BY name LIMIT $TOP_N;")

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    while IFS='|' read -r name setting unit boot_val note; do
      [[ -z "${name// /}" ]] && continue
      json_row name="${name// /}" setting="$(echo "$setting" | sed 's/^ *//;s/ *$//')" \
        unit="$(echo "$unit" | sed 's/^ *//;s/ *$//')" \
        boot_val="$(echo "$boot_val" | sed 's/^ *//;s/ *$//')" \
        note="$(echo "$note" | sed 's/^ *//;s/ *$//')"
    done <<< "$rows"
  else
    { echo "name|setting|unit|boot_val|note"; echo "$rows"; } | column -t -s '|'
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  return 0
}

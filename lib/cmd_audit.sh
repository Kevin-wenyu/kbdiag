# shellcheck shell=bash
# cmd_audit.sh — security posture audit: roles, grants, connection whitelist,
# SSL, and KingbaseES-specific compliance features (sepapower/sysaudit/
# sysmac/sys_anon). Each _audit_* check returns 0 (ok/info) or 1 (warn),
# feeding cmd_audit's overall --exit-code verdict.

# Render a captured "$rows" heredoc-style result set as a table, capped at
# TOP_N unless -v is set. Shared by every list-style check below (D6: one
# query, and the same captured rows drive both the count and the display).
_audit_render_list() {
  local header="$1" rows="$2" limit="$TOP_N"
  [[ -n "$VERBOSE" ]] && limit=999999
  local cnt total
  total=$(echo "$rows" | grep -c . || true)
  { echo "$header"; echo "$rows" | head -"$limit"; } | column -t -s '|'
  cnt=$(echo "$rows" | head -"$limit" | grep -c . || true)
  if [[ -z "$VERBOSE" && "$total" -gt "$cnt" ]]; then
    info "... $((total - cnt)) more, use -v to show all"
  fi
}

_audit_superusers() {
  local rows cnt=0 display=""
  rows=$(ksql_q "SELECT rolname FROM sys_roles WHERE rolsuper=true ORDER BY rolname;" 2>/dev/null) || rows=""
  while IFS='|' read -r rolname; do
    [[ -z "${rolname// /}" ]] && continue
    cnt=$((cnt + 1))
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_row check=superuser rolname="${rolname// /}"
    else
      display="${display}${rolname}"$'\n'
    fi
  done <<< "$rows"
  ok "Superuser roles: $cnt found"
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "superusers" "ok" "$cnt" ""
  elif [[ "$cnt" -gt 0 ]]; then
    { echo "rolname"; printf '%s' "$display"; } | column -t -s '|'
    echo ""
  fi
  return 0
}

_audit_no_password() {
  local rows cnt=0 display=""
  rows=$(ksql_q "SELECT rolname FROM sys_roles WHERE rolcanlogin=true AND rolpassword IS NULL ORDER BY rolname;" 2>/dev/null) || rows=""
  while IFS='|' read -r rolname; do
    [[ -z "${rolname// /}" ]] && continue
    cnt=$((cnt + 1))
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_row check=no_password rolname="${rolname// /}"
    else
      display="${display}${rolname}"$'\n'
    fi
  done <<< "$rows"
  if [[ "$cnt" -gt 0 ]]; then
    warn "$cnt login role(s) with no password set"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "no_password" "warn" "$cnt" ""
    if [[ "$OUTPUT_FMT" != "json" ]]; then
      { echo "rolname"; printf '%s' "$display"; } | column -t -s '|'
      echo ""
    fi
    return 1
  fi
  ok "All login roles have passwords set"
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "no_password" "ok" "0" ""
  return 0
}

_audit_password_expiry() {
  local rows cnt=0 display=""
  rows=$(ksql_q "SELECT rolname FROM sys_roles WHERE rolcanlogin=true AND rolvaliduntil IS NULL ORDER BY rolname;" 2>/dev/null) || rows=""
  while IFS='|' read -r rolname; do
    [[ -z "${rolname// /}" ]] && continue
    cnt=$((cnt + 1))
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_row check=no_expiry rolname="${rolname// /}"
    else
      display="${display}${rolname}"$'\n'
    fi
  done <<< "$rows"
  if [[ "$cnt" -gt 0 ]]; then
    warn "$cnt login role(s) have no password expiry (VALID UNTIL) set"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "password_expiry" "warn" "$cnt" ""
    [[ "$OUTPUT_FMT" != "json" ]] && _audit_render_list "rolname" "$display"
    return 1
  fi
  ok "All login roles have a password expiry date"
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "password_expiry" "ok" "0" ""
  return 0
}

_audit_public_grants() {
  local rows cnt=0 display=""
  rows=$(ksql_q "
    SELECT grantee, table_name, privilege_type
    FROM information_schema.role_table_grants
    WHERE table_schema='public' AND privilege_type IN ('INSERT','UPDATE','DELETE')
      AND grantee='PUBLIC'
    ORDER BY table_name, privilege_type;" 2>/dev/null) || rows=""
  while IFS='|' read -r grantee table_name priv; do
    [[ -z "${table_name// /}" ]] && continue
    cnt=$((cnt + 1))
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_row check=public_grant grantee="${grantee// /}" table_name="${table_name// /}" privilege="${priv// /}"
    else
      display="${display}${grantee}|${table_name}|${priv}"$'\n'
    fi
  done <<< "$rows"
  if [[ "$cnt" -gt 0 ]]; then
    warn "PUBLIC has $cnt write grant(s) on public schema tables"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "public_grants" "warn" "$cnt" ""
    [[ "$OUTPUT_FMT" != "json" ]] && _audit_render_list "grantee|table_name|privilege" "$display"
    return 1
  fi
  ok "No PUBLIC write grants on public schema tables"
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "public_grants" "ok" "0" ""
  return 0
}

_audit_no_pk() {
  local rows cnt=0 display=""
  rows=$(ksql_q "
    SELECT schemaname, relname FROM sys_stat_user_tables
    WHERE relname NOT IN (
      SELECT conrelid::regclass::text FROM sys_constraint WHERE contype='p'
    ) ORDER BY schemaname, relname;" 2>/dev/null) || rows=""
  while IFS='|' read -r schemaname relname; do
    [[ -z "${relname// /}" ]] && continue
    cnt=$((cnt + 1))
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_row check=no_pk schemaname="${schemaname// /}" relname="${relname// /}"
    else
      display="${display}${schemaname}|${relname}"$'\n'
    fi
  done <<< "$rows"
  if [[ "$cnt" -gt 0 ]]; then
    warn "$cnt user table(s) without primary keys"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "no_pk" "warn" "$cnt" ""
    [[ "$OUTPUT_FMT" != "json" ]] && _audit_render_list "schemaname|relname" "$display"
    return 1
  fi
  ok "All user tables have primary keys"
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "no_pk" "ok" "0" ""
  return 0
}

# Connection-source whitelist. Reads sys_hba_file_rules (what the server
# actually loaded, including parse errors) instead of parsing sys_hba.conf —
# the file on disk may differ from the active config until a reload. Fetches
# every rule with its computed risk text in one query (D6): total = row
# count, risky = rows where risk is non-empty, no second round-trip.
_audit_hba() {
  local rows
  # Guarded: the view may be absent or restricted on older/other editions.
  if ! rows=$(ksql_q "
    SELECT line_number, coalesce(type,'?'),
           coalesce(array_to_string(database,','),'?'),
           coalesce(array_to_string(user_name,','),'?'),
           -- KingbaseES concat treats NULL as '' (Oracle mode), so a
           -- coalesce(address||'/'||netmask, ...) fallback never fires.
           CASE WHEN address IS NULL THEN 'local'
                WHEN netmask IS NULL THEN address
                ELSE address || '/' || netmask END,
           coalesce(auth_method,'?'),
           concat_ws('; ',
             CASE WHEN error IS NOT NULL THEN 'parse error: ' || error END,
             CASE WHEN auth_method='trust' AND type <> 'local'
                    THEN 'no authentication over network'
                  WHEN auth_method='trust'
                    THEN 'any local OS user connects unauthenticated' END,
             CASE WHEN (address='0.0.0.0' AND netmask='0.0.0.0')
                    OR (address='::' AND netmask='::') OR address='all'
                  THEN CASE WHEN 'replication' = ANY(database)
                         THEN 'replication open to any host'
                         ELSE 'open to any host' END END,
             CASE WHEN auth_method IN ('password','md5')
                    THEN 'weak auth method' END
           )
    FROM sys_hba_file_rules ORDER BY line_number;" 2>/dev/null); then
    info "sys_hba_file_rules view not available — inspect sys_hba.conf manually"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "hba" "warn" "0" "view not available"
    return 1
  fi

  local total=0 risky_cnt=0 display=""
  while IFS='|' read -r line type database users source auth risk; do
    [[ -z "${line// /}" ]] && continue
    total=$((total + 1))
    if [[ -n "${risk// /}" ]]; then
      risky_cnt=$((risky_cnt + 1))
      if [[ "$OUTPUT_FMT" == "json" ]]; then
        json_row check=hba_rule line="${line// /}" type="${type// /}" database="${database// /}" users="${users// /}" source="$source" auth="${auth// /}" risk="$risk"
      else
        display="${display}${line}|${type}|${database}|${users}|${source}|${auth}|${risk}"$'\n'
      fi
    fi
  done <<< "$rows"

  if [[ "$risky_cnt" -gt 0 ]]; then
    warn "sys_hba.conf: $risky_cnt of $total rule(s) need review"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "hba" "warn" "$risky_cnt" "of $total"
    [[ "$OUTPUT_FMT" != "json" ]] && _audit_render_list "line|type|database|users|source|auth|risk" "$display"
    return 1
  fi
  ok "sys_hba.conf: $total rule(s) loaded — no trust, wide-open or weak-auth entries"
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "hba" "ok" "$total" ""
  return 0
}

_audit_ssl() {
  local ssl_on
  ssl_on=$(ksql_q "SHOW ssl;" 2>/dev/null | tr -d '[:space:]') || ssl_on=""
  if [[ "$ssl_on" == "on" ]]; then
    ok "SSL is enabled"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "ssl" "ok" "on" ""
    return 0
  fi
  warn "SSL is disabled — client connections are unencrypted"
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "ssl" "warn" "off" "client connections are unencrypted"
  return 1
}

# KingbaseES-specific: 三权分立 (three-power separation of DBA/SSO/SAO duties),
# a compliance feature (等保 2.0 level 3+) with no Postgres equivalent.
_audit_sepapower() {
  local sepa
  # Query sys_settings (never errors) rather than SHOW (errors under `set -e`
  # if the GUC isn't registered on some other KingbaseES edition/version).
  sepa=$(ksql_q "SELECT setting FROM sys_settings WHERE name='sepapower.separate_power_grant';" 2>/dev/null | tr -d '[:space:]') || sepa=""
  if [[ -z "$sepa" ]]; then
    info "Three-power separation (sepapower) not available on this instance"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sepapower" "ok" "unavailable" ""
    return 0
  elif [[ "$sepa" == "on" ]]; then
    ok "Three-power separation (sepapower) is enabled — DBA/SSO/SAO duties are separated"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sepapower" "ok" "on" ""
    return 0
  fi
  warn "Three-power separation (sepapower) is disabled — the DBA role has unrestricted control over audit/security config"
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "sepapower" "warn" "off" ""
  return 1
}

_audit_ext_installed() {
  ksql_q "SELECT 1 FROM sys_extension WHERE extname='$1';" 2>/dev/null | tr -d '[:space:]'
}

# sysaudit locks rule/log visibility to sso/sao at the function level, independent
# of GRANT and of sepapower's on/off state — a permission-denied here is the
# expected, compliant outcome, not a broken check.
_audit_sysaudit() {
  if [[ -z "$(_audit_ext_installed sysaudit)" ]]; then
    warn "sysaudit extension is not installed"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sysaudit" "warn" "0" "not installed"
    return 1
  fi

  local cnt
  # Guarded with if/then (not a bare assignment) so `set -e` doesn't abort
  # kbdiag when this is denied — permission-denied is the expected outcome.
  if ! cnt=$(ksql_q "SELECT count(*) FROM sysaudit.show_audit_rules();" 2>/dev/null); then
    ok "Audit rule/log details are restricted to the sso/sao role (three-power separation enforced) — inspect via: ksql -U sso -c 'SELECT * FROM sysaudit.all_audit_rules;'"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sysaudit" "ok" "restricted" "sso/sao only"
    return 0
  fi

  cnt="${cnt// /}"
  if [[ "${cnt:-0}" -eq 0 ]]; then
    warn "sysaudit is installed but no audit rules are configured"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sysaudit" "warn" "0" "no rules configured"
    return 1
  fi
  ok "$cnt audit rule(s) configured"
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "sysaudit" "ok" "$cnt" ""
  return 0
}

# sysmac (label-based mandatory access control) policy tables are readable by
# the DBA role even when sysaudit/sys_anon are locked down — this is by design,
# not an inconsistency.
_audit_sysmac() {
  if [[ -z "$(_audit_ext_installed sysmac)" ]]; then
    warn "sysmac extension is not installed"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sysmac" "warn" "0" "not installed"
    return 1
  fi

  local policy_cnt enforce_cnt
  policy_cnt=$(ksql_q "SELECT count(*) FROM sysmac.sysmac_policys;" 2>/dev/null | tr -d '[:space:]') || policy_cnt=0
  enforce_cnt=$(ksql_q "SELECT count(*) FROM sysmac.sysmac_policy_enforcements;" 2>/dev/null | tr -d '[:space:]') || enforce_cnt=0
  if [[ "${policy_cnt:-0}" -eq 0 ]]; then
    ok "sysmac installed, no MAC policies defined (not in use)"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sysmac" "ok" "0" "not in use"
  else
    ok "$policy_cnt MAC polic(ies) defined, ${enforce_cnt:-0} object(s) under enforcement"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sysmac" "ok" "$policy_cnt" "${enforce_cnt:-0} enforced"
  fi
  return 0
}

# sys_anon's policy view lives in a schema without USAGE granted to the DBA
# role by default — same three-power-separation pattern as sysaudit.
_audit_sys_anon() {
  if [[ -z "$(_audit_ext_installed sys_anon)" ]]; then
    warn "sys_anon extension is not installed"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sys_anon" "warn" "0" "not installed"
    return 1
  fi

  local cnt
  if ! cnt=$(ksql_q "SELECT count(*) FROM anon.all_policy;" 2>/dev/null); then
    ok "Masking policy details are restricted (no schema access for the DBA role) — inspect via the sso/sao role, or GRANT USAGE ON SCHEMA anon"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sys_anon" "ok" "restricted" ""
    return 0
  fi

  cnt="${cnt// /}"
  if [[ "${cnt:-0}" -eq 0 ]]; then
    warn "sys_anon is installed but no masking policies are configured"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "sys_anon" "warn" "0" "no policies configured"
    return 1
  fi
  ok "$cnt masking polic(ies) configured"
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "sys_anon" "ok" "$cnt" ""
  return 0
}

cmd_audit() {
  local _exit=0 status
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "audit"
  [[ "$OUTPUT_FMT" != "json" ]] && hdr "Security Audit"

  _audit_superusers || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_no_password || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_password_expiry || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_public_grants || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_no_pk || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_hba || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_ssl || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_sepapower || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_sysaudit || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_sysmac || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status
  _audit_sys_anon || status=$?; status="${status:-0}"; [[ "$status" -gt "$_exit" ]] && _exit=$status

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}

# shellcheck shell=bash
_audit_superusers() {
  info "Superuser roles:"
  ksql_qh "SELECT rolname FROM sys_roles WHERE rolsuper=true ORDER BY rolname;" | column -t -s '|' || true
}

_audit_no_password() {
  info "Roles with login but no password:"
  local no_pwd_cnt
  no_pwd_cnt=$(ksql_q "SELECT count(*) FROM sys_roles WHERE rolcanlogin=true AND rolpassword IS NULL;" | tr -d '[:space:]')
  if [[ "${no_pwd_cnt:-0}" -gt 0 ]]; then
    warn "Roles with no password set:"
    ksql_qh "SELECT rolname FROM sys_roles WHERE rolcanlogin=true AND rolpassword IS NULL ORDER BY rolname;" | column -t -s '|' || true
  else
    ok "All login roles have passwords set"
  fi
}

_audit_password_expiry() {
  info "Password expiry policy (login roles):"
  local no_expiry_cnt
  no_expiry_cnt=$(ksql_q "SELECT count(*) FROM sys_roles WHERE rolcanlogin=true AND rolvaliduntil IS NULL;" | tr -d '[:space:]')
  if [[ "${no_expiry_cnt:-0}" -gt 0 ]]; then
    warn "$no_expiry_cnt login role(s) have no password expiry (VALID UNTIL) set:"
    ksql_qh "SELECT rolname FROM sys_roles WHERE rolcanlogin=true AND rolvaliduntil IS NULL ORDER BY rolname LIMIT $TOP_N;" | column -t -s '|' || true
  else
    ok "All login roles have a password expiry date"
  fi
}

_audit_public_grants() {
  info "PUBLIC write grants on public schema tables (top $TOP_N):"
  local pub_grants_cnt
  pub_grants_cnt=$(ksql_q "
    SELECT count(*) FROM information_schema.role_table_grants
    WHERE table_schema='public' AND privilege_type IN ('INSERT','UPDATE','DELETE')
      AND grantee='PUBLIC';" | tr -d '[:space:]')
  if [[ "${pub_grants_cnt:-0}" -gt 0 ]]; then
    warn "PUBLIC has write grants on these tables:"
    ksql_qh "
      SELECT grantee, table_name, privilege_type
      FROM information_schema.role_table_grants
      WHERE table_schema='public' AND privilege_type IN ('INSERT','UPDATE','DELETE')
        AND grantee='PUBLIC'
      LIMIT $TOP_N;" | column -t -s '|' || true
  else
    ok "No PUBLIC write grants on public schema tables"
  fi
}

_audit_no_pk() {
  info "User tables without primary keys (top $TOP_N):"
  local no_pk_cnt
  no_pk_cnt=$(ksql_q "
    SELECT count(*) FROM sys_stat_user_tables
    WHERE relname NOT IN (
      SELECT conrelid::regclass::text FROM sys_constraint WHERE contype='p'
    );" | tr -d '[:space:]')
  if [[ "${no_pk_cnt:-0}" -gt 0 ]]; then
    warn "Tables without primary keys:"
    ksql_qh "
      SELECT schemaname, relname FROM sys_stat_user_tables
      WHERE relname NOT IN (
        SELECT conrelid::regclass::text FROM sys_constraint WHERE contype='p'
      ) ORDER BY schemaname, relname LIMIT $TOP_N;" | column -t -s '|' || true
  else
    ok "All user tables have primary keys"
  fi
}

# Connection-source whitelist. Reads sys_hba_file_rules (what the server
# actually loaded, including parse errors) instead of parsing sys_hba.conf —
# the file on disk may differ from the active config until a reload.
_audit_hba() {
  info "Connection whitelist (sys_hba.conf):"
  local total
  # Guarded: the view may be absent or restricted on older/other editions.
  if ! total=$(ksql_q "SELECT count(*) FROM sys_hba_file_rules;"); then
    info "sys_hba_file_rules view not available — inspect sys_hba.conf manually"
    return
  fi
  total="${total// /}"

  local risky_where="
    error IS NOT NULL
    OR auth_method IN ('trust','password','md5')
    OR (address='0.0.0.0' AND netmask='0.0.0.0')
    OR (address='::' AND netmask='::')
    OR address='all'"

  local risky_cnt
  risky_cnt=$(ksql_q "SELECT count(*) FROM sys_hba_file_rules WHERE $risky_where;" | tr -d '[:space:]')
  if [[ "${risky_cnt:-0}" -eq 0 ]]; then
    ok "$total hba rule(s) loaded — no trust, wide-open or weak-auth entries"
    return
  fi

  warn "$risky_cnt of $total hba rule(s) need review:"
  ksql_qh "
    SELECT line_number AS line, coalesce(type,'?') AS type,
           coalesce(array_to_string(database,','),'?') AS database,
           coalesce(array_to_string(user_name,','),'?') AS users,
           -- KingbaseES concat treats NULL as '' (Oracle mode), so a
           -- coalesce(address||'/'||netmask, ...) fallback never fires.
           CASE WHEN address IS NULL THEN 'local'
                WHEN netmask IS NULL THEN address
                ELSE address || '/' || netmask END AS source,
           coalesce(auth_method,'?') AS auth,
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
           ) AS risk
    FROM sys_hba_file_rules
    WHERE $risky_where
    ORDER BY line_number LIMIT $TOP_N;" | column -t -s '|' || true
}

_audit_ssl() {
  local ssl_on
  ssl_on=$(ksql_q "SHOW ssl;" | tr -d '[:space:]')
  if [[ "$ssl_on" == "on" ]]; then
    ok "SSL is enabled"
  else
    warn "SSL is disabled — client connections are unencrypted"
  fi
}

# KingbaseES-specific: 三权分立 (three-power separation of DBA/SSO/SAO duties),
# a compliance feature (等保 2.0 level 3+) with no Postgres equivalent.
_audit_sepapower() {
  local sepa
  # Query sys_settings (never errors) rather than SHOW (errors under `set -e`
  # if the GUC isn't registered on some other KingbaseES edition/version).
  sepa=$(ksql_q "SELECT setting FROM sys_settings WHERE name='sepapower.separate_power_grant';" | tr -d '[:space:]')
  if [[ -z "$sepa" ]]; then
    info "Three-power separation (sepapower) not available on this instance"
  elif [[ "$sepa" == "on" ]]; then
    ok "Three-power separation (sepapower) is enabled — DBA/SSO/SAO duties are separated"
  else
    warn "Three-power separation (sepapower) is disabled — the DBA role has unrestricted control over audit/security config"
  fi
}

_audit_ext_installed() {
  ksql_q "SELECT 1 FROM sys_extension WHERE extname='$1';" | tr -d '[:space:]'
}

# sysaudit locks rule/log visibility to sso/sao at the function level, independent
# of GRANT and of sepapower's on/off state — a permission-denied here is the
# expected, compliant outcome, not a broken check.
_audit_sysaudit() {
  info "sysaudit (audit logging):"
  if [[ -z "$(_audit_ext_installed sysaudit)" ]]; then
    warn "sysaudit extension is not installed"
    return
  fi

  local cnt
  # Guarded with if/then (not a bare assignment) so `set -e` doesn't abort
  # kbdiag when this is denied — permission-denied is the expected outcome.
  if ! cnt=$(ksql_q "SELECT count(*) FROM sysaudit.show_audit_rules();"); then
    ok "Audit rule/log details are restricted to the sso/sao role (three-power separation enforced) — inspect via: ksql -U sso -c 'SELECT * FROM sysaudit.all_audit_rules;'"
    return
  fi

  cnt="${cnt// /}"
  if [[ "${cnt:-0}" -eq 0 ]]; then
    warn "sysaudit is installed but no audit rules are configured"
  else
    ok "$cnt audit rule(s) configured"
  fi
}

# sysmac (label-based mandatory access control) policy tables are readable by
# the DBA role even when sysaudit/sys_anon are locked down — this is by design,
# not an inconsistency.
_audit_sysmac() {
  info "sysmac (mandatory access control / label security):"
  if [[ -z "$(_audit_ext_installed sysmac)" ]]; then
    warn "sysmac extension is not installed"
    return
  fi

  local policy_cnt enforce_cnt
  policy_cnt=$(ksql_q "SELECT count(*) FROM sysmac.sysmac_policys;" | tr -d '[:space:]')
  enforce_cnt=$(ksql_q "SELECT count(*) FROM sysmac.sysmac_policy_enforcements;" | tr -d '[:space:]')
  if [[ "${policy_cnt:-0}" -eq 0 ]]; then
    ok "sysmac installed, no MAC policies defined (not in use)"
  else
    ok "$policy_cnt MAC polic(ies) defined, ${enforce_cnt:-0} object(s) under enforcement"
  fi
}

# sys_anon's policy view lives in a schema without USAGE granted to the DBA
# role by default — same three-power-separation pattern as sysaudit.
_audit_sys_anon() {
  info "sys_anon (data masking):"
  if [[ -z "$(_audit_ext_installed sys_anon)" ]]; then
    warn "sys_anon extension is not installed"
    return
  fi

  local cnt
  if ! cnt=$(ksql_q "SELECT count(*) FROM anon.all_policy;"); then
    ok "Masking policy details are restricted (no schema access for the DBA role) — inspect via the sso/sao role, or GRANT USAGE ON SCHEMA anon"
    return
  fi

  cnt="${cnt// /}"
  if [[ "${cnt:-0}" -eq 0 ]]; then
    warn "sys_anon is installed but no masking policies are configured"
  else
    ok "$cnt masking polic(ies) configured"
  fi
}

cmd_audit() {
  hdr "Security Audit"
  _audit_superusers
  _audit_no_password
  _audit_password_expiry
  _audit_public_grants
  _audit_no_pk
  _audit_hba
  _audit_ssl
  _audit_sepapower
  _audit_sysaudit
  _audit_sysmac
  _audit_sys_anon
}

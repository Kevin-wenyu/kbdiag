# shellcheck shell=bash
cmd_audit() {
  hdr "Security Audit"

  # Superusers
  info "Superuser roles:"
  ksql_qh "SELECT rolname FROM sys_roles WHERE rolsuper=true ORDER BY rolname;" | column -t -s '|' || true

  # Roles that can login without a password
  info "Roles with login but no password:"
  local no_pwd_cnt
  no_pwd_cnt=$(ksql_q "SELECT count(*) FROM sys_roles WHERE rolcanlogin=true AND rolpassword IS NULL;" | tr -d '[:space:]')
  if [[ "${no_pwd_cnt:-0}" -gt 0 ]]; then
    warn "Roles with no password set:"
    ksql_qh "SELECT rolname FROM sys_roles WHERE rolcanlogin=true AND rolpassword IS NULL ORDER BY rolname;" | column -t -s '|' || true
  else
    ok "All login roles have passwords set"
  fi

  # PUBLIC schema write grants
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

  # Tables without primary keys
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

cmd_audit() {
  hdr "Security Audit"

  # Superusers
  info "Superuser roles:"
  ksql_q "SELECT rolname FROM sys_roles WHERE rolsuper=true ORDER BY rolname;" | column -t -s '|' || true

  # Roles that can login without a password
  info "Roles with login but no password:"
  local no_pwd
  no_pwd=$(ksql_q "SELECT rolname FROM sys_roles WHERE rolcanlogin=true AND rolpassword IS NULL ORDER BY rolname;" || true)
  if [[ -n "$no_pwd" ]]; then
    warn "Roles with no password set:"
    echo "$no_pwd" | column -t -s '|'
  else
    ok "All login roles have passwords set"
  fi

  # PUBLIC schema write grants
  info "PUBLIC write grants on public schema tables (top $TOP_N):"
  local pub_grants
  pub_grants=$(ksql_q "
    SELECT grantee, table_name, privilege_type
    FROM information_schema.role_table_grants
    WHERE table_schema='public' AND privilege_type IN ('INSERT','UPDATE','DELETE')
      AND grantee='PUBLIC'
    LIMIT $TOP_N;" | column -t -s '|' || true)
  if [[ -n "$pub_grants" ]]; then
    warn "PUBLIC has write grants on these tables:"
    echo "$pub_grants"
  else
    ok "No PUBLIC write grants on public schema tables"
  fi

  # Tables without primary keys
  info "User tables without primary keys (top $TOP_N):"
  local no_pk
  no_pk=$(ksql_q "
    SELECT schemaname, relname FROM sys_stat_user_tables
    WHERE relname NOT IN (
      SELECT conrelid::regclass::text FROM sys_constraint WHERE contype='p'
    ) ORDER BY schemaname, relname LIMIT $TOP_N;" | column -t -s '|' || true)
  if [[ -n "$no_pk" ]]; then
    warn "Tables without primary keys:"
    echo "$no_pk"
  else
    ok "All user tables have primary keys"
  fi
}

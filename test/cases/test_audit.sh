# test_audit.sh — tests for "kbdiag audit" command

test_audit_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE audit")
  assert_exit_code 0 "${code:-0}"
}

test_audit_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE audit")
  assert_contains "$out" "Security Audit"
}

test_audit_shows_superuser_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE audit")
  assert_contains "$out" "Superuser"
}

test_audit_shows_no_password_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE audit")
  assert_contains "$out" "password"
}

test_audit_shows_pk_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE audit")
  assert_contains "$out" "primary key"
}

test_audit_shows_public_grants_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE audit")
  assert_contains "$out" "PUBLIC"
}

test_audit_superuser_has_column_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE audit")
  assert_contains "$out" "rolname"
}

test_audit_shows_hba_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE audit")
  assert_contains "$out" "sys_hba.conf"
}

# The test VMs ship a wide-open hba (0.0.0.0/0 + local trust), so the risky-rule
# table must appear with per-rule reasons.
test_audit_hba_flags_wide_open_rule() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE audit")
  assert_contains "$out" "open to any host"
}

test_audit_hba_flags_trust_rule() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE audit")
  assert_contains "$out" "unauthenticated"
}

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

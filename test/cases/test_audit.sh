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

# ── exit-code gating / JSON ──────────────────────────────────────────────────

test_audit_default_exit_zero_even_with_warnings() {
  # The test VM has a wide-open hba rule, so at least one check warns — the
  # default (no --exit-code) run must still exit 0.
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE audit")
  assert_exit_code 0 "${code:-0}"
}

test_audit_exit_code_flag_reflects_verdict() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --exit-code audit")
  [[ "${code:-0}" -ge 1 ]] && _pass || _fail "audit --exit-code should reflect a warn/fail verdict, got $code"
}

test_audit_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json audit")
  assert_json_valid "$out"
  assert_contains "$out" '"command":"audit"'
}

test_audit_verbose_shows_all_hba_rules() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v audit")
  assert_contains "$out" "line"
}

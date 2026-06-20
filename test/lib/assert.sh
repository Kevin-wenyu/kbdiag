# Test state
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
FAILURES=""

_pass() {
  TESTS_RUN=$(( TESTS_RUN + 1 ))
  TESTS_PASSED=$(( TESTS_PASSED + 1 ))
  echo "  [PASS] ${CURRENT_TEST:-unknown}"
}

_fail() {
  TESTS_RUN=$(( TESTS_RUN + 1 ))
  TESTS_FAILED=$(( TESTS_FAILED + 1 ))
  local msg="  [FAIL] ${CURRENT_TEST:-unknown}: $1"
  echo "$msg"
  FAILURES="${FAILURES}${msg}\n"
}

assert_contains() {
  local output="$1" pattern="$2"
  if echo "$output" | grep -qF "$pattern" 2>/dev/null; then
    _pass
  else
    _fail "expected to contain: '$pattern'\n         got: $(echo "$output" | head -5)"
  fi
}

assert_not_contains() {
  local output="$1" pattern="$2"
  if echo "$output" | grep -qF "$pattern" 2>/dev/null; then
    _fail "expected NOT to contain: '$pattern'"
  else
    _pass
  fi
}

assert_exit_code() {
  local expected="$1" actual="$2"
  if [[ "$actual" -eq "$expected" ]]; then
    _pass
  else
    _fail "expected exit code $expected, got $actual"
  fi
}

assert_json_valid() {
  local output="$1"
  if echo "$output" | python3 -m json.tool > /dev/null 2>&1; then
    _pass
  else
    _fail "invalid JSON output:\n$(echo "$output" | head -3)"
  fi
}

print_summary() {
  echo ""
  echo "Results: $TESTS_PASSED passed, $TESTS_FAILED failed (total: $TESTS_RUN)"
  if [[ $TESTS_FAILED -gt 0 ]]; then
    echo ""
    echo "Failures:"
    echo -e "$FAILURES"
    return 1
  fi
  return 0
}

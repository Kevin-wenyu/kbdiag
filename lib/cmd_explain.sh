# shellcheck shell=bash
# cmd_explain.sh — plan analysis for a queryid or a literal SQL statement.
#
# DBA tier: verifies ROOT-CAUSE conclusions (diagnose/advisor point at a
# query; explain shows why it's slow). Uses plain EXPLAIN — never executes
# the statement, so DML is safe to analyze.

_explain_stmt_stats() {
  local qid="$1" row
  row=$(ksql_q "SELECT calls::text
                  ||'|'||round(mean_exec_time::numeric,1)
                  ||'|'||round(total_exec_time::numeric,1)
                  ||'|'||shared_blks_read
                  ||'|'||temp_blks_written
                FROM sys_stat_statements WHERE queryid=$qid LIMIT 1;" 2>/dev/null) || return 0
  [[ -z "${row// /}" ]] && return 0
  local calls mean total blks temp
  IFS='|' read -r calls mean total blks temp <<< "$row"
  info "History: ${calls// /} call(s), mean ${mean// /}ms, total ${total// /}ms, ${blks// /} shared blks read, ${temp// /} temp blks written"
}

_explain_flags() {
  local plan="$1"
  local seq_cnt loop_cnt sort_cnt
  seq_cnt=$(printf '%s' "$plan" | grep -c 'Seq Scan' || true)
  loop_cnt=$(printf '%s' "$plan" | grep -c 'Nested Loop' || true)
  sort_cnt=$(printf '%s' "$plan" | grep -cE 'Sort|Materialize' || true)

  printf "\n"
  if [[ "${seq_cnt:-0}" -gt 0 ]]; then
    warn "$seq_cnt sequential scan(s) in plan — if the table is large, check: kbdiag idx missing / kbdiag colstat <table>"
  else
    ok "No sequential scans"
  fi
  if [[ "${loop_cnt:-0}" -gt 1 ]]; then
    warn "$loop_cnt nested loop(s) — misestimated row counts can explode these; verify with: kbdiag colstat <table>"
  fi
  if [[ "${sort_cnt:-0}" -gt 0 ]]; then
    info "$sort_cnt sort/materialize node(s) — if spilling to disk, check: kbdiag temp"
  fi

  # Top line carries the planner's total estimate: "... (cost=a..b rows=n width=w)"
  local est
  est=$(printf '%s' "$plan" | head -1 | grep -oE 'cost=[0-9.]+\.\.[0-9.]+ rows=[0-9]+' || true)
  if [[ -n "$est" ]]; then
    info "Planner estimate: $est"
  fi
}

_explain_plan() {
  local sql="$1" plan
  if ! plan=$(ksql_q "EXPLAIN $sql" 2>&1); then
    fail "EXPLAIN failed:"
    printf '%s\n' "$plan" | head -3
    return 0
  fi
  printf "\nPlan:\n%s\n" "$plan"
  _explain_flags "$plan"
}

cmd_explain() {
  hdr "Plan Analysis"
  # Unquoted multi-word SQL arrives as multiple args — join them back.
  local target="$*"
  target="${target#"${target%%[![:space:]]*}"}"

  if [[ -z "$target" ]]; then
    warn "Usage: kbdiag explain <queryid | \"SQL statement\">"
    info "Find queryids with: kbdiag stmt"
    return 0
  fi

  if [[ "$target" =~ ^-?[0-9]+$ ]]; then
    _stmt_check_ext || return 0
    local sql
    sql=$(ksql_q "SELECT query FROM sys_stat_statements WHERE queryid=$target LIMIT 1;" 2>/dev/null) || sql=""
    if [[ -z "${sql// /}" ]]; then
      warn "queryid $target not found in sys_stat_statements — list candidates with: kbdiag stmt"
      return 0
    fi
    _explain_stmt_stats "$target"
    printf "\nQuery:\n%s\n" "$sql"
    # Normalized statements carry $1,$2… placeholders — EXPLAIN can't plan those.
    if printf '%s' "$sql" | grep -qE '\$[0-9]+'; then
      warn "Query is normalized with \$n parameters — substitute literal values, then run: kbdiag explain \"<SQL>\""
      return 0
    fi
    _explain_plan "$sql"
  else
    _explain_plan "$target"
  fi
}

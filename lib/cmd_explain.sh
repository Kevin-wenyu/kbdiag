# shellcheck shell=bash
# cmd_explain.sh — plan analysis for a queryid or a literal SQL statement.
#
# DBA tier: verifies ROOT-CAUSE conclusions (diagnose/advisor point at a
# query; explain shows why it's slow). Uses plain EXPLAIN — never executes
# the statement, so DML is safe to analyze.

_explain_stmt_stats() {
  local qid="$1" row
  _EXPLAIN_CALLS=""; _EXPLAIN_MEAN=""; _EXPLAIN_TOTAL=""; _EXPLAIN_BLKS=""; _EXPLAIN_TEMP=""
  row=$(ksql_q "SELECT calls::text
                  ||'|'||round(mean_exec_time::numeric,1)
                  ||'|'||round(total_exec_time::numeric,1)
                  ||'|'||shared_blks_read
                  ||'|'||temp_blks_written
                FROM sys_stat_statements WHERE queryid=$qid LIMIT 1;" 2>/dev/null) || row=""
  [[ -z "${row// /}" ]] && return 0
  IFS='|' read -r _EXPLAIN_CALLS _EXPLAIN_MEAN _EXPLAIN_TOTAL _EXPLAIN_BLKS _EXPLAIN_TEMP <<< "$row"
  _EXPLAIN_CALLS="${_EXPLAIN_CALLS// /}"; _EXPLAIN_MEAN="${_EXPLAIN_MEAN// /}"
  _EXPLAIN_TOTAL="${_EXPLAIN_TOTAL// /}"; _EXPLAIN_BLKS="${_EXPLAIN_BLKS// /}"; _EXPLAIN_TEMP="${_EXPLAIN_TEMP// /}"
  info "History: ${_EXPLAIN_CALLS} call(s), mean ${_EXPLAIN_MEAN}ms, total ${_EXPLAIN_TOTAL}ms, ${_EXPLAIN_BLKS} shared blks read, ${_EXPLAIN_TEMP} temp blks written"
}

# Reads plan text, prints red-flag verdicts (text mode) / json_items (json
# mode). Returns 1 if a warn-level flag was raised, else 0 — feeds the
# caller's exit-code accumulator.
_explain_flags() {
  local plan="$1"
  local seq_cnt loop_cnt sort_cnt est
  seq_cnt=$(printf '%s' "$plan" | grep -c 'Seq Scan' || true)
  loop_cnt=$(printf '%s' "$plan" | grep -c 'Nested Loop' || true)
  sort_cnt=$(printf '%s' "$plan" | grep -cE 'Sort|Materialize' || true)
  est=$(printf '%s' "$plan" | head -1 | grep -oE 'cost=[0-9.]+\.\.[0-9.]+ rows=[0-9]+' || true)

  if [[ "$OUTPUT_FMT" != "json" ]]; then
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
    if [[ -n "$est" ]]; then
      info "Planner estimate: $est"
    fi
  else
    if [[ "${seq_cnt:-0}" -gt 0 ]]; then
      json_item "seq_scans" "warn" "$seq_cnt" "check idx missing / colstat"
    else
      json_item "seq_scans" "ok" "0" ""
    fi
    if [[ "${loop_cnt:-0}" -gt 1 ]]; then
      json_item "nested_loops" "warn" "$loop_cnt" "verify with colstat"
    else
      json_item "nested_loops" "ok" "${loop_cnt:-0}" ""
    fi
    json_item "sort_materialize" "ok" "${sort_cnt:-0}" "$est"
  fi

  [[ "${seq_cnt:-0}" -gt 0 || "${loop_cnt:-0}" -gt 1 ]] && return 1
  return 0
}

# Runs EXPLAIN, sets $_EXPLAIN_PLAN. Returns 2 on EXPLAIN failure, otherwise
# whatever _explain_flags returns (0 ok / 1 warn).
_explain_plan() {
  local sql="$1"
  # ksql_q discards stderr internally — call ksql directly here so a real
  # syntax/planning error is captured instead of an empty string.
  if ! _EXPLAIN_PLAN=$(timeout "$KB_QUERY_TIMEOUT" "$KSQL" -p "$KB_PORT" -U "$KB_SUPERUSER" "$KB_DB" -AXtc "EXPLAIN $sql" 2>&1); then
    local err_head
    err_head=$(printf '%s\n' "$_EXPLAIN_PLAN" | head -3)
    fail "EXPLAIN failed:"
    if [[ "$OUTPUT_FMT" != "json" ]]; then
      printf '%s\n' "$err_head"
    else
      json_item "explain" "fail" "" "$(printf '%s' "$err_head" | tr '\n' ' ')"
    fi
    return 2
  fi
  [[ "$OUTPUT_FMT" != "json" ]] && printf "\nPlan:\n%s\n" "$_EXPLAIN_PLAN"
  _explain_flags "$_EXPLAIN_PLAN"
}

cmd_explain() {
  hdr "Plan Analysis"
  # Unquoted multi-word SQL arrives as multiple args — join them back.
  local target="$*"
  target="${target#"${target%%[![:space:]]*}"}"

  local _exit=0 qid="" sql
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "explain"

  if [[ -z "$target" ]]; then
    warn "Usage: kbdiag explain <queryid | \"SQL statement\">"
    info "Find queryids with: kbdiag stmt"
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return 1
  fi

  _EXPLAIN_PLAN=""
  _EXPLAIN_CALLS=""; _EXPLAIN_MEAN=""; _EXPLAIN_TOTAL=""; _EXPLAIN_BLKS=""; _EXPLAIN_TEMP=""

  if [[ "$target" =~ ^-?[0-9]+$ ]]; then
    qid="$target"
    if ! _stmt_check_ext; then
      if [[ "$OUTPUT_FMT" == "json" ]]; then
        json_item "queryid" "warn" "$target" "sys_stat_statements not loaded"
        json_end
      else
        _stmt_install_hint
      fi
      [[ -n "$EXIT_CODE_MODE" ]] && return 1
      return 0
    fi
    sql=$(ksql_q "SELECT query FROM sys_stat_statements WHERE queryid=$target LIMIT 1;" 2>/dev/null) || sql=""
    if [[ -z "${sql// /}" ]]; then
      warn "queryid $target not found in sys_stat_statements — list candidates with: kbdiag stmt"
      if [[ "$OUTPUT_FMT" == "json" ]]; then
        json_item "queryid" "warn" "$target" "not found"
        json_end
      fi
      [[ -n "$EXIT_CODE_MODE" ]] && return 1
      return 0
    fi
    _explain_stmt_stats "$target"
    [[ "$OUTPUT_FMT" != "json" ]] && printf "\nQuery:\n%s\n" "$sql"

    # Normalized statements carry $1,$2… placeholders — EXPLAIN can't plan those.
    if printf '%s' "$sql" | grep -qE '\$[0-9]+'; then
      warn "Query is normalized with \$n parameters — substitute literal values, then run: kbdiag explain \"<SQL>\""
      if [[ "$OUTPUT_FMT" == "json" ]]; then
        json_item "queryid" "warn" "$target" "normalized with \$n params, cannot EXPLAIN directly"
        json_end
      fi
      [[ -n "$EXIT_CODE_MODE" ]] && return 1
      return 0
    fi
  else
    sql="$target"
  fi

  local plan_status=0
  _explain_plan "$sql" || plan_status=$?
  case "$plan_status" in
    2) _exit=2 ;;
    1) _exit=1 ;;
  esac

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    [[ -n "$qid" ]] && json_item "queryid" "ok" "$qid" ""
    [[ -n "$_EXPLAIN_CALLS" ]] && json_item "history" "ok" "${_EXPLAIN_CALLS} calls" "mean=${_EXPLAIN_MEAN}ms total=${_EXPLAIN_TOTAL}ms shared_blks_read=${_EXPLAIN_BLKS} temp_blks_written=${_EXPLAIN_TEMP}"
    json_row queryid="$qid" sql="$(printf '%s' "$sql" | tr '\n' ' ')" plan="$(printf '%s' "$_EXPLAIN_PLAN" | tr '\n' ' ')"
    json_end
  fi

  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}

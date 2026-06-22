cmd_license() {
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "license"
  hdr "License"

  local validdays
  validdays=$(ksql_q "SELECT get_license_validdays();" | tr -d '[:space:]')
  validdays="${validdays:-0}"

  local status_label status_level
  if [[ "$validdays" == "-2" ]]; then
    status_label="永久授权（正式）"
    status_level="ok"
  elif [[ "$validdays" -gt "$KB_WARN_LICENSE_DAYS" ]]; then
    status_label="剩余 ${validdays} 天"
    status_level="ok"
  elif [[ "$validdays" -gt 0 ]]; then
    status_label="剩余 ${validdays} 天，即将到期"
    status_level="warn"
  else
    status_label="已过期（validdays=${validdays}）"
    status_level="fail"
  fi

  local info_text serial product edition username project issued
  info_text=$(ksql_q "SELECT get_license_info();")
  serial=$(echo "$info_text"   | grep 'License序列号'  | sed 's/.*--- 启用 --- //' | tr -d ' \t\r\n+')
  product=$(echo "$info_text"  | grep '产品名称'       | sed 's/.*--- 启用 --- //' | tr -d ' \t\r\n+')
  edition=$(echo "$info_text"  | grep '细分版本模板名' | sed 's/.*--- 启用 --- //' | tr -d ' \t\r\n+')
  username=$(echo "$info_text" | grep '用户名称'       | sed 's/.*--- 启用 --- //' | tr -d ' \t\r\n+')
  project=$(echo "$info_text"  | grep '项目名称'       | sed 's/.*--- 启用 --- //' | tr -d ' \t\r\n+')
  issued=$(echo "$info_text"   | grep '生产日期'       | sed 's/.*--- 启用 --- //' | tr -d ' \t\r\n+')

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "license_status" "$status_level" "$status_label"  "validdays=${validdays}"
    json_item "serial"         "ok"            "$serial"        ""
    json_item "product"        "ok"            "$product"       ""
    json_item "edition"        "ok"            "$edition"       ""
    json_item "user"           "ok"            "$username"      ""
    json_item "project"        "ok"            "$project"       ""
    json_item "issued"         "ok"            "$issued"        ""
    json_end
    return
  fi

  case "$status_level" in
    ok)   ok   "授权状态: $status_label" ;;
    warn) warn "授权状态: $status_label" ;;
    fail) fail "授权状态: $status_label" ;;
  esac

  info "序列号:   $serial"
  info "产品名称: $product"
  info "版本模板: $edition"
  info "用户名称: $username"
  info "项目名称: $project"
  info "生产日期: $issued"

  if [[ -n "$VERBOSE" ]]; then
    echo ""
    echo "$info_text"
  fi
}

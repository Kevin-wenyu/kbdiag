# shellcheck shell=bash
cmd_cluster() {
  hdr "Cluster status (repmgr)"

  if ! repmgr_available; then
    warn "repmgr not found — standalone instance"
    return 0
  fi

  "$REPMGR" -f "$KB_REPMGR_CONF" cluster show
}

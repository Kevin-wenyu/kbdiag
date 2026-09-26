package probe

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// instInfoSQL is inst.info.
// Source: written for kbdiag against the KES V8R6 manual
// (help.kingbase.com.cn/v8: 系统信息函数 sys_postmaster_start_time;
// 动态性能视图 sys_stat_database; 系统视图 sys_settings), checked
// 2026-09-24 on node1/node2. KES notes:
//   - connections is sum(numbackends): backends connected to a database,
//     visible to any user (sys_stat_activity masks backend_type)
//   - data_directory goes through sys_settings, not current_setting(),
//     so a user without the right sees NULL instead of an error
//   - version() is the full text; InstInfo keeps only the version number
const instInfoSQL = `
select version(),
       sys_postmaster_start_time(),
       round(extract(epoch from now() - sys_postmaster_start_time())::numeric, 1)::float8,
       (select coalesce(sum(numbackends), 0) from sys_stat_database)::int,
       current_setting('max_connections')::int,
       current_setting('superuser_reserved_connections')::int,
       (select setting from sys_settings where name = 'data_directory'),
       current_setting('port')::int`

// instDatabasesSQL is inst.databases. pg_database_size needs CONNECT on the
// database (size functions only have the pg_ prefix in KES).
const instDatabasesSQL = `
select datname::text,
       case when has_database_privilege(oid, 'CONNECT') then pg_database_size(oid) end
from sys_database
where not datistemplate
order by datname`

// instDownstreamsSQL is inst.downstreams: one row per walsender feeding a
// downstream, whatever its state.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 动态性能视图 sys_stat_replication), columns read 2026-09-26 on node1.
// KES notes:
//   - sync_state is kept raw: with repmgr the lab reports quorum, not sync/async
//   - host() drops the /32 that inet::text would add
//   - unlike PG, a user without sys_monitor sees every column (kbdiag_ro,
//     2026-09-26); NULL state/sync_state is still handled as masked
const instDownstreamsSQL = `
select application_name::text, host(client_addr), state::text, sync_state::text
from sys_stat_replication
order by application_name, pid`

// instUpstreamSQL is inst.upstream: the WAL receiver of a standby, no row
// when there is none.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 动态性能视图 sys_stat_wal_receiver), columns read 2026-09-26 on node2.
// KES notes:
//   - pg_is_wal_receiver_up() does not exist; the status column is the answer
//   - the view names no repmgr node, only sender_host:sender_port
//   - unlike PG, a user without sys_monitor sees every column (kbdiag_ro,
//     2026-09-26); a row of NULLs is still handled as masked
const instUpstreamSQL = `
select status::text, sender_host::text, sender_port, slot_name::text,
       round(extract(epoch from now() - last_msg_receipt_time)::numeric, 1)::float8
from sys_stat_wal_receiver`

func InstInfo(ctx context.Context, x *pgx.Conn) facts.InstInfo {
	st, reason, out := collect(ctx, x, instInfoSQL, func(r pgx.CollectableRow) (facts.Info, error) {
		var i facts.Info
		err := r.Scan(&i.Version, &i.StartTime, &i.UptimeS, &i.Connections, &i.MaxConnections, &i.SuperuserReserved, &i.DataDirectory, &i.Port)
		i.Version = versionNumber(i.Version)
		return i, err
	})
	return facts.InstInfo{Status: st, Reason: reason, Rows: out}
}

func InstDatabases(ctx context.Context, x *pgx.Conn) facts.InstDatabases {
	st, reason, out := collect(ctx, x, instDatabasesSQL, func(r pgx.CollectableRow) (facts.Database, error) {
		var d facts.Database
		err := r.Scan(&d.Datname, &d.SizeBytes)
		return d, err
	})
	return facts.InstDatabases{Status: st, Reason: reason, Rows: out}
}

func InstDownstreams(ctx context.Context, x *pgx.Conn) facts.InstDownstreams {
	st, reason, out := collect(ctx, x, instDownstreamsSQL, func(r pgx.CollectableRow) (facts.Downstream, error) {
		var d facts.Downstream
		err := r.Scan(&d.ApplicationName, &d.ClientAddr, &d.State, &d.SyncState)
		return d, err
	})
	return facts.InstDownstreams{Status: st, Reason: reason, Rows: out}
}

// InstUpstream reads the WAL receiver. A primary has none to read.
func InstUpstream(ctx context.Context, x *pgx.Conn, c facts.Context) facts.InstUpstream {
	if c.Role != "standby" {
		return facts.InstUpstream{Status: facts.StatusNotApplicable, Reason: "primary"}
	}
	st, reason, out := collect(ctx, x, instUpstreamSQL, func(r pgx.CollectableRow) (facts.Upstream, error) {
		var u facts.Upstream
		err := r.Scan(&u.Status, &u.SenderHost, &u.SenderPort, &u.SlotName, &u.LastMsgAgeS)
		return u, err
	})
	return facts.InstUpstream{Status: st, Reason: reason, Rows: out}
}

// statfs is swapped out in tests.
var statfs = statfsOS

// InstDisk reads the filesystem holding data_directory. It is the one probe
// that is not SQL: KES has no function for free disk space, so it only works
// when kbdiag runs on the database host. A Unix socket proves that; a
// loopback host only does if the directory is there, since the port may be
// forwarded to another machine. Nothing is judged on it.
func InstDisk(i facts.InstInfo, socket, loopback bool) facts.InstDisk {
	if !socket && !loopback {
		return facts.InstDisk{Status: facts.StatusNotApplicable, Reason: "remote connection"}
	}
	if i.Status != facts.StatusOK || len(i.Rows) == 0 {
		return facts.InstDisk{Status: facts.StatusSkipped, Reason: "inst.info was not collected, so data_directory is unknown"}
	}
	dir := i.Rows[0].DataDirectory
	if dir == nil {
		return facts.InstDisk{Status: facts.StatusSkipped, Reason: "insufficient_privilege: data_directory is not visible"}
	}
	d, err := statfs(*dir)
	switch {
	case err == nil:
		return facts.InstDisk{Status: facts.StatusOK, Rows: []facts.Disk{d}}
	case socket:
		return facts.InstDisk{Status: facts.StatusError, Reason: err.Error()}
	}
	return facts.InstDisk{Status: facts.StatusNotApplicable, Reason: "data_directory is not accessible on this host (the port may be forwarded elsewhere): " + err.Error()}
}

// versionNumber keeps "V008R006C009B0014" from "KingbaseES V008R006C009B0014
// on ...". Anything else is kept whole rather than guessed at.
func versionNumber(v string) string {
	f := strings.Fields(v)
	if len(f) < 2 || f[0] != "KingbaseES" {
		return v
	}
	return f[1]
}

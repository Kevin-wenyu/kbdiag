package probe

import (
	"context"

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
const instInfoSQL = `
select version(),
       sys_postmaster_start_time(),
       round(extract(epoch from now() - sys_postmaster_start_time())::numeric, 1)::float8,
       (select coalesce(sum(numbackends), 0) from sys_stat_database)::int,
       current_setting('max_connections')::int,
       current_setting('superuser_reserved_connections')::int,
       (select setting from sys_settings where name = 'data_directory')`

// instDatabasesSQL is inst.databases. pg_database_size needs CONNECT on the
// database (size functions only have the pg_ prefix in KES).
const instDatabasesSQL = `
select datname::text,
       case when has_database_privilege(oid, 'CONNECT') then pg_database_size(oid) end
from sys_database
where not datistemplate
order by datname`

// instDownstreamsSQL is inst.downstreams: walsenders feeding this instance's
// downstreams, whatever their state.
const instDownstreamsSQL = `select count(*) from sys_stat_replication`

func InstInfo(ctx context.Context, x *pgx.Conn) facts.InstInfo {
	st, reason, out := collect(ctx, x, instInfoSQL, func(r pgx.CollectableRow) (facts.Info, error) {
		var i facts.Info
		err := r.Scan(&i.Version, &i.StartTime, &i.UptimeS, &i.Connections, &i.MaxConnections, &i.SuperuserReserved, &i.DataDirectory)
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
	st, reason, out := collect(ctx, x, instDownstreamsSQL, pgx.RowTo[int64])
	return facts.InstDownstreams{Status: st, Reason: reason, Rows: out}
}

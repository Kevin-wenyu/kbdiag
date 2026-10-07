package probe

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// spaceTablespacesSQL is space.tablespaces.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 系统表 sys_tablespace; 数据库对象管理函数 pg_tablespace_location,
// pg_tablespace_size). Stage 0 capture (space_*_tablespace): kbdiag_ro gets
// "permission denied for tablespace sys_global", which fails the whole
// query, so the size is only asked for where it may be read: CREATE on the
// tablespace, the database's own default tablespace, or sys_monitor (as
// pg_monitor, it includes reading all statistics). Not run on a VM yet.
// The location is an empty string for sys_default and sys_global (inside data_directory).
const spaceTablespacesSQL = `
select spcname::text,
       pg_tablespace_location(oid),
       case when has_tablespace_privilege(oid, 'CREATE')
              or oid = (select dattablespace from sys_database where datname = current_database())
              or pg_has_role('sys_monitor', 'MEMBER')
            then pg_tablespace_size(oid) end
from sys_tablespace
order by spcname`

// spaceWALSQL is space.wal.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 系统管理函数 sys_ls_waldir; 系统视图 sys_settings). Stage 0 capture
// (space_*_waldir, space_*_settings): 177 files, 2.8 GB on node1;
// max_wal_size is in MB, wal_segment_size in B, wal_keep_segments has no
// unit. kbdiag_ro may not call sys_ls_waldir, so the probe is skipped for it.
// Not run on a VM yet.
const spaceWALSQL = `
with s as (
  select name, setting::numeric * case unit when 'B' then 1 when 'kB' then 1024 when '8kB' then 8192
                                             when 'MB' then 1048576 when 'GB' then 1073741824 else 1 end as v
  from sys_settings
  where name in ('max_wal_size', 'wal_keep_segments', 'wal_segment_size'))
select count(*)::bigint,
       coalesce(sum(size), 0)::bigint,
       (select v from s where name = 'max_wal_size')::bigint,
       ((select v from s where name = 'wal_keep_segments') * (select v from s where name = 'wal_segment_size'))::bigint,
       (select v from s where name = 'wal_segment_size')::bigint
from sys_ls_waldir()`

func SpaceTablespaces(ctx context.Context, x *pgx.Conn) facts.SpaceTablespaces {
	st, reason, out := collect(ctx, x, spaceTablespacesSQL, func(r pgx.CollectableRow) (facts.Tablespace, error) {
		var t facts.Tablespace
		err := r.Scan(&t.Name, &t.Location, &t.SizeBytes)
		return t, err
	})
	return facts.SpaceTablespaces{Status: st, Reason: reason, Rows: out}
}

func SpaceWAL(ctx context.Context, x *pgx.Conn) facts.SpaceWAL {
	st, reason, out := collect(ctx, x, spaceWALSQL, func(r pgx.CollectableRow) (facts.WAL, error) {
		var w facts.WAL
		err := r.Scan(&w.Files, &w.Bytes, &w.MaxWALSizeBytes, &w.WALKeepBytes, &w.WALSegmentBytes)
		return w, err
	})
	return facts.SpaceWAL{Status: st, Reason: reason, Rows: out}
}

// SpaceDisk reads the filesystem of data_directory, of sys_wal (often a
// symlink to another disk) and of each tablespace outside data_directory.
// Like inst.disk it is a statfs, not SQL, and follows the same rules for
// when it may run.
func SpaceDisk(i facts.InstInfo, t facts.SpaceTablespaces, socket, loopback bool) facts.SpaceDisk {
	base := InstDisk(i, socket, loopback)
	if base.Status != facts.StatusOK {
		return facts.SpaceDisk{Status: base.Status, Reason: base.Reason}
	}
	dir := *i.Rows[0].DataDirectory
	first := base.Rows[0]
	var err error
	if first.FSID, err = deviceOf(dir); err != nil {
		return facts.SpaceDisk{Status: facts.StatusError, Reason: err.Error()}
	}
	out := []facts.Mount{{Kind: "data_directory", Path: dir, Disk: first}}
	paths := [][2]string{{"wal", filepath.Join(dir, "sys_wal")}}
	if t.Status == facts.StatusOK {
		for _, x := range t.Rows {
			if x.Location != nil && *x.Location != "" {
				paths = append(paths, [2]string{"tablespace", *x.Location})
			}
		}
	}
	// A tablespace that cannot be statted (unmounted, removed) fails the
	// probe but keeps the rows already read: the data and WAL filesystems
	// can still be judged.
	var bad []string
	for _, p := range paths {
		d, err := statfs(p[1])
		if err == nil {
			d.FSID, err = deviceOf(p[1])
		}
		if err != nil {
			if p[0] != "tablespace" {
				return facts.SpaceDisk{Status: facts.StatusError, Reason: err.Error()}
			}
			bad = append(bad, err.Error())
			continue
		}
		out = append(out, facts.Mount{Kind: p[0], Path: p[1], Disk: d})
	}
	if len(bad) > 0 {
		return facts.SpaceDisk{Status: facts.StatusError, Reason: strings.Join(bad, "; "), Rows: out}
	}
	return facts.SpaceDisk{Status: facts.StatusOK, Rows: out}
}

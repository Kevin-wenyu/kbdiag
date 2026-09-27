package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// replDownstreamsSQL is repl.downstreams: every walsender, with how far each
// of its LSNs is behind this node's current position. sys_current_wal_lsn()
// fails on a standby (recovery is in progress), so a standby feeding a
// cascade measures from the furthest WAL it has (received or replayed: a
// cascading walsender sends up to both).
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 动态性能视图 sys_stat_replication; 备份控制函数 sys_current_wal_lsn,
// sys_last_wal_replay_lsn, sys_wal_lsn_diff). Stage 0 capture
// (repl_*_stat): the lag intervals are NULL on an idle, caught-up standby;
// kbdiag_ro sees every column. Not run on a VM yet.
const replDownstreamsSQL = `
with cur as (select case when sys_is_in_recovery() then greatest(sys_last_wal_receive_lsn(), sys_last_wal_replay_lsn())
                         else sys_current_wal_lsn() end as lsn)
select r.pid, r.application_name::text, host(r.client_addr), r.state::text, r.sync_state::text, r.sync_priority,
       r.sent_lsn::text, r.write_lsn::text, r.flush_lsn::text, r.replay_lsn::text,
       sys_wal_lsn_diff(cur.lsn, r.sent_lsn)::bigint,
       sys_wal_lsn_diff(cur.lsn, r.flush_lsn)::bigint,
       sys_wal_lsn_diff(cur.lsn, r.replay_lsn)::bigint,
       extract(epoch from r.write_lag)::float8,
       extract(epoch from r.flush_lag)::float8,
       extract(epoch from r.replay_lag)::float8,
       round(extract(epoch from now() - r.reply_time)::numeric, 1)::float8
from sys_stat_replication r, cur
order by r.application_name, r.pid`

// replSyncSQL is repl.sync (stage 0 capture repl_*_settings:
// 'ANY 1( node2)' and remote_apply on the lab).
const replSyncSQL = `
select (select setting from sys_settings where name = 'synchronous_standby_names'),
       (select setting from sys_settings where name = 'synchronous_commit')`

// replReplaySQL is repl.replay on a standby.
// Source: KES V8R6 manual (备份控制函数 sys_last_wal_receive_lsn,
// sys_last_wal_replay_lsn, sys_last_xact_replay_timestamp,
// sys_is_wal_replay_paused). Stage 0 capture (repl_node2_*_lsn): the last
// replayed transaction was 7h old while received = replayed on an idle
// primary, so its age is shown, never judged. Not run on a VM yet.
const replReplaySQL = `
select sys_last_wal_receive_lsn()::text, sys_last_wal_replay_lsn()::text,
       sys_wal_lsn_diff(sys_last_wal_receive_lsn(), sys_last_wal_replay_lsn())::bigint,
       round(extract(epoch from now() - sys_last_xact_replay_timestamp())::numeric, 1)::float8,
       sys_is_wal_replay_paused()`

func ReplDownstreams(ctx context.Context, x *pgx.Conn) facts.ReplDownstreams {
	st, reason, out := collect(ctx, x, replDownstreamsSQL, func(r pgx.CollectableRow) (facts.Replica, error) {
		var d facts.Replica
		err := r.Scan(&d.PID, &d.ApplicationName, &d.ClientAddr, &d.State, &d.SyncState, &d.SyncPriority, &d.SentLSN, &d.WriteLSN, &d.FlushLSN, &d.ReplayLSN,
			&d.SentLagBytes, &d.FlushLagBytes, &d.ReplayLagBytes, &d.WriteLagS, &d.FlushLagS, &d.ReplayLagS, &d.ReplyAgeS)
		return d, err
	})
	return facts.ReplDownstreams{Status: st, Reason: reason, Rows: out}
}

// ReplSync reads the synchronous replication settings; they only act on a
// primary.
func ReplSync(ctx context.Context, x *pgx.Conn, c facts.Context) facts.ReplSync {
	if c.Role == "standby" {
		return facts.ReplSync{Status: facts.StatusNotApplicable, Reason: "standby: synchronous replication is decided on the primary"}
	}
	st, reason, out := collect(ctx, x, replSyncSQL, func(r pgx.CollectableRow) (facts.SyncSetting, error) {
		var s facts.SyncSetting
		err := r.Scan(&s.StandbyNames, &s.Commit)
		return s, err
	})
	return facts.ReplSync{Status: st, Reason: reason, Rows: out}
}

// ReplReplay reads the standby's receive and replay positions.
func ReplReplay(ctx context.Context, x *pgx.Conn, c facts.Context) facts.ReplReplay {
	if c.Role != "standby" {
		return facts.ReplReplay{Status: facts.StatusNotApplicable, Reason: "primary"}
	}
	st, reason, out := collect(ctx, x, replReplaySQL, func(r pgx.CollectableRow) (facts.Replay, error) {
		var p facts.Replay
		err := r.Scan(&p.ReceiveLSN, &p.ReplayLSN, &p.ReplayGapBytes, &p.LastReplayAgeS, &p.ReplayPaused)
		return p, err
	})
	return facts.ReplReplay{Status: st, Reason: reason, Rows: out}
}

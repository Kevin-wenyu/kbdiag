package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// seqListSQL is seq.list: every sequence of the current database.
// last_value is NULL both for a sequence never called and for one this
// account may not read (stage 0 capture seq_*_ro_list: all NULL for
// kbdiag_ro), so readable says which: the view shows it only with SELECT
// or USAGE on the sequence.
// Source: KES V8R6 manual (系统视图 sys_sequences); stage 0 capture
// seq_*_list (bigint and integer sequences on the lab).
// has_sequence_privilege with a list of privileges is the PG kernel's
// (not captured). Not run on a VM yet.
const seqListSQL = `
select s.schemaname::text, s.sequencename::text, s.data_type::text, s.start_value, s.min_value, s.max_value, s.increment_by, s.cycle, s.cache_size, s.last_value,
       coalesce(has_sequence_privilege(c.oid, 'SELECT, USAGE'), false)
from sys_sequences s
left join sys_namespace n on n.nspname = s.schemaname
left join sys_class c on c.relnamespace = n.oid and c.relname = s.sequencename
order by 1, 2`

func SeqList(ctx context.Context, x *pgx.Conn) facts.SeqList {
	st, reason, out := collect(ctx, x, seqListSQL, func(r pgx.CollectableRow) (facts.Sequence, error) {
		var s facts.Sequence
		err := r.Scan(&s.Schemaname, &s.Sequencename, &s.DataType, &s.StartValue, &s.MinValue, &s.MaxValue, &s.IncrementBy, &s.Cycle, &s.CacheSize, &s.LastValue, &s.Readable)
		return s, err
	})
	return facts.SeqList{Status: st, Reason: reason, Rows: out}
}

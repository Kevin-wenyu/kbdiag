package rule

import (
	"fmt"
	"strconv"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// Table judges one table with the freeze and vacuum rules, no rule of its
// own: its age against the freeze and stop limits (freeze.table_age), and
// autovacuum turned off for it while past its threshold
// (vacuum.table_disabled, primary only: statistics are local).
func Table(i facts.TableInfos, s facts.TableStats, l facts.FreezeLimits, v facts.VacuumSettings, role, db string) Result {
	judge, unknown := collected(i.Status)
	if !judge || len(i.Rows) == 0 {
		return Result{Verdict: verdictOf(nil, true)}
	}
	t := i.Rows[0]
	rel := t.Schemaname + "." + t.Relname
	var fs []Finding
	if t.XIDAge != nil {
		mxid := int32(0)
		if t.MXIDAge != nil {
			mxid = *t.MXIDAge
		}
		lim, limOK := freezeLimit(l)
		f, ok, blind := ageFinding("freeze.table_age", "table "+rel, *t.XIDAge, mxid, lim, limOK)
		unknown = unknown || blind
		if ok {
			f.Evidence = []Evidence{{ProbeID: facts.TableInfoID, Fields: map[string]any{"schemaname": t.Schemaname, "relname": t.Relname, "xid_age": *t.XIDAge, "mxid_age": t.MXIDAge}}}
			f.Next = []Next{{Kind: "verify", Command: "kbdiag txn", Note: "the oldest transaction or 2PC holding back freezing"},
				tableFix("VACUUM (FREEZE, VERBOSE)", t, "on the primary, connected to database "+db)}
			fs = append(fs, f)
		}
	}
	if role == "standby" {
		return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
	}
	sj, su := collected(s.Status)
	set, setOK := vacuumSetting(v)
	unknown = unknown || su || !setOK
	if sj && setOK && len(s.Rows) > 0 {
		vt := facts.VacuumTable{Schemaname: t.Schemaname, Relname: t.Relname, NDeadTup: s.Rows[0].NDeadTup, Reltuples: t.Reltuples, Reloptions: t.Reloptions}
		threshold, on := VacuumThreshold(vt, set)
		if !on && float32(vt.NDeadTup) > threshold {
			fs = append(fs, Finding{
				ID:    "vacuum.table_disabled",
				Level: LevelWARN,
				Symptom: fmt.Sprintf("table %s has %d dead tuples, past its autovacuum threshold %s, but autovacuum is off for it (autovacuum_enabled=off): nothing will clean it",
					rel, vt.NDeadTup, strconv.FormatFloat(float64(threshold), 'f', -1, 32)),
				Evidence: []Evidence{{ProbeID: facts.TableStatsID, Fields: map[string]any{
					"schemaname": t.Schemaname, "relname": t.Relname, "n_dead_tup": vt.NDeadTup, "reltuples": t.Reltuples, "threshold": float64(threshold),
				}}},
				Next: []Next{tableFix("VACUUM", t, "connected to database "+db+"; turn autovacuum back on with ALTER TABLE ... RESET (autovacuum_enabled) unless it was turned off on purpose")},
			})
		}
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

// tableFix is a fix SQL on the table. A name the text shows escaped is left
// out of the SQL: the reader typed it, and pasting the escaped one would
// not match.
func tableFix(sql string, t facts.TableInfo, note string) Next {
	if hasControl(t.Schemaname + t.Relname) {
		return Next{Kind: "fix", SQL: sql + " <table>", Note: "the name has control characters and is shown escaped: use it as you gave it; " + note}
	}
	return Next{Kind: "fix", SQL: sql + " " + qualified(t.Schemaname, t.Relname), Note: note}
}

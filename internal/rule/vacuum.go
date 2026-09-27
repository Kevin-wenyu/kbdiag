package rule

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// Vacuum flags the two cases where nothing will clean dead tuples: the
// autovacuum (or track_counts) switch is off, or a table past its
// autovacuum threshold has autovacuum turned off for itself. A table past
// its threshold with autovacuum on is autovacuum's normal queue, not a
// fault. On a standby autovacuum does not run: nothing is judged.
func Vacuum(t facts.VacuumTables, s facts.VacuumSettings, role, db string) Result {
	if role == "standby" {
		return Result{Verdict: VerdictOK}
	}
	judge, unknown := collected(t.Status)
	set, setOK := vacuumSetting(s)
	unknown = unknown || !setOK
	var fs []Finding
	if setOK && (set.Autovacuum != "on" || set.TrackCounts != "on") {
		fs = append(fs, vacuumDisabled(set))
	}
	if judge && setOK {
		for _, x := range t.Rows {
			threshold, on := VacuumThreshold(x, set)
			if on || float32(x.NDeadTup) <= threshold {
				continue
			}
			rel := x.Schemaname + "." + x.Relname
			where := "connected to database " + db
			if hasControl(db) {
				where = "connected to this database"
			}
			fix := Next{Kind: "fix", SQL: "VACUUM " + qualified(x.Schemaname, x.Relname), Note: where + "; turn autovacuum back on with ALTER TABLE ... RESET (autovacuum_enabled) unless it was turned off on purpose"}
			if hasControl(rel) {
				fix = Next{Kind: "verify", Command: "kbdiag vacuum --json", Note: "the table name has control characters and the text shows it escaped: take the raw name from the JSON before running VACUUM"}
			}
			fs = append(fs, Finding{
				ID:    "vacuum.table_disabled",
				Level: LevelWARN,
				Symptom: fmt.Sprintf("table %s has %d dead tuples, past its autovacuum threshold %s, but autovacuum is off for it (autovacuum_enabled=off): nothing will clean it",
					rel, x.NDeadTup, strconv.FormatFloat(float64(threshold), 'f', -1, 32)),
				Evidence: []Evidence{{ProbeID: facts.VacuumTablesID, Fields: map[string]any{
					"schemaname": x.Schemaname, "relname": x.Relname, "n_dead_tup": x.NDeadTup, "reltuples": x.Reltuples, "threshold": float64(threshold),
				}}},
				Next: []Next{
					fix,
					{Kind: "verify", Command: "kbdiag txn", Note: "if dead tuples remain after VACUUM: the oldest transaction holding back the horizon"},
				},
			})
		}
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

func vacuumSetting(s facts.VacuumSettings) (facts.VacuumSetting, bool) {
	if s.Status != facts.StatusOK || len(s.Rows) == 0 {
		return facts.VacuumSetting{}, false
	}
	return s.Rows[0], true
}

// VacuumThreshold is the dead-tuple count past which autovacuum vacuums the
// table: autovacuum_vacuum_threshold + autovacuum_vacuum_scale_factor ×
// reltuples, either overridden by the table's reloptions; on is false when
// the table has autovacuum_enabled set to a false value. The arithmetic is
// float4, as the server's (relation_needs_vacanalyze), so a table right at
// the line is judged the same way. Options that do not parse are ignored,
// as the server would have refused them.
func VacuumThreshold(t facts.VacuumTable, s facts.VacuumSetting) (threshold float32, on bool) {
	base, sf, on := float32(s.Threshold), float32(s.ScaleFactor), true
	for _, o := range t.Reloptions {
		k, v, ok := strings.Cut(o, "=")
		if !ok {
			continue
		}
		switch k {
		case "autovacuum_enabled":
			if b, ok := parseBool(v); ok {
				on = b
			}
		case "autovacuum_vacuum_threshold":
			if n, err := strconv.ParseFloat(v, 32); err == nil {
				base = float32(n)
			}
		case "autovacuum_vacuum_scale_factor":
			if n, err := strconv.ParseFloat(v, 32); err == nil {
				sf = float32(n)
			}
		}
	}
	return base + sf*t.Reltuples, on
}

// vacuumSwitch names the switch to turn back on (autovacuum first: without
// it track_counts alone changes nothing).
func vacuumSwitch(s facts.VacuumSetting) string {
	if s.Autovacuum != "on" {
		return "autovacuum"
	}
	return "track_counts"
}

// vacuumDisabled is the finding for autovacuum or track_counts turned off.
func vacuumDisabled(set facts.VacuumSetting) Finding {
	return Finding{
		ID:    "vacuum.disabled",
		Level: LevelWARN,
		Symptom: fmt.Sprintf("autovacuum=%s, track_counts=%s: no table is vacuumed automatically any more (only the anti-wraparound vacuum still runs), so dead tuples pile up",
			set.Autovacuum, set.TrackCounts),
		Evidence: []Evidence{{ProbeID: facts.VacuumSettingsID, Fields: map[string]any{"autovacuum": set.Autovacuum, "track_counts": set.TrackCounts}}},
		Next:     []Next{{Kind: "fix", SQL: "ALTER SYSTEM SET " + vacuumSwitch(set) + " = on", Note: "then SELECT sys_reload_conf(); unless it was turned off on purpose"}},
	}
}

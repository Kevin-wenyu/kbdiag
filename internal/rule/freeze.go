package rule

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// Freeze judges each database's age against two server lines: past
// autovacuum_freeze_max_age the anti-wraparound autovacuum should already be
// freezing it (WARN: nothing fails yet), and at the xid stop limit the
// server refuses new transaction IDs (FAIL: writes fail). Tables are not
// judged one by one: the oldest table is what makes the database old.
func Freeze(d facts.FreezeDatabases, l facts.FreezeLimits, role string) Result {
	judge, unknown := collected(d.Status)
	if !judge {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	lim, limOK := freezeLimit(l)
	var fs []Finding
	for _, x := range d.Rows {
		f, ok, blind := ageFinding("freeze.database_age", "database "+x.Datname, x.XIDAge, x.MXIDAge, lim, limOK)
		unknown = unknown || blind
		if !ok {
			continue
		}
		fields := map[string]any{"datname": x.Datname, "xid_age": x.XIDAge, "mxid_age": x.MXIDAge,
			"autovacuum_freeze_max_age": nil, "autovacuum_multixact_freeze_max_age": nil}
		if limOK {
			fields["autovacuum_freeze_max_age"], fields["autovacuum_multixact_freeze_max_age"] = lim.FreezeMaxAge, lim.MultiFreezeMaxAge
		}
		f.Evidence = []Evidence{{ProbeID: facts.FreezeDatabasesID, Fields: fields}}
		f.Next = freezeNext(x.Datname, x.AllowConn, role, f.Level == LevelFAIL)
		fs = append(fs, f)
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

func freezeLimit(l facts.FreezeLimits) (facts.FreezeLimit, bool) {
	if l.Status != facts.StatusOK || len(l.Rows) == 0 {
		return facts.FreezeLimit{}, false
	}
	return l.Rows[0], true
}

// ageFinding judges one xid and multixact age (of a database or a table).
// Without the limits only the stop limits can be judged; an age below them
// is then unknown.
func ageFinding(id, what string, xidAge, mxidAge int32, lim facts.FreezeLimit, limOK bool) (f Finding, found, unknown bool) {
	var fail, warn []string
	if int64(xidAge) >= facts.XIDStopAge {
		fail = append(fail, fmt.Sprintf("%d xids old, at or past the stop limit %d", xidAge, facts.XIDStopAge))
	} else if limOK && int64(xidAge) >= lim.FreezeMaxAge {
		warn = append(warn, fmt.Sprintf("%d xids old, past autovacuum_freeze_max_age %d (%d xids left before new transaction IDs are refused)",
			xidAge, lim.FreezeMaxAge, int64(facts.XIDStopAge)-int64(xidAge)))
	}
	if int64(mxidAge) >= facts.MXIDStopAge {
		fail = append(fail, fmt.Sprintf("%d multixacts old, at or past the stop limit %d", mxidAge, facts.MXIDStopAge))
	} else if limOK && int64(mxidAge) >= lim.MultiFreezeMaxAge {
		warn = append(warn, fmt.Sprintf("%d multixacts old, past autovacuum_multixact_freeze_max_age %d", mxidAge, lim.MultiFreezeMaxAge))
	}
	switch {
	case len(fail) > 0:
		return Finding{ID: id, Level: LevelFAIL, Symptom: what + " is " + strings.Join(append(fail, warn...), " and ") +
			": the server refuses new transaction IDs or multixacts, so writes fail"}, true, false
	case len(warn) > 0:
		return Finding{ID: id, Level: LevelWARN, Symptom: what + " is " + strings.Join(warn, " and ") +
			": the anti-wraparound autovacuum is due or cannot finish"}, true, false
	}
	return Finding{}, false, !limOK
}

func freezeNext(db string, allowConn bool, role string, stopped bool) []Next {
	next := []Next{{Kind: "verify", Command: "kbdiag txn", Note: "the oldest transaction or 2PC holding back freezing"}}
	if !allowConn { // template0: autovacuum freezes it; nobody can connect to
		return next
	}
	where := "connected to database " + db
	if hasControl(db) {
		// the text shows the name escaped: a pasted -d would not match
		next = append(next, Next{Kind: "verify", Command: "kbdiag freeze --json", Note: "the database name has control characters and the text shows it escaped: take the raw name from the JSON, then run kbdiag -d <name> freeze"})
		where = "connected to that database"
	} else {
		next = append(next, Next{Kind: "verify", Command: fmt.Sprintf("kbdiag -d %s freeze", shellWord(db)), Note: "its oldest tables"})
	}
	if role == "standby" {
		where = "on the primary, " + where
	}
	note := where + ", oldest tables first"
	if stopped {
		note += "; past the stop limit a PG12 kernel only runs it in single-user mode: follow the KES manual on transaction ID wraparound"
	}
	return append(next, Next{Kind: "fix", SQL: "VACUUM (FREEZE, VERBOSE) <table>", Note: note})
}

// shellWord quotes a name for a shell command line when it needs it.
func shellWord(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_-.", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

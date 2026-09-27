package facts

// ParamsChangedID is the probe_id of the non-default parameters.
const ParamsChangedID = "params.changed"

// ParamColumns is the column contract of params.changed (PRD §5.2).
var ParamColumns = []string{"name", "setting", "unit", "source", "sourcefile", "sourceline", "boot_val", "reset_val", "context", "pending_restart"}

// Param is one row of sys_settings that someone set: from a configuration
// file (ALTER SYSTEM writes kingbase.auto.conf), a database, a role, the
// command line or the environment.
type Param struct {
	Name           string
	Setting        *string
	Unit           *string
	Source         string
	Sourcefile     *string // NULL for accounts that may not read it
	Sourceline     *int32
	BootVal        *string
	ResetVal       *string
	Context        string
	PendingRestart bool
}

func (p Param) Row() []any {
	return []any{p.Name, p.Setting, p.Unit, p.Source, p.Sourcefile, p.Sourceline, p.BootVal, p.ResetVal, p.Context, p.PendingRestart}
}

type ParamsChanged struct {
	Status Status
	Reason string
	Rows   []Param
}

// Redacted reports parameters set in a file whose file this account may not
// read (stage 0 capture: kbdiag_ro sees sourcefile NULL). Such an account
// also does not see superuser-only parameters at all.
func (p ParamsChanged) Redacted() []Redaction {
	n := 0
	for _, x := range p.Rows {
		if x.Source == "configuration file" && x.Sourcefile == nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{
		{ProbeID: ParamsChangedID, Field: "sourcefile", Reason: ReasonInsufficientPrivilege, RowsAffected: n},
		{ProbeID: ParamsChangedID, Field: "sourceline", Reason: ReasonInsufficientPrivilege, RowsAffected: n},
	}
}

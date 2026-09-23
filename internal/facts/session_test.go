package facts

import "testing"

func str(s string) *string { return &s }

func TestRedacted(t *testing.T) {
	cases := []struct {
		name string
		rows []Session
		want int // rows_affected; 0 means no redaction entries
	}{
		{"empty", nil, 0},
		{"visible rows", []Session{{PID: 1, Query: str("select 1")}, {PID: 2}}, 0},
		{"null query is not masked", []Session{{PID: 1, Query: nil}}, 0},
		{"similar text is not masked", []Session{{PID: 1, Query: str("select '<insufficient privilege>'")}}, 0},
		{"two masked of three", []Session{{PID: 1, Query: str(insufficientPrivilege)}, {PID: 2, Query: str("x")}, {PID: 3, Query: str(insufficientPrivilege)}}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SessionActivity{Status: StatusOK, Rows: c.rows}.Redacted()
			if c.want == 0 {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if len(got) != len(maskedSessionColumns) {
				t.Fatalf("got %d entries, want %d", len(got), len(maskedSessionColumns))
			}
			for i, r := range got {
				if r.ProbeID != SessionActivityID || r.Field != maskedSessionColumns[i] || r.Reason != ReasonInsufficientPrivilege || r.RowsAffected != c.want {
					t.Errorf("entry %d = %+v", i, r)
				}
			}
		})
	}
}

func TestRedactedUntracked(t *testing.T) {
	disabled := Session{PID: 1, State: str("disabled"), Query: str("")}
	rows := []Session{disabled, {PID: 2, Query: str(insufficientPrivilege)}, disabled, {PID: 3, State: str("idle")}}
	got := SessionActivity{Status: StatusOK, Rows: rows}.Redacted()
	var untracked []Redaction
	for _, r := range got {
		if r.Reason == ReasonTrackActivitiesOff {
			untracked = append(untracked, r)
		} else if r.RowsAffected != 1 {
			t.Errorf("privilege entry %+v, want 1 row", r)
		}
	}
	if len(untracked) != len(untrackedSessionColumns) || len(got) != len(maskedSessionColumns)+len(untrackedSessionColumns) {
		t.Fatalf("got %v", got)
	}
	for i, r := range untracked {
		if r.ProbeID != SessionActivityID || r.Field != untrackedSessionColumns[i] || r.RowsAffected != 2 {
			t.Errorf("entry %d = %+v", i, r)
		}
	}
	if (Session{PID: 4, State: str("disabled")}).Masked() {
		t.Error("an untracked row is not a privilege mask")
	}
}

func TestRowMatchesColumns(t *testing.T) {
	if n := len(Session{}.Row()); n != len(SessionColumns) {
		t.Fatalf("Row has %d values, SessionColumns has %d", n, len(SessionColumns))
	}
}

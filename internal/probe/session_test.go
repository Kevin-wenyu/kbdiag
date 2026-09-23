package probe

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		err    error
		status facts.Status
		reason string
	}{
		{&pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}, facts.StatusSkipped, "timeout 57014: canceling statement due to statement timeout"},
		{&pgconn.PgError{Code: "55P03", Message: "lock timeout"}, facts.StatusSkipped, "timeout 55P03: lock timeout"},
		{&pgconn.PgError{Code: "42501", Message: "permission denied"}, facts.StatusSkipped, "insufficient_privilege 42501: permission denied"},
		{&pgconn.PgError{Code: "42P01", Message: "relation does not exist"}, facts.StatusError, "42P01: relation does not exist"},
		{fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "42501", Message: "x"}), facts.StatusSkipped, "insufficient_privilege 42501: x"},
		{errors.New("conn closed"), facts.StatusError, "conn closed"},
	}
	for _, c := range cases {
		st, reason := classify(c.err)
		if st != c.status || reason != c.reason {
			t.Errorf("classify(%v) = %s %q, want %s %q", c.err, st, reason, c.status, c.reason)
		}
	}
}

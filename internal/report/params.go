package report

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// paramsView is the params text layout (plan 2026-09-27 appendix B.5): the
// parameters waiting for a restart first, then everything someone set.
type paramsView struct{ p facts.ParamsChanged }

func (r *Report) SetParams(p facts.ParamsChanged) {
	v := &paramsView{p: p}
	r.layout = func(r *Report, w io.Writer) error { return v.write(w) }
}

func (v *paramsView) write(w io.Writer) error {
	if v.p.Status != facts.StatusOK {
		writeNotOKAs(w, "changed", v.p.Status, v.p.Reason)
		return nil
	}
	var pending [][]string
	for _, x := range v.p.Rows {
		if x.PendingRestart {
			pending = append(pending, []string{escapeControl(x.Name), paramValue(x.Setting), paramFile(x)})
		}
	}
	fmt.Fprintf(w, "\npending restart: %d\n", len(pending))
	if len(pending) > 0 {
		if err := writeTable(w, "  ", []string{"name", "running", "file"}, pending); err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "\nchanged: %d parameters not at their default (sources other than default, override and this connection)\n", len(v.p.Rows))
	fmt.Fprintln(w, "  this connection sets application_name, default_transaction_read_only, lock_timeout and statement_timeout itself, so their configured values are not shown; per-role and per-database settings show only for this role and database")
	if len(v.p.Rows) == 0 {
		return nil
	}
	var rows [][]string
	for _, x := range v.p.Rows {
		rows = append(rows, []string{escapeControl(x.Name), paramValue(x.Setting), cell(x.Unit), escapeControl(x.Source), paramFile(x)})
	}
	if err := writeTable(w, "  ", []string{"name", "setting", "unit", "source", "file"}, rows); err != nil {
		return err
	}
	if len(v.p.Redacted()) > 0 {
		fmt.Fprintln(w, "  only the parameters this account may read: superuser-only ones are not listed")
	}
	return nil
}

// paramFile is file:line, the file's base name (they sit in data_directory);
// "?" when this account may not read it, "-" when the value is not from a file.
func paramFile(x facts.Param) string {
	switch {
	case x.Sourcefile != nil:
		f := escapeControl(filepath.Base(*x.Sourcefile))
		if x.Sourceline != nil {
			f += fmt.Sprintf(":%d", *x.Sourceline)
		}
		return f
	case x.Source == "configuration file":
		return "?"
	}
	return "-"
}

// paramValue shows an empty setting as ” so it is not read as a gap.
func paramValue(s *string) string {
	if s != nil && *s == "" {
		return "''"
	}
	return cell(s)
}

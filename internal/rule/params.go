package rule

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// Params warns on each parameter changed and reloaded that only takes
// effect after a restart (the server's own pending_restart): the instance
// still runs with the old value, and the next restart, a failover's
// included, switches to the new one. An account that may not read the
// source files does not see superuser-only parameters either, so without a
// finding the answer is UNKNOWN.
func Params(p facts.ParamsChanged) Result {
	judge, unknown := collected(p.Status)
	if !judge {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	unknown = len(p.Redacted()) > 0
	var fs []Finding
	for _, x := range p.Rows {
		if !x.PendingRestart {
			continue
		}
		// source default: the new value is in a file but not yet in
		// effect, so the server has no file for it
		where := ""
		if x.Sourcefile != nil {
			where = " (" + filepath.Base(*x.Sourcefile)
			if x.Sourceline != nil {
				where += fmt.Sprintf(":%d", *x.Sourceline)
			}
			where += ")"
		} else if x.Source != "default" {
			where = " (" + x.Source + ")"
		}
		running := orDash(x.Setting)
		if x.Unit != nil && *x.Unit != "" {
			running += " (" + *x.Unit + ")"
		}
		lit := strings.ReplaceAll(x.Name, "'", "''")
		back := ""
		if x.Setting != nil {
			back = fmt.Sprintf("ALTER SYSTEM SET %s = '%s'", x.Name, strings.ReplaceAll(*x.Setting, "'", "''"))
		}
		fs = append(fs, Finding{
			ID:    "params.pending_restart",
			Level: LevelWARN,
			Symptom: fmt.Sprintf("parameter %s was changed%s but the instance still runs with %s: the new value takes effect only after a restart, the next one included",
				x.Name, where, running),
			Evidence: []Evidence{{ProbeID: facts.ParamsChangedID, Fields: map[string]any{
				"name": x.Name, "setting": x.Setting, "sourcefile": x.Sourcefile, "sourceline": x.Sourceline, "context": x.Context,
			}}},
			Next: []Next{
				{Kind: "verify", SQL: fmt.Sprintf("select sourcefile, sourceline, setting, applied, error from sys_file_settings where name = '%s'", lit), Note: "the value waiting in the file (needs a superuser)"},
				{Kind: "fix", SQL: back, Note: "if the change was not meant: writes the running value back; after SELECT sys_reload_conf() the flag clears (a reload clears it only when the file's value equals the running one, so ALTER SYSTEM RESET alone leaves it until a restart). Otherwise plan the restart"},
			},
		})
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

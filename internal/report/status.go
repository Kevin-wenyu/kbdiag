package report

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// writeStatus lays status out for a person (plan 2026-09-24-polish-status §1):
// single rows as key/value, sizes and durations in readable units, sections
// in the order the questions come up. JSON keeps the raw units.
func (r *Report) writeStatus(w io.Writer) error {
	order := []string{"inst.info", "inst.downstreams", "inst.upstream", "inst.databases", "inst.disk"}
	if r.Context.Role == "standby" {
		order[1], order[2] = order[2], order[1]
	}
	for _, id := range order {
		p, ok := r.Data[id]
		if !ok {
			continue
		}
		if p.Status != "ok" {
			fmt.Fprintf(w, "\n%s: %s", id, p.Status)
			if p.Reason != nil && *p.Reason != "" {
				fmt.Fprintf(w, "  (%s)", escapeControl(*p.Reason))
			}
			fmt.Fprintln(w)
			continue
		}
		var err error
		switch id {
		case "inst.info":
			err = writeInfo(w, p)
		case "inst.downstreams":
			err = writeDownstreams(w, p)
		case "inst.upstream":
			err = writeUpstream(w, p)
		case "inst.databases":
			err = writeDatabases(w, p)
		case "inst.disk":
			err = writeDisk(w, p)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func writeInfo(w io.Writer, p Probe) error {
	fmt.Fprintln(w, "\ninst.info")
	for _, row := range p.rows() {
		start := cell(row["start_time"])
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			start = t.Format(time.DateTime)
		}
		if up, ok := number(row["uptime_s"]); ok {
			start += "  (up " + duration(up) + ")"
		}
		conns := fmt.Sprintf("%s / %s  (max_connections %s - superuser_reserved %s)", cell(row["connections"]),
			cell(row["usable_connections"]), cell(row["max_connections"]), cell(row["superuser_reserved_connections"]))
		writeKV(w, 0, [][2]string{
			{"version", cell(row["version"])},
			{"data_directory", cell(row["data_directory"])},
			{"port", cell(row["port"])},
			{"start_time", start},
			{"connections", conns},
		})
	}
	return nil
}

func writeDownstreams(w io.Writer, p Probe) error {
	fmt.Fprintf(w, "\ninst.downstreams: %d\n", len(p.Rows))
	if len(p.Rows) == 0 {
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  name\taddress\tstate\tsync")
	for _, row := range p.rows() {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", cell(row["application_name"]), cell(row["client_addr"]), cell(row["state"]), cell(row["sync_state"]))
	}
	return tw.Flush()
}

func writeUpstream(w io.Writer, p Probe) error {
	fmt.Fprintln(w, "\ninst.upstream")
	if len(p.Rows) == 0 {
		// Same key width as a full upstream, so the value lines up with
		// what the reader saw on other standbys.
		writeKV(w, len("last_msg"), [][2]string{{"status", "(no walreceiver process)"}})
		return nil
	}
	for _, row := range p.rows() {
		up := "-"
		if row["sender_host"] != nil && !isNil(row["sender_host"]) {
			up = cell(row["sender_host"]) + ":" + cell(row["sender_port"])
		}
		last := "-"
		if s, ok := number(row["last_msg_age_s"]); ok {
			last = duration(s) + " ago"
		}
		writeKV(w, 0, [][2]string{{"status", cell(row["status"])}, {"upstream", up}, {"slot", cell(row["slot_name"])}, {"last_msg", last}})
	}
	return nil
}

// writeDatabases lists the largest first; sizes we may not read go last.
func writeDatabases(w io.Writer, p Probe) error {
	type db struct {
		name string
		size float64
		ok   bool
	}
	var dbs []db
	var total float64
	hidden := 0
	for _, row := range p.rows() {
		s, ok := number(row["size_bytes"])
		dbs = append(dbs, db{cell(row["datname"]), s, ok})
		if ok {
			total += s
		} else {
			hidden++
		}
	}
	sort.SliceStable(dbs, func(i, j int) bool {
		a, b := dbs[i], dbs[j]
		if a.ok != b.ok {
			return a.ok
		}
		if a.size != b.size {
			return a.size > b.size
		}
		return a.name < b.name
	})
	fmt.Fprintf(w, "\ninst.databases: %d", len(dbs))
	if len(dbs) > hidden {
		fmt.Fprintf(w, ", total %s", size(total))
	}
	if hidden > 0 {
		fmt.Fprintf(w, "  (size of %d not visible)", hidden)
	}
	fmt.Fprintln(w)
	nameW, sizeW := 0, 0
	sizes := make([]string, len(dbs))
	for i, d := range dbs {
		sizes[i] = "-"
		if d.ok {
			sizes[i] = size(d.size)
		}
		nameW = max(nameW, len([]rune(d.name)))
		sizeW = max(sizeW, len(sizes[i]))
	}
	for i, d := range dbs {
		fmt.Fprintf(w, "  %s%s%*s\n", d.name, strings.Repeat(" ", nameW-len([]rune(d.name))+2), sizeW, sizes[i])
	}
	return nil
}

// writeDisk follows df: the percentage is of what non-root users can fill
// (used + avail), not of total, which also counts root's reserve.
func writeDisk(w io.Writer, p Probe) error {
	fmt.Fprintln(w, "\ninst.disk  (filesystem of data_directory)")
	for _, row := range p.rows() {
		total, _ := number(row["total_bytes"])
		used, _ := number(row["used_bytes"])
		avail, _ := number(row["avail_bytes"])
		u, f := size(used), size(avail)
		width := max(len(u), len(f))
		line := fmt.Sprintf("  used  %*s / %s", width, u, size(total))
		if used+avail > 0 {
			line += fmt.Sprintf("  (%.0f%%)", math.Round(used*100/(used+avail)))
		}
		fmt.Fprintln(w, line)
		fmt.Fprintf(w, "  free  %*s\n", width, f)
	}
	return nil
}

// writeKV aligns values after the widest key, or after width if wider.
func writeKV(w io.Writer, width int, kv [][2]string) {
	for _, x := range kv {
		width = max(width, len(x[0]))
	}
	for _, x := range kv {
		fmt.Fprintf(w, "  %-*s  %s\n", width, x[0], x[1])
	}
}

// rows returns every row as a column → value map.
func (p Probe) rows() []map[string]any {
	out := make([]map[string]any, len(p.Rows))
	for i, r := range p.Rows {
		m := map[string]any{}
		for j, c := range p.Columns {
			if j < len(r) {
				m[c] = r[j]
			}
		}
		out[i] = m
	}
	return out
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// number reads any numeric cell, through a pointer; NULL is not a number.
func number(v any) (float64, bool) {
	if isNil(v) {
		return 0, false
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	switch {
	case rv.CanInt():
		return float64(rv.Int()), true
	case rv.CanUint():
		return float64(rv.Uint()), true
	case rv.CanFloat():
		return rv.Float(), true
	}
	return 0, false
}

// size matches pg_size_pretty: 1024 steps, the next unit only once the
// number reaches 10240, rounded half up.
func size(b float64) string {
	units := []string{"bytes", "kB", "MB", "GB", "TB", "PB"}
	i := 0
	for math.Abs(math.Round(b)) >= 10240 && i < len(units)-1 {
		b /= 1024
		i++
	}
	return fmt.Sprintf("%.0f %s", math.Round(b), units[i])
}

// duration keeps the two largest units: 5d 20h, 3m 7s, 8s.
func duration(s float64) string {
	n := int64(s)
	if n < 0 {
		n = 0
	}
	parts := []struct {
		v    int64
		unit string
	}{{n / 86400, "d"}, {n % 86400 / 3600, "h"}, {n % 3600 / 60, "m"}, {n % 60, "s"}}
	for i, p := range parts {
		if p.v == 0 && i < len(parts)-1 {
			continue
		}
		if i == len(parts)-1 {
			return fmt.Sprintf("%d%s", p.v, p.unit)
		}
		return fmt.Sprintf("%d%s %d%s", p.v, p.unit, parts[i+1].v, parts[i+1].unit)
	}
	return "0s"
}

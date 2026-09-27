package report

import (
	"fmt"
	"io"
	"math"
	"math/big"
)

// Shared text formatting of the v0.2 commands.

// pct is n of total, rounded; "-" when there is no total.
func pct(n, total int64) string {
	if total <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", math.Round(float64(n)*100/float64(total)))
}

func behind(b *int64) string {
	if b == nil {
		return "-"
	}
	return size(max(0, float64(*b)))
}

// count is exact below a million, three significant digits above.
func count(n *big.Int) string {
	if n.IsInt64() && n.Int64() < 1_000_000 {
		return n.String()
	}
	f, _ := new(big.Float).SetInt(n).Float64()
	return fmt.Sprintf("%.3g", f)
}

// execTime shows statement times the way they are read: 0.09 ms, 503 ms,
// 1.75 s, then the two-unit durations past a minute.
// The unit is picked after rounding, so 0.9996 s reads 1.00 s, not 1000 ms.
func execTime(s float64) string {
	ms := math.Round(s*1e5) / 100 // to 0.01 ms
	switch {
	case math.Round(s*100)/100 >= 60:
		return duration(s)
	case math.Round(ms) >= 1000:
		return fmt.Sprintf("%.2f s", math.Round(s*100)/100)
	case ms >= 10:
		return fmt.Sprintf("%.0f ms", math.Round(ms))
	}
	return fmt.Sprintf("%.2f ms", ms)
}

// bytesCell is a size in readable units, "-" for NULL.
func bytesCell(v any) string {
	if b, ok := number(v); ok {
		return size(b)
	}
	return "-"
}

// ago is "never" for NULL, else how long ago.
func ago(s *float64) string {
	if s == nil {
		return "never"
	}
	return duration(*s) + " ago"
}

func running(v any) string {
	if s, ok := number(v); ok {
		return duration(s)
	}
	return "?"
}

// relkind names sys_class.relkind for a reader.
func relkind(k string) string {
	if n, ok := map[string]string{"r": "table", "p": "partitioned", "m": "matview", "t": "toast", "i": "index", "I": "partitioned index", "v": "view", "S": "sequence", "f": "foreign"}[k]; ok {
		return n
	}
	return k
}

func reasonOf(p Probe) string {
	if p.Reason == nil {
		return ""
	}
	return *p.Reason
}

func writeTruncated(w io.Writer, n int) {
	if n > 0 {
		fmt.Fprintf(w, "  ... %d more not shown (--limit 0 shows all)\n", n)
	}
}

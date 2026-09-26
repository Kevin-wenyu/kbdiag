// Package units formats sizes and durations for people: the text output
// and the finding symptoms use the same units. JSON keeps bytes and seconds.
package units

import (
	"fmt"
	"math"
)

// Bytes matches pg_size_pretty: 1024 steps, the next unit only once the
// number reaches 10240, rounded half up.
func Bytes(b float64) string {
	units := []string{"bytes", "kB", "MB", "GB", "TB", "PB"}
	i := 0
	for math.Abs(math.Round(b)) >= 10240 && i < len(units)-1 {
		b /= 1024
		i++
	}
	return fmt.Sprintf("%.0f %s", math.Round(b), units[i])
}

// Duration keeps the two largest units: 5d 20h, 3m 7s, 8s. Seconds are
// rounded, as the findings' "%.0fs" are, so text and finding agree.
func Duration(s float64) string {
	n := int64(math.Round(s))
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

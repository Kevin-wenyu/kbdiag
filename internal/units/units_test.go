package units

import (
	"strings"
	"testing"
)

// Bytes agrees with pg_size_pretty, including where it switches units.
func TestBytes(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0 bytes"}, {10239, "10239 bytes"}, {10240, "10 kB"}, {10239 * 1024, "10239 kB"}, {10240*1024 - 1, "10 MB"},
		{10240 * 1024, "10 MB"}, {340459571, "325 MB"}, {400819359, "382 MB"}, {15614003, "15 MB"},
		{15089946624, "14 GB"}, {213452304384, "199 GB"}, {198362357760, "185 GB"},
		{1.5 * (1 << 20), "1536 kB"}, {10.5 * (1 << 30), "11 GB"}, {1 << 60, "1024 PB"},
	}
	for _, c := range cases {
		if got := Bytes(c.in); got != c.want {
			t.Errorf("Bytes(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDuration(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0s"}, {0.4, "0s"}, {0.5, "1s"}, {0.9, "1s"}, {8.0, "8s"}, {59.4, "59s"}, {59.9, "1m 0s"}, {60, "1m 0s"}, {187, "3m 7s"}, {3600, "1h 0m"},
		{3661, "1h 1m"}, {86399, "23h 59m"}, {86400, "1d 0h"}, {268991, "3d 2h"}, {504535, "5d 20h"},
		{400 * 86400, "400d 0h"}, {-5, "0s"}, {-0.4, "0s"},
	}
	for _, c := range cases {
		if got := Duration(c.in); got != c.want {
			t.Errorf("Duration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Any input formats without panicking, and the result keeps its shape.
func FuzzBytes(f *testing.F) {
	for _, v := range []float64{0, -1, 10239, 10240, 1 << 60, 1e300, -1e300} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v float64) {
		s := Bytes(v)
		if s == "" || !strings.Contains(s, " ") {
			t.Errorf("Bytes(%v) = %q", v, s)
		}
	})
}

func FuzzDuration(f *testing.F) {
	for _, v := range []float64{0, -1, 0.5, 59.5, 86400, 1e12, -1e12} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v float64) {
		if s := Duration(v); s == "" || strings.HasPrefix(s, "-") {
			t.Errorf("Duration(%v) = %q", v, s)
		}
	})
}

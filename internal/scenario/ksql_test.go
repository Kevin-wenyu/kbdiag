package scenario

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ksqlCapture is one stage 0 capture of the v0.2 plan
// (e2e/testdata/captures/v02): a comment line with the SQL, then
// `ksql -X -A -F'|' -P null='<NULL>'` output, then EXIT_CODE=n. A failed
// query leaves its error line in err and no rows.
type ksqlCapture struct {
	cols []string
	rows []map[string]*string
	err  string
}

func loadKsql(t *testing.T, name string) ksqlCapture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "e2e", "testdata", "captures", "v02", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	var c ksqlCapture
	for i, l := range lines[1:] {
		switch {
		case strings.HasPrefix(l, "ERROR:"):
			c.err = strings.TrimSpace(strings.TrimPrefix(l, "ERROR:"))
			return c
		case strings.HasPrefix(l, "EXIT_CODE="), strings.HasPrefix(l, "("):
			return c
		case i == 0:
			c.cols = strings.Split(l, "|")
		default:
			f := strings.Split(l, "|")
			m := map[string]*string{}
			for j, col := range c.cols {
				if j < len(f) && f[j] != "<NULL>" {
					v := f[j]
					m[col] = &v
				} else if j < len(f) {
					m[col] = nil
				}
			}
			c.rows = append(c.rows, m)
		}
	}
	return c
}

func kI64(t *testing.T, s *string) *int64 {
	t.Helper()
	if s == nil {
		return nil
	}
	n, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(*s, 64) // float4 prints as 1e+06
		if ferr != nil {
			t.Fatal(err)
		}
		n = int64(f)
	}
	return &n
}

func kF64(t *testing.T, s *string) *float64 {
	t.Helper()
	if s == nil {
		return nil
	}
	f, err := strconv.ParseFloat(*s, 64)
	if err != nil {
		t.Fatal(err)
	}
	return &f
}

// kBool reads ksql's t/f.
func kBool(s *string) bool { return s != nil && (*s == "t" || *s == "true") }

// kI64OrNil is kI64 for cells that may not be numbers (settings like "on").
func kI64OrNil(s *string) *int64 {
	if s == nil {
		return nil
	}
	n, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

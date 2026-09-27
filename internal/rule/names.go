package rule

import (
	"strings"
	"unicode"
)

// keywords are the PG12 keywords quote_ident quotes (every category but
// unreserved: reserved, type/function names, column names).
var keywords = map[string]bool{}

func init() {
	for _, k := range strings.Fields(`all analyse analyze and any array as asc asymmetric authorization binary both case cast check
		collate collation column concurrently constraint create cross current_catalog current_date current_role current_schema
		current_time current_timestamp current_user default deferrable desc distinct do else end except false fetch for foreign
		freeze from full grant group having ilike in initially inner intersect into is isnull join lateral leading left like limit
		localtime localtimestamp natural not notnull null offset on only or order outer overlaps placing primary references
		returning right select session_user similar some symmetric table tablesample then to trailing true union unique user
		using variadic verbose when where window with
		between bigint bit boolean char character coalesce dec decimal exists extract float greatest grouping inout int integer
		interval least national nchar none nullif numeric out overlay position precision real row setof smallint substring time
		timestamp treat trim values varchar xmlattributes xmlconcat xmlelement xmlexists xmlforest xmlnamespaces xmlparse xmlpi
		xmlroot xmlserialize xmltable`) {
		keywords[k] = true
	}
}

// quoteIdent quotes an identifier for a fix SQL the way quote_ident does:
// names that are not plain lower-case identifiers, and keywords, get double
// quotes.
func quoteIdent(s string) string {
	plain := s != "" && !keywords[s]
	for i, r := range s {
		if !(r >= 'a' && r <= 'z' || r == '_' || i > 0 && (r >= '0' && r <= '9' || r == '$')) {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// qualified is schema.relation, each part quoted when it needs it.
func qualified(schema, rel string) string { return quoteIdent(schema) + "." + quoteIdent(rel) }

// hasControl reports a name the text shows escaped (the same characters
// report escapes: controls, format characters such as bidi overrides and
// zero-width spaces, line and paragraph separators): a command or fix SQL
// built from it would not match once pasted, so the reader is sent to
// --json instead.
func hasControl(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
	}) >= 0
}

// parseBool reads a boolean the way the server's parse_bool does: any
// unique prefix of true/false/yes/no (on/off need two letters), 1 or 0,
// case-insensitive, surrounding spaces ignored.
func parseBool(s string) (value, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false, false
	}
	for _, x := range []struct {
		word string
		min  int
		v    bool
	}{{"true", 1, true}, {"false", 1, false}, {"yes", 1, true}, {"no", 1, false}, {"on", 2, true}, {"off", 2, false}, {"1", 1, true}, {"0", 1, false}} {
		if len(s) >= x.min && len(s) <= len(x.word) && strings.HasPrefix(x.word, s) {
			if x.word == "on" || x.word == "off" {
				if s == "o" {
					return false, false
				}
			}
			return x.v, true
		}
	}
	return false, false
}

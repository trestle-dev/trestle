package query

import (
	"strings"
	"testing"
)

func FuzzParseCompile(f *testing.F) {
	for _, seed := range []string{"", `title = "hello"`, `count >= 3 && active = true`, `x ~ "%' OR 1=1 --"`, strings.Repeat("a", MaxExpressionBytes+1)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		expr, err := Parse(input)
		if err != nil {
			return
		}
		sql, args, err := Compile(expr, map[string]Field{
			"title":  {Column: "c_title", Type: "text"},
			"count":  {Column: "c_count", Type: "number"},
			"active": {Column: "c_active", Type: "boolean"},
		})
		if err != nil {
			return
		}
		if strings.Count(sql, "?") != len(args) {
			t.Fatalf("placeholder mismatch: %q, %d args", sql, len(args))
		}
		if strings.Contains(sql, input) && input != "" {
			t.Fatalf("raw input reached SQL: %q", sql)
		}
	})
}

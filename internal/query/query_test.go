package query

import (
	"strings"
	"testing"
)

func TestParseAndCompileUsesParameters(t *testing.T) {
	expr, err := Parse(`title ~ "x' OR 1=1 --" && score >= 4`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := Compile(expr, map[string]Field{"title": {Column: "field_title", Type: "text"}, "score": {Column: "field_score", Type: "number"}})
	if err != nil || strings.Contains(sql, "OR 1=1") || len(args) != 2 {
		t.Fatalf("sql=%q args=%#v err=%v", sql, args, err)
	}
}

func TestLimitsAndTypes(t *testing.T) {
	if _, err := Parse(strings.Repeat("x", MaxExpressionBytes+1)); err == nil {
		t.Fatal("long filter accepted")
	}
	expr, _ := Parse(`done > true`)
	if _, _, err := Compile(expr, map[string]Field{"done": {Column: "done", Type: "boolean"}}); err == nil {
		t.Fatal("invalid boolean operator accepted")
	}
	if _, err := Parse(`title = "x" && title = "x" && title = "x" && title = "x" && title = "x" && title = "x" && title = "x" && title = "x" && title = "x"`); err == nil {
		t.Fatal("clause bound not enforced")
	}
}

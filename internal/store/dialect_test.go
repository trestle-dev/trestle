package store

import "testing"

func TestRebindPostgresPreservesLiteralsAndComments(t *testing.T) {
	input := "SELECT '?', \"?\" FROM x WHERE a=? -- ?\nAND b=? /* ? */"
	want := "SELECT '?', \"?\" FROM x WHERE a=$1 -- ?\nAND b=$2 /* ? */"
	got, err := RebindPostgres(input)
	if err != nil || got != want {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := RebindPostgres("SELECT $1, ?"); err == nil {
		t.Fatal("accepted mixed placeholders")
	}
	if _, err := RebindPostgres("SELECT 'unterminated"); err == nil {
		t.Fatal("accepted unterminated literal")
	}
}

func FuzzRebindPostgres(f *testing.F) {
	for _, seed := range []string{"SELECT ?", "SELECT '?'", "-- ?\n?", "/* ? */ ?", `SELECT "?"`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, query string) { _, _ = RebindPostgres(query) })
}

func TestIdentifierAndErrorContracts(t *testing.T) {
	if got, err := QuoteIdentifier("col_123"); err != nil || got != `"col_123"` {
		t.Fatalf("got=%q err=%v", got, err)
	}
	for _, bad := range []string{"user input", "x;DROP", "a.b", ""} {
		if _, err := QuoteIdentifier(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

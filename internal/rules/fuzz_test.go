package rules

import "testing"

func FuzzRuleValidationAndEvaluation(f *testing.F) {
	for _, seed := range []string{"true", "false", `actor.kind == "user"`, "actor.id == record.owner", "actor.id == input.owner", "actor.id == record.__proto__", ""} {
		f.Add(seed, "user", "usr_1", "usr_1")
	}
	f.Fuzz(func(t *testing.T, expression, kind, actorID, owner string) {
		err := Validate(expression)
		allowed := Evaluate(expression, Actor{Kind: kind, ID: actorID}, map[string]any{"owner": owner})
		if err != nil && allowed {
			t.Fatalf("invalid expression evaluated true: %q", expression)
		}
	})
}

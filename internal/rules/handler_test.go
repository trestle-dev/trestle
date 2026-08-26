package rules

import "testing"

func TestRuleValidationAndEvaluation(t *testing.T) {
	valid := []string{"true", "false", `actor.kind == "user"`, `actor.kind == "service"`, "actor.id == record.owner", "actor.id == input.owner"}
	for _, expr := range valid {
		if err := Validate(expr); err != nil {
			t.Fatalf("%q: %v", expr, err)
		}
	}
	if Validate("actor.id == record.owner; DROP TABLE x") == nil {
		t.Fatal("injection-like rule accepted")
	}
	actor := Actor{ID: "usr_123", Kind: "user"}
	if !Evaluate(`actor.kind == "user"`, actor, nil) || Evaluate(`actor.kind == "service"`, actor, nil) {
		t.Fatal("kind evaluation")
	}
	if !Evaluate("actor.id == record.owner", actor, map[string]any{"owner": "usr_123"}) || Evaluate("actor.id == record.owner", actor, map[string]any{"owner": "usr_other"}) {
		t.Fatal("owner evaluation")
	}
}
func TestExplanationRedaction(t *testing.T) {
	if got := redact("usr_sensitive_identifier"); got == "usr_sensitive_identifier" || got != "usr…er" {
		t.Fatalf("redaction %q", got)
	}
}

package httperr

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnvelopeContract(t *testing.T) {
	body := New("validation_failed", "The request could not be applied.", "req_test", Field{
		Path: "title",
		Code: "required",
	})
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"validation_failed", "req_test", "title", "required"} {
		if !strings.Contains(got, want) {
			t.Fatalf("encoded error %q does not contain %q", got, want)
		}
	}
}

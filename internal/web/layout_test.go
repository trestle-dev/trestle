package web

import (
	"strings"
	"testing"
)

func TestFixedViewportContractIsPresent(t *testing.T) {
	css, err := embedded.ReadFile("public/assets/css/style.css")
	if err != nil {
		t.Fatal(err)
	}
	source := string(css)
	for _, required := range []string{"html,body", "overflow:hidden", "height:100dvh", ".workspace", "min-width:0", "min-height:0", ".pane-scroll", "overflow:auto"} {
		if !strings.Contains(source, required) {
			t.Errorf("dashboard CSS is missing %q", required)
		}
	}
}

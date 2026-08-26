package webhooks

import "testing"

func TestTargetConfigurationDoesNotRequireLiveDNS(t *testing.T) {
	if _, err := validateTargetURL("https://receiver.invalid/trestle"); err != nil {
		t.Fatalf("offline target configuration: %v", err)
	}
	for _, target := range []string{"http://example.com/hook", "https://user@example.com/hook", "not a URL"} {
		if _, err := validateTargetURL(target); err == nil {
			t.Fatalf("accepted unsafe target %q", target)
		}
	}
}

func TestDeliveryStillRefusesPrivateDestination(t *testing.T) {
	if err := safeDestination("https://127.0.0.1/hook"); err == nil {
		t.Fatal("private delivery destination accepted")
	}
}

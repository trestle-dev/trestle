package webhooks

import "testing"

func FuzzTargetURLValidation(f *testing.F) {
	for _, seed := range []string{"https://example.com/hook", "http://127.0.0.1", "file:///etc/passwd", "https://[::1]/", "https://user:pass@example.com/", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, target string) {
		u, err := validateTargetURL(target)
		if err == nil && (u.Scheme != "https" || u.Hostname() == "" || u.User != nil) {
			t.Fatalf("unsafe target accepted: %#v", u)
		}
	})
}

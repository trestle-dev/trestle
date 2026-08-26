package buildinfo

import "testing"

func TestCurrentHasDevelopmentIdentity(t *testing.T) {
	info := Current()
	if info.Version == "" || info.Commit == "" || info.Date == "" {
		t.Fatalf("incomplete build information: %#v", info)
	}
	if info.Go == "" || info.OS == "" || info.Arch == "" {
		t.Fatalf("incomplete runtime information: %#v", info)
	}
}

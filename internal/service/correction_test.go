package service

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStrictService returns a setup whose fake runner fails on any unconfigured
// systemd invocation.
func newStrictService(t *testing.T) *fakeRunner {
	t.Helper()
	r := setupService(t)
	r.strict = true
	return r
}

type stateMatrixEntry struct {
	name          string
	enabled       string
	active        string
	accepted      bool
	wantEnableSeq []string
	wantActive    string
}

func TestInstallStateMatrix(t *testing.T) {
	matrix := []stateMatrixEntry{
		{name: "enabled+active", enabled: "enabled", active: "active", accepted: true, wantEnableSeq: []string{"systemctl enable trestle.service"}, wantActive: "restart"},
		{name: "enabled+inactive", enabled: "enabled", active: "inactive", accepted: true, wantEnableSeq: []string{"systemctl enable trestle.service"}, wantActive: "stop"},
		{name: "enabled-runtime+active", enabled: "enabled-runtime", active: "active", accepted: true, wantEnableSeq: []string{"systemctl enable --runtime trestle.service"}, wantActive: "restart"},
		{name: "enabled-runtime+inactive", enabled: "enabled-runtime", active: "inactive", accepted: true, wantEnableSeq: []string{"systemctl enable --runtime trestle.service"}, wantActive: "stop"},
		{name: "disabled+active", enabled: "disabled", active: "active", accepted: true, wantEnableSeq: []string{}, wantActive: "restart"},
		{name: "disabled+inactive", enabled: "disabled", active: "inactive", accepted: true, wantEnableSeq: []string{}, wantActive: "stop"},
		{name: "masked+active", enabled: "masked", active: "active", accepted: false},
		{name: "masked-runtime+inactive", enabled: "masked-runtime", active: "inactive", accepted: false},
		{name: "static+inactive", enabled: "static", active: "inactive", accepted: false},
		{name: "linked+inactive", enabled: "linked", active: "inactive", accepted: false},
		{name: "generated+inactive", enabled: "generated", active: "inactive", accepted: false},
		{name: "transient+inactive", enabled: "transient", active: "inactive", accepted: false},
		{name: "failed+inactive", enabled: "failed", active: "inactive", accepted: false},
		{name: "enabled+reloading", enabled: "enabled", active: "reloading", accepted: false},
		{name: "enabled+activating", enabled: "enabled", active: "activating", accepted: false},
	}
	for _, tc := range matrix {
		t.Run(tc.name, func(t *testing.T) {
			r := newStrictService(t)
			installManagedUnit(t)
			setState(r, tc.enabled, tc.active)
			exe := filepath.Join(t.TempDir(), "tr")
			os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755)
			if tc.accepted {
				r.script["systemctl daemon-reload"] = fakeResult{}
				r.script["systemctl enable trestle.service"] = fakeResult{out: "failed to enable", code: 1}
				r.script["systemctl restart trestle.service"] = fakeResult{}
				r.script["systemctl stop trestle.service"] = fakeResult{}
				r.script["systemctl disable trestle.service"] = fakeResult{}
				if e := Install(exe, "/var/lib/trestle", "127.0.0.1:9090", ""); e == nil {
					t.Fatalf("install with activation failure should return an error")
				}
				for _, want := range tc.wantEnableSeq {
					if !contains(r.log, want) {
						t.Fatalf("rollback did not restore enablement %q: log=%v", tc.enabled, r.log)
					}
				}
				if tc.wantEnableSeq == nil {
					if contains(r.log, "systemctl enable --runtime trestle.service") || contains(r.log, "systemctl enable trestle.service") {
						t.Fatalf("disabled prior should not be re-enabled: log=%v", r.log)
					}
				}
				if tc.wantActive == "restart" && !contains(r.log, "systemctl restart trestle.service") {
					t.Fatalf("active prior not restarted on rollback: log=%v", r.log)
				}
				if tc.wantActive == "stop" && !contains(r.log, "systemctl stop trestle.service") {
					t.Fatalf("inactive prior not stopped on rollback: log=%v", r.log)
				}
				b, _ := os.ReadFile(UnitPath)
				if !strings.Contains(string(b), "127.0.0.1:8090") {
					t.Fatalf("prior unit not restored: log=%v", r.log)
				}
			} else {
				beforeBin, _ := os.ReadFile(BinaryPath)
				if e := Install(exe, "/var/lib/trestle", "127.0.0.1:9090", ""); e == nil {
					t.Fatalf("install of non-restorable state %s succeeded", tc.name)
				}
				afterBin, _ := os.ReadFile(BinaryPath)
				if !bytes.Equal(beforeBin, afterBin) {
					t.Fatalf("rejected state %s still mutated the binary", tc.name)
				}
				for _, call := range r.log {
					if strings.HasPrefix(call, "systemctl enable ") || strings.HasPrefix(call, "systemctl disable ") ||
						strings.HasPrefix(call, "systemctl start ") || strings.HasPrefix(call, "systemctl stop ") ||
						strings.HasPrefix(call, "systemctl restart ") || call == "systemctl daemon-reload" {
						t.Fatalf("rejected state %s mutated lifecycle before refusal: %s", tc.name, call)
					}
				}
			}
		})
	}
}

func TestInstallStateQueryFailureAborts(t *testing.T) {
	r := newStrictService(t)
	installManagedUnit(t)
	beforeBin, _ := os.ReadFile(BinaryPath)
	r.script["systemctl is-enabled trestle.service"] = fakeResult{out: "", code: 1, err: fmt.Errorf("is-enabled failed")}
	exe := filepath.Join(t.TempDir(), "tr")
	os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	if e := Install(exe, "/var/lib/trestle", "127.0.0.1:9090", ""); e == nil {
		t.Fatal("install proceeded despite is-enabled query failure")
	}
	afterBin, _ := os.ReadFile(BinaryPath)
	if !bytes.Equal(beforeBin, afterBin) {
		t.Fatal("state-query failure still mutated the binary")
	}
}

func TestInstallRollbackSurfacesRecoveryFailure(t *testing.T) {
	r := newStrictService(t)
	installManagedUnit(t)
	setState(r, "enabled", "active")
	r.seq["systemctl daemon-reload"] = []fakeResult{{}, {out: "reload failed", code: 1}}
	r.script["systemctl enable trestle.service"] = fakeResult{out: "failed to enable", code: 1}
	r.script["systemctl restart trestle.service"] = fakeResult{out: "activation failed", code: 1}
	r.script["systemctl stop trestle.service"] = fakeResult{}
	r.script["systemctl disable trestle.service"] = fakeResult{}
	exe := filepath.Join(t.TempDir(), "tr")
	os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	e := Install(exe, "/var/lib/trestle", "127.0.0.1:9090", "")
	if e == nil {
		t.Fatal("install succeeded despite activation failure")
	}
	if !strings.Contains(e.Error(), "rollback incomplete") {
		t.Fatalf("install did not surface the rollback failure: %v", e)
	}
	if !strings.Contains(e.Error(), "reload systemd") {
		t.Fatalf("install did not surface the rollback root cause: %v", e)
	}
}

func TestEnvFileRequiresRootOwnership(t *testing.T) {
	setupService(t)
	dir := t.TempDir()
	env := filepath.Join(dir, "trestle.env")
	os.WriteFile(env, []byte("TRESTLE_DATABASE_URL=postgres://x\n"), 0o600)
	oldUID := fileUID
	fileUID = func(os.FileInfo) int { return 0 }
	defer func() { fileUID = oldUID }()
	if e := validateEnvFile(env); e != nil {
		t.Fatalf("root-owned 0600 env file rejected: %v", e)
	}
	fileUID = func(os.FileInfo) int { return 4242 }
	if e := validateEnvFile(env); e == nil {
		t.Fatal("service-user-owned 0600 env file accepted")
	} else if !strings.Contains(e.Error(), "root") {
		t.Fatalf("owner rejection lacks root diagnostic: %v", e)
	}
	os.Chmod(env, 0o640)
	fileUID = func(os.FileInfo) int { return 0 }
	if e := validateEnvFile(env); e == nil {
		t.Fatal("root-owned 0640 env file accepted")
	}
	os.Chmod(env, 0o600)
	link := filepath.Join(dir, "link.env")
	if e := os.Symlink(env, link); e != nil {
		t.Fatal(e)
	}
	fileUID = func(os.FileInfo) int { return 0 }
	if e := validateEnvFile(link); e == nil {
		t.Fatal("symlink env file accepted")
	}
}

func TestDataDirRejectsSystemRoots(t *testing.T) {
	r := newStrictService(t)
	for _, root := range []string{"/", "/etc", "/usr", "/var", "/home"} {
		exe := filepath.Join(t.TempDir(), "tr")
		os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755)
		if e := Install(exe, root, "127.0.0.1:8090", ""); e == nil {
			t.Fatalf("install accepted system data directory %q", root)
		}
		if len(r.log) != 0 {
			t.Fatalf("install of %q touched systemctl", root)
		}
	}
}

func TestDataDirRefusesUnrelatedExistingDirectory(t *testing.T) {
	r := newStrictService(t)
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing-data")
	os.MkdirAll(existing, 0o755)
	marker := filepath.Join(existing, "sentinel")
	os.WriteFile(marker, []byte("keep"), 0o644)
	oldOwned := requireServiceOwned
	requireServiceOwned = func(path string) error { return fmt.Errorf("owned by UID 1000") }
	defer func() { requireServiceOwned = oldOwned }()
	exe := filepath.Join(t.TempDir(), "tr")
	os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	if e := Install(exe, existing, "127.0.0.1:8090", ""); e == nil {
		t.Fatal("install adopted an unrelated existing directory")
	}
	if b, e := os.ReadFile(marker); e != nil || string(b) != "keep" {
		t.Fatalf("existing directory content mutated: %q %v", b, e)
	}
	info, _ := os.Lstat(existing)
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("existing directory mode mutated to %v", info.Mode().Perm())
	}
	if len(r.log) != 0 {
		t.Fatalf("rejected existing directory still ran systemctl: %v", r.log)
	}
}

func TestDataDirNewLeafAdopted(t *testing.T) {
	r := newStrictService(t)
	dir := t.TempDir()
	newData := filepath.Join(dir, "leaf", "trestle-data")
	exe := filepath.Join(t.TempDir(), "tr")
	os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	oldChown := chownData
	chowned := ""
	chownData = func(p string) error { chowned = p; return nil }
	defer func() { chownData = oldChown }()
	r.script["systemctl daemon-reload"] = fakeResult{}
	r.script["systemctl enable trestle.service"] = fakeResult{}
	r.script["systemctl restart trestle.service"] = fakeResult{}
	if e := Install(exe, newData, "127.0.0.1:8090", ""); e != nil {
		t.Fatal(e)
	}
	if chowned != newData {
		t.Fatalf("leaf data dir not handed to the service account: chowned=%q", chowned)
	}
}

func TestRecoveryTimeMarkerProvesTiming(t *testing.T) {
	r := newStrictService(t)
	installManagedUnit(t)
	setState(r, "enabled", "active")
	prepareUpdate(r, "active", false)
	healthWindow = 1
	exe := filepath.Join(t.TempDir(), "tr2")
	newBin := []byte("#!/bin/sh\n# v2\nexit 0\n")
	os.WriteFile(exe, newBin, 0o755)
	r.script["systemctl restart trestle.service"] = fakeResult{}
	r.script["systemctl stop trestle.service"] = fakeResult{}
	var seamSaw struct {
		markerWritten bool
		binaryIsNew   bool
		logLenAtSeam  int
	}
	orig := priorStateFileRead
	priorStateFileRead = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, ".prior-active") {
			if _, e := os.Stat(BinaryPath + ".prior-active"); e == nil {
				seamSaw.markerWritten = true
			}
			cur, _ := os.ReadFile(BinaryPath)
			seamSaw.binaryIsNew = bytes.Equal(cur, newBin)
			seamSaw.logLenAtSeam = len(r.log)
			stopCount, restartCount := 0, 0
			for _, c := range r.log {
				if c == "systemctl stop trestle.service" {
					stopCount++
				}
				if c == "systemctl restart trestle.service" {
					restartCount++
				}
			}
			if stopCount != 0 {
				t.Fatalf("recovery issued stop before reading the marker (log=%v)", r.log)
			}
			if restartCount != 1 {
				t.Fatalf("expected exactly the update's restart before recovery marker read, got %d (log=%v)", restartCount, r.log)
			}
			return nil, fmt.Errorf("marker vanished at recovery time")
		}
		return os.ReadFile(path)
	}
	defer func() { priorStateFileRead = orig }()
	uerr := Update(exe, fakeSHA(exe))
	if uerr == nil {
		t.Fatal("update succeeded despite recovery marker failure")
	}
	if !seamSaw.markerWritten {
		t.Fatal("recovery seam fired but Update never wrote the marker; test is not exercising recovery-time")
	}
	if !seamSaw.binaryIsNew {
		t.Fatal("recovery seam fired but the installed binary is not the new one; timing wrong")
	}
	if seamSaw.logLenAtSeam == 0 {
		t.Fatal("recovery seam did not record a log position")
	}
	if !strings.Contains(uerr.Error(), "recovery") {
		t.Fatalf("recovery fail-closed not surfaced: %v", uerr)
	}
	for _, c := range r.log[seamSaw.logLenAtSeam:] {
		if strings.HasPrefix(c, "systemctl ") {
			t.Fatalf("recovery issued a lifecycle verb after the failed marker read: %s", c)
		}
	}
}

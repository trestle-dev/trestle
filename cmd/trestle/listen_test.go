package main

import (
	"flag"
	"os"
	"testing"
)

// listenerFlags parses raw argv through the same flag definitions the CLI uses
// so "provided but empty" (--host "") is distinguishable from "not provided".
func listenerFlags(args ...string) (h, p, l string, hs, ps, ls bool) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("host", "", "")
	fs.String("port", "", "")
	fs.String("listen", "", "")
	_ = fs.Parse(args)
	return fs.Lookup("host").Value.String(),
		fs.Lookup("port").Value.String(),
		fs.Lookup("listen").Value.String(),
		flagProvided(fs, "host"),
		flagProvided(fs, "port"),
		flagProvided(fs, "listen")
}

func TestResolveListenerDefaults(t *testing.T) {
	os.Unsetenv("TRESTLE_HOST")
	os.Unsetenv("TRESTLE_PORT")
	os.Unsetenv("TRESTLE_LISTEN")
	h, p, l, hs, ps, ls := listenerFlags()
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:7333" {
		t.Fatalf("default listener = %q want 127.0.0.1:7333", addr)
	}
}

func TestResolveListenerEnvOnly(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	t.Setenv("TRESTLE_HOST", "0.0.0.0")
	t.Setenv("TRESTLE_PORT", "7403")
	h, p, l, hs, ps, ls := listenerFlags()
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "0.0.0.0:7403" {
		t.Fatalf("env listener = %q want 0.0.0.0:7403", addr)
	}
}

func TestResolveListenerCLIOnly(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	t.Setenv("TRESTLE_HOST", "192.0.2.1")
	t.Setenv("TRESTLE_PORT", "9999")
	h, p, l, hs, ps, ls := listenerFlags("--host", "127.0.0.1", "--port", "7403")
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:7403" {
		t.Fatalf("cli listener = %q want 127.0.0.1:7403", addr)
	}
}

func TestResolveListenerCLIOverridesEnv(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	t.Setenv("TRESTLE_HOST", "0.0.0.0")
	t.Setenv("TRESTLE_PORT", "9000")
	h, p, l, hs, ps, ls := listenerFlags("--host", "127.0.0.1", "--port", "7403")
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:7403" {
		t.Fatalf("cli should override env: %q", addr)
	}
}

func TestResolveListenerInvalidPorts(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	for _, p := range []string{"abc", "0", "-5", "65536", "70000", "7 4 0 3", "7403x"} {
		h, pp, l, hs, ps, ls := listenerFlags("--host", "127.0.0.1", "--port", p)
		if _, err := resolveListener(h, pp, l, hs, ps, ls); err == nil {
			t.Fatalf("invalid --port %q accepted", p)
		}
	}
	for _, p := range []string{"abc", "0", "-5", "65536", "70000"} {
		t.Setenv("TRESTLE_HOST", "127.0.0.1")
		t.Setenv("TRESTLE_PORT", p)
		h, pp, l, hs, ps, ls := listenerFlags()
		if _, err := resolveListener(h, pp, l, hs, ps, ls); err == nil {
			t.Fatalf("invalid TRESTLE_PORT %q accepted", p)
		}
	}
}

func TestResolveListenerEmptyEnvFails(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	t.Setenv("TRESTLE_HOST", "127.0.0.1")
	t.Setenv("TRESTLE_PORT", "")
	h, p, l, hs, ps, ls := listenerFlags()
	if _, err := resolveListener(h, p, l, hs, ps, ls); err == nil {
		t.Fatal("empty TRESTLE_PORT accepted")
	}
	t.Setenv("TRESTLE_HOST", "")
	t.Setenv("TRESTLE_PORT", "7333")
	if _, err := resolveListener(h, p, l, hs, ps, ls); err == nil {
		t.Fatal("empty TRESTLE_HOST accepted")
	}
}

func TestResolveListenerEmptyCLIValuesFail(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	t.Setenv("TRESTLE_HOST", "127.0.0.1")
	t.Setenv("TRESTLE_PORT", "7333")
	h, p, l, hs, ps, ls := listenerFlags("--host", "", "--port", "7333")
	if _, err := resolveListener(h, p, l, hs, ps, ls); err == nil {
		t.Fatal("empty --host accepted")
	}
	t.Setenv("TRESTLE_HOST", "127.0.0.1")
	t.Setenv("TRESTLE_PORT", "7333")
	h, p, l, hs, ps, ls = listenerFlags("--host", "127.0.0.1", "--port", "")
	if _, err := resolveListener(h, p, l, hs, ps, ls); err == nil {
		t.Fatal("empty --port accepted")
	}
}

func TestResolveListenerIPv6(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	t.Setenv("TRESTLE_HOST", "::1")
	t.Setenv("TRESTLE_PORT", "7333")
	h, p, l, hs, ps, ls := listenerFlags()
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "[::1]:7333" {
		t.Fatalf("IPv6 listener = %q want [::1]:7333", addr)
	}
}

func TestResolveListenerLegacyListen(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	t.Setenv("TRESTLE_HOST", "0.0.0.0")
	t.Setenv("TRESTLE_PORT", "9000")
	h, p, l, hs, ps, ls := listenerFlags("--listen", "127.0.0.1:8080")
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:8080" {
		t.Fatalf("legacy --listen = %q", addr)
	}
	// TRESTLE_LISTEN environment is honored as the legacy single-address form.
	os.Unsetenv("TRESTLE_HOST")
	os.Unsetenv("TRESTLE_PORT")
	t.Setenv("TRESTLE_LISTEN", "127.0.0.1:9000")
	h, p, l, hs, ps, ls = listenerFlags()
	addr, err = resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:9000" {
		t.Fatalf("TRESTLE_LISTEN = %q", addr)
	}
	// Legacy form combined with --host/--port must fail.
	os.Unsetenv("TRESTLE_LISTEN")
	h2, p2, l2, hs2, ps2, ls2 := listenerFlags("--host", "127.0.0.1", "--port", "7403", "--listen", "127.0.0.1:8080")
	if _, err := resolveListener(h2, p2, l2, hs2, ps2, ls2); err == nil {
		t.Fatal("--listen combined with --host/--port accepted")
	}
}

func TestValidatePort(t *testing.T) {
	for _, ok := range []string{"1", "7333", "65535"} {
		if err := validatePort(ok); err != nil {
			t.Fatalf("valid port %q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "0", "-1", "65536", "7333.5", "x"} {
		if err := validatePort(bad); err == nil {
			t.Fatalf("invalid port %q accepted", bad)
		}
	}
}

func TestResolveListenerTrimsWhitespace(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	h, p, l, hs, ps, ls := listenerFlags("--host", "  127.0.0.1  ", "--port", "  7403  ")
	host, port, err := resolveHostPort(h, p, hs, ps)
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" || port != "7403" {
		t.Fatalf("cli trimmed host/port = %q/%q want 127.0.0.1/7403", host, port)
	}
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:7403" {
		t.Fatalf("cli listener = %q want 127.0.0.1:7403", addr)
	}

	// Whitespace-surrounded environment values must resolve canonically.
	t.Setenv("TRESTLE_HOST", "  0.0.0.0  ")
	t.Setenv("TRESTLE_PORT", "  7404  ")
	h, p, l, hs, ps, ls = listenerFlags()
	host, port, err = resolveHostPort(h, p, hs, ps)
	if err != nil {
		t.Fatal(err)
	}
	if host != "0.0.0.0" || port != "7404" {
		t.Fatalf("env trimmed host/port = %q/%q want 0.0.0.0/7404", host, port)
	}
	addr, err = resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "0.0.0.0:7404" {
		t.Fatalf("env listener = %q want 0.0.0.0:7404", addr)
	}
}

func TestResolveListenerWhitespaceOnlyFails(t *testing.T) {
	os.Unsetenv("TRESTLE_LISTEN")
	h, p, l, hs, ps, ls := listenerFlags("--host", "   ", "--port", "   ")
	if _, err := resolveListener(h, p, l, hs, ps, ls); err == nil {
		t.Fatal("whitespace-only --host/--port accepted")
	}
	t.Setenv("TRESTLE_HOST", "   ")
	t.Setenv("TRESTLE_PORT", "   ")
	h, p, l, hs, ps, ls = listenerFlags()
	if _, err := resolveListener(h, p, l, hs, ps, ls); err == nil {
		t.Fatal("whitespace-only TRESTLE_HOST/TRESTLE_PORT accepted")
	}
	t.Setenv("TRESTLE_HOST", "   ")
	t.Setenv("TRESTLE_PORT", "7403")
	h, p, l, hs, ps, ls = listenerFlags()
	if _, err := resolveListener(h, p, l, hs, ps, ls); err == nil {
		t.Fatal("whitespace-only host accepted with valid port")
	}
}

func TestResolveListenerExplicitPortOverridesLegacyEnv(t *testing.T) {
	os.Unsetenv("TRESTLE_HOST")
	os.Unsetenv("TRESTLE_PORT")
	t.Setenv("TRESTLE_LISTEN", "127.0.0.1:8080")
	h, p, l, hs, ps, ls := listenerFlags("--port", "7403")
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:7403" {
		t.Fatalf("explicit --port must override TRESTLE_LISTEN: %q", addr)
	}
}

func TestResolveListenerExplicitHostOverridesLegacyEnv(t *testing.T) {
	os.Unsetenv("TRESTLE_PORT")
	t.Setenv("TRESTLE_LISTEN", "127.0.0.1:8080")
	t.Setenv("TRESTLE_HOST", "0.0.0.0")
	h, p, l, hs, ps, ls := listenerFlags("--host", "10.0.0.1", "--port", "7403")
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "10.0.0.1:7403" {
		t.Fatalf("explicit --host/--port must override TRESTLE_LISTEN: %q", addr)
	}
}

func TestResolveListenerExplicitListenOverridesHostPortEnv(t *testing.T) {
	t.Setenv("TRESTLE_HOST", "0.0.0.0")
	t.Setenv("TRESTLE_PORT", "9000")
	h, p, l, hs, ps, ls := listenerFlags("--listen", "127.0.0.1:8080")
	addr, err := resolveListener(h, p, l, hs, ps, ls)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:8080" {
		t.Fatalf("explicit --listen must override host/port env: %q", addr)
	}
}

func TestResolveListenerEnvConflict(t *testing.T) {
	// Only environment variables involved: legacy TRESTLE_LISTEN conflicts with
	// TRESTLE_HOST or TRESTLE_PORT and must fail rather than silently pick one.
	t.Setenv("TRESTLE_LISTEN", "127.0.0.1:8080")
	t.Setenv("TRESTLE_HOST", "0.0.0.0")
	h, p, l, hs, ps, ls := listenerFlags()
	if _, err := resolveListener(h, p, l, hs, ps, ls); err == nil {
		t.Fatal("TRESTLE_LISTEN + TRESTLE_HOST conflict accepted")
	}
	os.Unsetenv("TRESTLE_HOST")
	t.Setenv("TRESTLE_PORT", "9000")
	if _, err := resolveListener(h, p, l, hs, ps, ls); err == nil {
		t.Fatal("TRESTLE_LISTEN + TRESTLE_PORT conflict accepted")
	}
}

func TestListenerFlagsFromArgs(t *testing.T) {
	host, port, listen, hs, ps, ls := listenerFlagsFromArgs([]string{"--data-dir", "/x", "--host", "127.0.0.1", "--port", "7403"})
	if host != "127.0.0.1" || port != "7403" || !hs || !ps || ls {
		t.Fatalf("args scan host=%q port=%q hs=%v ps=%v ls=%v", host, port, hs, ps, ls)
	}
	host, port, listen, hs, ps, ls = listenerFlagsFromArgs([]string{"--host=0.0.0.0", "--port=9000", "--listen=127.0.0.1:8080"})
	if host != "0.0.0.0" || port != "9000" || listen != "127.0.0.1:8080" || !hs || !ps || !ls {
		t.Fatalf("args scan host=%q port=%q listen=%q hs=%v ps=%v ls=%v", host, port, listen, hs, ps, ls)
	}
	host, port, listen, hs, ps, ls = listenerFlagsFromArgs([]string{"--listen", "127.0.0.1:8080"})
	if listen != "127.0.0.1:8080" || !ls || hs || ps {
		t.Fatalf("args scan listen=%q ls=%v hs=%v ps=%v", listen, ls, hs, ps)
	}
	// An empty --host= is "provided but empty".
	host, port, _, hs, ps, _ = listenerFlagsFromArgs([]string{"--host=", "--port", "7403"})
	if host != "" || !hs || !ps {
		t.Fatalf("empty flag scan host=%q hs=%v ps=%v", host, hs, ps)
	}
	// A flag with no value token is left unset for the flag package to report.
	host, port, _, hs, ps, _ = listenerFlagsFromArgs([]string{"--host"})
	if hs {
		t.Fatal("--host with no value must not be treated as provided")
	}
}

func TestListenerOverrideSelected(t *testing.T) {
	os.Unsetenv("TRESTLE_HOST")
	os.Unsetenv("TRESTLE_PORT")
	os.Unsetenv("TRESTLE_LISTEN")
	if listenerOverrideSelected(false, false) {
		t.Fatal("bare invocation must not override durable config")
	}
	// TRESTLE_LISTEN only: not an explicit new-form override.
	t.Setenv("TRESTLE_LISTEN", "127.0.0.1:8080")
	if listenerOverrideSelected(false, false) {
		t.Fatal("legacy TRESTLE_LISTEN must not override durable config")
	}
	// TRESTLE_HOST / TRESTLE_PORT environment: override.
	os.Unsetenv("TRESTLE_LISTEN")
	t.Setenv("TRESTLE_HOST", "0.0.0.0")
	if !listenerOverrideSelected(false, false) {
		t.Fatal("TRESTLE_HOST must override durable config")
	}
	os.Unsetenv("TRESTLE_HOST")
	t.Setenv("TRESTLE_PORT", "7403")
	if !listenerOverrideSelected(false, false) {
		t.Fatal("TRESTLE_PORT must override durable config")
	}
	// Explicit CLI --host/--port: override.
	os.Unsetenv("TRESTLE_PORT")
	if !listenerOverrideSelected(true, true) {
		t.Fatal("explicit --host/--port must override durable config")
	}
}

func TestRunServiceInstallResolvesListenerOnly(t *testing.T) {
	// Malformed listener environment in the invoking shell must not break
	// non-install commands: status fails for its own reason, never with the
	// listener error (exit 2).
	t.Setenv("TRESTLE_HOST", "not a host")
	t.Setenv("TRESTLE_PORT", "not-a-port")
	if code := runService([]string{"status"}); code == 2 {
		t.Fatal("status must ignore malformed listener env")
	}
	if code := runService([]string{"logs"}); code == 2 {
		t.Fatal("logs must ignore malformed listener env")
	}
	// install resolves listener flags and environment, so malformed values fail.
	if code := runService([]string{"install", "--port", "abc"}); code != 2 {
		t.Fatalf("install with invalid --port exit=%d want 2", code)
	}
	if code := runService([]string{"install", "--host", "0.0.0.0", "--port", "70000"}); code != 2 {
		t.Fatalf("install with oversized --port exit=%d want 2", code)
	}
	if code := runService([]string{"install", "--listen", "127.0.0.1:8080", "--host", "0.0.0.0"}); code != 2 {
		t.Fatalf("install with --listen+--host conflict exit=%d want 2", code)
	}
	// A valid explicit install resolves and proceeds into the installer (which
	// fails for operational reasons such as non-root), not the usage error path.
	if code := runService([]string{"install", "--host", "127.0.0.1", "--port", "7403"}); code == 2 {
		t.Fatal("valid explicit install must not fail listener resolution")
	}
	if code := runService([]string{"install", "--listen", "127.0.0.1:8080"}); code == 2 {
		t.Fatal("valid legacy install must not fail listener resolution")
	}
}

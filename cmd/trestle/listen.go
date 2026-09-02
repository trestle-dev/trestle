package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Default Trestle host and port used by the shared project configuration
// pattern. Trestle's default listener is 127.0.0.1:7333; both remain
// configurable. An existing durable config keeps the resolved listener once it
// exists, and an explicit host/port selection overrides it in memory.
const (
	defaultHost = "127.0.0.1"
	defaultPort = "7333"
)

// flagProvided reports whether name was explicitly supplied on the command
// line. Standard flag strings cannot otherwise distinguish `--port ""` from an
// absent flag, and the contract requires empty CLI values to fail.
func flagProvided(fs *flag.FlagSet, name string) bool {
	provided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}

// validatePort reports whether p is a valid TCP port: an integer from 1
// through 65535. It never silently falls back to a default.
func validatePort(p string) error {
	n, err := strconv.Atoi(strings.TrimSpace(p))
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("port must be an integer from 1 through 65535; got %q", p)
	}
	return nil
}

// resolveHostPort computes the effective bind host and port. Precedence per
// field is CLI flag > environment variable > default. Values are trimmed once
// and the canonical trimmed form is returned, so surrounding whitespace cannot
// leak into the listener string. A port that is present but empty, malformed,
// zero, negative or greater than 65535 is an error; an empty host value is
// rejected rather than silently meaning "all interfaces".
func resolveHostPort(hostFlag, portFlag string, hostSet, portSet bool) (host, port string, err error) {
	host = defaultHost
	if hostSet {
		host = strings.TrimSpace(hostFlag)
		if host == "" {
			return "", "", errors.New("--host is set but empty")
		}
	} else if v, ok := os.LookupEnv("TRESTLE_HOST"); ok {
		host = strings.TrimSpace(v)
		if host == "" {
			return "", "", errors.New("TRESTLE_HOST is set but empty")
		}
	}
	port = defaultPort
	if portSet {
		if err := validatePort(portFlag); err != nil {
			return "", "", fmt.Errorf("--port: %w", err)
		}
		port = strings.TrimSpace(portFlag)
	} else if v, ok := os.LookupEnv("TRESTLE_PORT"); ok {
		if err := validatePort(v); err != nil {
			return "", "", fmt.Errorf("TRESTLE_PORT: %w", err)
		}
		port = strings.TrimSpace(v)
	}
	return host, port, nil
}

// resolveListener computes the effective HTTP listen address from the CLI
// flags and environment with the contract CLI > environment > default.
//
//   - explicit --listen wins and cannot be combined with explicit --host/--port;
//   - explicit --host and/or --port override the legacy TRESTLE_LISTEN variable;
//   - with only environment variables, legacy TRESTLE_LISTEN conflicts with
//     TRESTLE_HOST/TRESTLE_PORT rather than silently picking one;
//   - otherwise TRESTLE_LISTEN is used, then host/port defaults.
//
// IPv6 hosts are bracketed via net.JoinHostPort.
func resolveListener(hostFlag, portFlag, listenFlag string, hostSet, portSet, listenSet bool) (string, error) {
	if listenSet {
		if hostSet || portSet {
			return "", errors.New("--listen cannot be combined with --host or --port")
		}
		if strings.TrimSpace(listenFlag) == "" {
			return "", errors.New("--listen is set but empty")
		}
		return listenFlag, nil
	}
	// Explicit --host/--port override the legacy TRESTLE_LISTEN variable.
	if hostSet || portSet {
		host, port, err := resolveHostPort(hostFlag, portFlag, hostSet, portSet)
		if err != nil {
			return "", err
		}
		return net.JoinHostPort(host, port), nil
	}
	// Only environment variables are involved: legacy TRESTLE_LISTEN versus the
	// new TRESTLE_HOST/TRESTLE_PORT forms must not silently pick one.
	if v, ok := os.LookupEnv("TRESTLE_LISTEN"); ok {
		if _, hasHost := os.LookupEnv("TRESTLE_HOST"); hasHost {
			return "", errors.New("TRESTLE_LISTEN cannot be combined with TRESTLE_HOST")
		}
		if _, hasPort := os.LookupEnv("TRESTLE_PORT"); hasPort {
			return "", errors.New("TRESTLE_LISTEN cannot be combined with TRESTLE_PORT")
		}
		if strings.TrimSpace(v) == "" {
			return "", errors.New("TRESTLE_LISTEN is set but empty")
		}
		return v, nil
	}
	host, port, err := resolveHostPort(hostFlag, portFlag, hostSet, portSet)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, port), nil
}

// listenerFlagsFromArgs scans raw argv for the --host/--port/--listen flags and
// reports their values and whether each was explicitly supplied. It runs before
// the durable config load so the CLI can distinguish "provided but empty"
// (--host "") from "not provided", and so the legacy --listen form stays
// mutually exclusive with the new host/port form. A flag whose value token is
// missing or begins with "-" is left unset: the flag package reports the
// malformed argument when the durable config is parsed.
func listenerFlagsFromArgs(args []string) (host, port, listen string, hostSet, portSet, listenSet bool) {
	needValue := func(name string, i int) bool {
		return i+1 < len(args) && !strings.HasPrefix(args[i+1], "-")
	}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--host" && needValue("--host", i):
			host, hostSet, i = args[i+1], true, i+1
		case strings.HasPrefix(args[i], "--host="):
			host, hostSet = strings.TrimPrefix(args[i], "--host="), true
		case args[i] == "--port" && needValue("--port", i):
			port, portSet, i = args[i+1], true, i+1
		case strings.HasPrefix(args[i], "--port="):
			port, portSet = strings.TrimPrefix(args[i], "--port="), true
		case args[i] == "--listen" && needValue("--listen", i):
			listen, listenSet, i = args[i+1], true, i+1
		case strings.HasPrefix(args[i], "--listen="):
			listen, listenSet = strings.TrimPrefix(args[i], "--listen="), true
		}
	}
	return host, port, listen, hostSet, portSet, listenSet
}

// listenerOverrideSelected reports whether the user explicitly selected the new
// host/port listener form (CLI flags or TRESTLE_HOST/TRESTLE_PORT environment).
// Legacy --listen and bare invocations keep the durable config listener.
func listenerOverrideSelected(hostSet, portSet bool) bool {
	if hostSet || portSet {
		return true
	}
	if _, ok := os.LookupEnv("TRESTLE_HOST"); ok {
		return true
	}
	if _, ok := os.LookupEnv("TRESTLE_PORT"); ok {
		return true
	}
	return false
}

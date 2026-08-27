package server

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSlowHeaderClientIsDisconnected(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }), ReadHeaderTimeout: 100 * time.Millisecond}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Close(); <-done })
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte("GET / HTTP/1.1\r\nHost: example")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	connection.SetReadDeadline(time.Now().Add(time.Second))
	line, err := bufio.NewReader(connection).ReadString('\n')
	if err == nil && strings.Contains(line, "204") {
		t.Fatalf("slow incomplete request reached handler: %q", line)
	}
}

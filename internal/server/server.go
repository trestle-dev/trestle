package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/trestle-dev/trestle/internal/buildinfo"
	"github.com/trestle-dev/trestle/internal/httperr"
)

type Server struct {
	logger  *slog.Logger
	ready   atomic.Bool
	ids     atomic.Uint64
	handler http.Handler
}

func New(logger *slog.Logger) *Server {
	s := &Server{logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /system/health", s.health)
	mux.HandleFunc("GET /system/ready", s.readiness)
	mux.HandleFunc("GET /system/version", s.version)
	s.handler = s.requestContext(mux)
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }
func (s *Server) SetReady(value bool)   { s.ready.Store(value) }

func (s *Server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		id := "req_" + strconv.FormatUint(s.ids.Add(1), 36)
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
		s.logger.Info("request", "request_id", id, "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, httperr.New("not_ready", "The service is not ready.", w.Header().Get("X-Request-ID")))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, buildinfo.Current())
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

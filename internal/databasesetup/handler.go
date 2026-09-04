package databasesetup

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/trestle-cv/trestle/internal/adminauth"
	"github.com/trestle-cv/trestle/internal/config"
	"github.com/trestle-cv/trestle/internal/httperr"
	"github.com/trestle-cv/trestle/internal/requestmeta"
	"github.com/trestle-cv/trestle/internal/store"
)

type Options struct {
	DataDir                         string
	Current                         store.Provider
	Explicit                        bool
	MaxOpen, MaxIdle                int
	ConnectTimeout, ConnMaxLifetime time.Duration
}
type Handler struct {
	admin   *adminauth.Handler
	options Options
}
type input struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
}

func New(admin *adminauth.Handler, options Options) *Handler {
	return &Handler{admin: admin, options: options}
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	required, err := h.admin.SetupRequired(r.Context())
	if err != nil {
		writeError(w, 500, "setup_unavailable", "The request could not be completed.")
		return
	}
	if !required {
		writeError(w, 409, "setup_complete", "First-run setup is complete.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		write(w, 200, map[string]any{"provider": h.options.Current, "explicit": h.options.Explicit, "selectable": !h.options.Explicit})
	case http.MethodPost:
		h.save(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	if h.options.Explicit {
		writeError(w, 409, "database_fixed", "The database is fixed by startup configuration.")
		return
	}
	if !sameOrigin(r) {
		writeError(w, 403, "origin_denied", "The request origin is not allowed.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var in input
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil {
		writeError(w, 400, "invalid_request", "The request is invalid.")
		return
	}
	provider, err := store.ParseProvider(strings.TrimSpace(in.Provider))
	if err != nil {
		writeError(w, 422, "invalid_provider", err.Error())
		return
	}
	value := config.DatabaseBootstrap{Provider: string(provider), URL: strings.TrimSpace(in.URL), MaxOpen: h.options.MaxOpen, MaxIdle: h.options.MaxIdle, ConnectTimeout: h.options.ConnectTimeout, ConnMaxLifetime: h.options.ConnMaxLifetime}
	if err := value.Validate(); err != nil {
		writeError(w, 422, "invalid_database_configuration", err.Error())
		return
	}
	probeContext, cancel := context.WithTimeout(r.Context(), h.options.ConnectTimeout)
	defer cancel()
	version, err := store.Probe(probeContext, provider, strings.TrimSpace(in.URL), h.options.ConnectTimeout)
	if err != nil {
		writeError(w, 422, "connection_failed", err.Error())
		return
	}
	if err := config.PersistDatabaseBootstrap(h.options.DataDir, value); err != nil {
		writeError(w, 500, "persist_failed", "The database configuration could not be saved.")
		return
	}
	write(w, 200, map[string]any{"provider": provider, "version": version, "restartRequired": provider != h.options.Current})
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || origin == requestmeta.Scheme(r)+"://"+r.Host
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	write(w, status, httperr.New(code, message, w.Header().Get("X-Request-ID")))
}

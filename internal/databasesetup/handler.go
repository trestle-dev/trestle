package databasesetup

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/config"
	"github.com/trestle-dev/trestle/internal/requestmeta"
	"github.com/trestle-dev/trestle/internal/store"
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
		write(w, 500, map[string]string{"error": "database setup unavailable"})
		return
	}
	if !required {
		write(w, 409, map[string]string{"error": "first-run setup is complete"})
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
		write(w, 409, map[string]string{"error": "database is fixed by startup configuration"})
		return
	}
	if !sameOrigin(r) {
		write(w, 403, map[string]string{"error": "request origin is not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var in input
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil {
		write(w, 400, map[string]string{"error": "invalid database setup request"})
		return
	}
	provider, err := store.ParseProvider(strings.TrimSpace(in.Provider))
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	value := config.DatabaseBootstrap{Provider: string(provider), URL: strings.TrimSpace(in.URL), MaxOpen: h.options.MaxOpen, MaxIdle: h.options.MaxIdle, ConnectTimeout: h.options.ConnectTimeout, ConnMaxLifetime: h.options.ConnMaxLifetime}
	if err := value.Validate(); err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	probeContext, cancel := context.WithTimeout(r.Context(), h.options.ConnectTimeout)
	defer cancel()
	version, err := store.Probe(probeContext, provider, strings.TrimSpace(in.URL))
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	if err := config.PersistDatabaseBootstrap(h.options.DataDir, value); err != nil {
		write(w, 500, map[string]string{"error": "database configuration could not be saved"})
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
